// Package pbft
//
// @author: xwc1125
package pbft

import (
	"context"
	"fmt"
	"github.com/chain5j/chain5j-pbft/core"
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pbft/validator"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/database/kvstore"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-protocol/models"
	pcrypto "github.com/chain5j/chain5j-protocol/pkg/crypto"
	"github.com/chain5j/chain5j-protocol/protocol"
	"github.com/chain5j/logger"
	"github.com/hashicorp/golang-lru"
	"reflect"
	"strings"
	"sync"
)

var (
	_ pbftProtocol.PBFTBackend = new(pbftEngine)
)

const (
	memoryValidatorLen = 20
	memorySnapshotsLen = 128
	checkpointInterval = 1024
)

// pbftEngine pbft引擎
type pbftEngine struct {
	log    logger.Logger
	ctx    context.Context
	cancel context.CancelFunc
	nodeId models.NodeID // 节点ID

	nodeKey     protocol.NodeKey
	config      protocol.Config
	broadcaster protocol.Broadcaster
	blockReader protocol.BlockReader
	dbReader    protocol.DatabaseReader
	apis        protocol.APIs

	selfConfig *pbftProtocol.PBFTConfig // pbft的配置
	pbftCore   pbftProtocol.PBFTEngine  // pbft核心引擎

	db kvstore.Database

	commitCh      chan *models.Block
	finalCommitCh chan struct{}

	sealLock          sync.Mutex
	proposedBlockHash types.Hash

	voteManager *manager

	recentSnapshotCache  *lru.ARCCache // 最近区块的缓存
	recentValidatorCache *lru.ARCCache // 最近验证者缓存
}

func (b *pbftEngine) VerifyHeaders(blockReader protocol.BlockReader, headers []*models.Header, seals []bool) (chan<- struct{}, <-chan error) {
	//TODO implement me
	panic("implement me")
}

// NewConsensus 创建pbft
func NewConsensus(rootCtx context.Context, opts ...option) (protocol.Consensus, error) {
	ctx, cancel := context.WithCancel(rootCtx)
	e := &pbftEngine{
		log:    logger.New("pbft"),
		ctx:    ctx,
		cancel: cancel,

		commitCh:      make(chan *models.Block, 1),
		finalCommitCh: make(chan struct{}, 1),

		voteManager: newManager(),
	}

	var err error
	if err := apply(e, opts...); err != nil {
		e.log.Error("apply options error", "err", err)
		return nil, err
	}
	e.recentValidatorCache, err = lru.NewARC(memoryValidatorLen)
	if err != nil {
		e.log.Error("lru new recent validator arc err", "err", err)
		return nil, err
	}
	// 缓存
	e.recentSnapshotCache, err = lru.NewARC(memorySnapshotsLen)
	if err != nil {
		e.log.Error("lru new recent snapshot arc err", "err", err)
		return nil, err
	}
	e.nodeId, err = e.nodeKey.ID()
	if err != nil {
		e.log.Error("get nodeId err", "err", err)
		return nil, err
	}

	chainConfig := e.config.ChainConfig()
	if cName := chainConfig.Consensus.Name; !strings.EqualFold("pbft", cName) {
		e.log.Error("consensus is not pbft", "consensus", cName)
		return nil, fmt.Errorf("consensus is not pbft: %s", cName)
	}

	// 解析
	selfConfig := new(pbftProtocol.PBFTConfig)
	err = chainConfig.Consensus.Data.ToStruct(selfConfig)
	if err != nil {
		e.log.Error("decode consensus data err", "err", err)
		return nil, err
	}
	e.selfConfig = selfConfig

	// 当前区块
	//currentBlock := e.blockReader.CurrentBlock()
	//// 当前区块的校验者
	//valSet := e.getValidators(currentBlock.Height(), currentBlock.Hash())

	// 创建PBFT的核心
	e.pbftCore = core.NewEngine(e, e.broadcaster)

	// 注册API
	e.apis.RegisterAPI([]protocol.API{{
		Namespace: "pbft",
		Version:   "1.0",
		Service:   &API{b: e, chain: e.blockReader},
		Public:    true,
	}})
	return e, nil
}

// ID 实现 pbft.Backend.ID
func (b *pbftEngine) ID() string {
	return b.nodeId.String()
}

// Author 从header中获取签名者地址
func (b *pbftEngine) Author(header *models.Header) (string, error) {
	return b.ecrecover(b.nodeKey, header)
}

// Validators 校验提案
func (b *pbftEngine) Validators(proposal pbftProtocol.Proposal) pbftProtocol.ValidatorSet {
	return b.getValidators(proposal.Height(), proposal.Hash())
}

// LastProposal 获取最新的提案
func (b *pbftEngine) LastProposal() (pbftProtocol.Proposal, string) {
	block := b.blockReader.CurrentBlock()

	var proposer string
	if block.Height() > b.config.ChainConfig().GenesisHeight {
		var err error
		proposer, err = b.Author(block.Header())
		if err != nil {
			b.log.Error("Failed to get block proposer", "err", err)
			return nil, ""
		}
	}

	return block, proposer
}

// Commit 实现 pbft.Backend.Commit
func (b *pbftEngine) Commit(proposal pbftProtocol.Proposal, seals []*signature.SignResult) error {
	// 判断提案是否为合法区块
	block := &models.Block{}
	block, ok := proposal.(*models.Block)
	if !ok {
		b.log.Error("Invalid proposal, %v", proposal)
		return errInvalidProposal
	}

	h := block.Header()
	// todo block已经被签名了，共识部分的内容需要放在什么地方，不能进行hash处理
	// 将CommittedSeals写入header consensus data
	err := b.writeCommittedSeals(h, seals)
	if err != nil {
		return err
	}
	// 更新区块为含签名的数据
	block = block.WithSeal(h)

	b.log.Debug("Committed", "validator", b.nodeId, "proposed", b.proposedBlockHash, "hash", proposal.Hash(), "height", proposal.Height())
	// - if the proposed and committed blocks are the same, send the proposed hash
	//   to commit channel, which is being watched inside the engine.Seal() function.
	// - otherwise, we try to insert the block.
	// -- if success, the ChainHeadEvent event will be broadcasted, try to build
	//    the next block and the previous Seal() will be stopped.
	// -- otherwise, a error will be returned and a round change event will be fired.
	//if b.proposedBlockHash == block.Hash() {
	//	// feed block hash to Seal() and wait the Seal() result
	//	b.commitCh <- block
	//	return nil
	//} else if (b.proposedBlockHash != types.Hash{}) {
	//	b.commitCh <- nil
	//} else {
	//	b.log.Trace("engine quit")
	//}

	go func() {
		select {
		case b.commitCh <- block:
			// 将区块提交给commit
			b.log.Debug("Commit block", "height", block.Height(), "hash", block.Hash())
		default:
			return
		}
	}()

	return nil
}

// Verify 实现 pbft.Backend.Verify，判断区块是否为合法区块
func (b *pbftEngine) Verify(proposal pbftProtocol.Proposal) error {
	block := &models.Block{}
	block, ok := proposal.(*models.Block)
	if !ok {
		b.log.Error("Invalid proposal, %v", proposal)
		return errInvalidProposal
	}

	// 校验区块的body
	if !reflect.DeepEqual(block.Transactions(), block.Header().TxsRoot) {
		return errMismatchTxRoot
	}

	// 通过已经落库的区块来校验提案区块
	err := b.VerifyHeader(b.blockReader, block.Header())
	// ignore errEmptyCommittedSeals错误，因为还没有进行签名
	if err == nil || err == errEmptyCommittedSeals {
		return nil
	}

	return err
}

// Sign 实现 pbft.Backend.Sign
func (b *pbftEngine) Sign(data []byte) (*signature.SignResult, error) {
	// 节点对pbft的数据进行签名
	// 将数据进行转换为SignResult
	return b.nodeKey.Sign(data)
}

// CheckSignature 实现 pbft.Backend.CheckSignature
func (b *pbftEngine) CheckSignature(data []byte, nodeId string, sig *signature.SignResult) error {
	signer, err := pbftProtocol.GetSignatureValidator(b.nodeKey, data, sig)
	if err != nil {
		b.log.Error("Failed to get signer address", "err", err)
		return err
	}
	// 比较peerId和签名中的地址
	if signer != nodeId {
		return errInvalidSignature
	}
	return nil
}

// HasProposal 实现 pbft.Backend.HashBlock
func (b *pbftEngine) HasProposal(hash types.Hash, height uint64) bool {
	return b.blockReader.GetHeader(hash, height) != nil
}

// ParentValidators 实现 pbft.Backend.GetParentValidators
func (b *pbftEngine) ParentValidators(proposal pbftProtocol.Proposal) pbftProtocol.ValidatorSet {
	if block, ok := proposal.(*models.Block); ok {
		return b.getValidators(block.Height()-1, block.ParentHash())
	}
	return validator.NewSet(nil, b.selfConfig.ProposerPolicy)
}

// GetProposer 实现 pbft.Backend.GetProposer
func (b *pbftEngine) GetProposer(height uint64) string {
	if h := b.blockReader.GetHeaderByNumber(height); h != nil {
		a, _ := b.Author(h)
		return a
	}
	return ""
}

// getValidators 获取验证者集合
func (b *pbftEngine) getValidators(height uint64, hash types.Hash) pbftProtocol.ValidatorSet {
	snap, err := b.snapshot(b.blockReader, height, hash, nil)
	if err != nil {
		return validator.NewSet(nil, b.selfConfig.ProposerPolicy)
	}
	return snap.ValSet
}

// ecrecover 从header中获取签名地址
func (b *pbftEngine) ecrecover(nodeKey protocol.NodeKey, h *models.Header) (string, error) {
	hash := h.HashNoSign()
	if nodeId, ok := b.recentValidatorCache.Get(hash); ok {
		return nodeId.(string), nil
	}

	// 获取签名地址
	peerId, err := pbftProtocol.GetSignatureValidator(nodeKey, hash.Bytes(), h.Signature)
	if err != nil {
		return peerId, err
	}

	b.recentValidatorCache.Add(hash, peerId)
	return peerId, nil
}

// ecrecoverVote 恢复签名地址
func ecrecoverVote(nodeKey protocol.NodeKey, data []byte, sig *signature.SignResult) (types.Address, error) {
	pub, err := nodeKey.RecoverPub(data, sig)
	if err != nil {
		return types.Address{}, err
	}
	return pcrypto.PubkeyToAddress(pub)
}
