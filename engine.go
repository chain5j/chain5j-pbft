// Package pbft
//
// @author: xwc1125
package pbft

import (
	"context"
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pbft/validator"
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-pkg/util/dateutil"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/chain5j-protocol/protocol"
	"time"
)

var (
	_ protocol.Consensus = new(pbftEngine)
)

var (
	faultTolerantTime = int64(5 * 1000) // pbft对服务器时间容错的间隔（毫秒）
)

// Start 启动引擎
func (b *pbftEngine) Start() error {
	b.pbftCore.Start()
	//b.pbftCore.NewChainHead()// 在begin中已经调用
	return nil
}

// Stop 停止引擎
func (b *pbftEngine) Stop() error {
	return b.pbftCore.Stop()
}

// Begin 启动新的链head
func (b *pbftEngine) Begin() error {
	return b.pbftCore.NewChainHead()
}

// VerifyHeader 检查Header是否符合引擎的一致规则
func (b *pbftEngine) VerifyHeader(blockReader protocol.BlockReader, header *models.Header) error {
	return b.verifyHeader(blockReader, header, nil)
}

// verifyHeader 检查标头是否符合一致性规则。
// 调用方可以选择传入一批父级（升序），
// 以避免从数据库中查找父级。这对于并发验证一批新标头非常有用。
func (b *pbftEngine) verifyHeader(blockReader protocol.BlockReader, header *models.Header, parents []*models.Header) error {
	if header.Height == b.config.ChainConfig().GenesisHeight {
		// 如果是创世高度，那么返回错误
		return errBlockIsGenesis
	}

	// 增加5秒钟的时间容错，避免时间不同步
	if header.Timestamp > uint64(dateutil.CurrentTime()+faultTolerantTime) {
		return pbftProtocol.ErrFutureBlock
	}

	// 从header中提取共识所需的参数数据
	consensusData := new(ConsensusData)
	if err := consensusData.FromConsensus(header.Consensus); err != nil {
		b.log.Error("extract consensus data err", "err", err)
		return err
	}

	return b.verifyCascadingFields(blockReader, header, parents)
}

// verifyCascadingFields 验证并非独立的所有标头字段，而是依赖于以前的一批标头。
// 调用方可以选择传入一批父级（升序），以避免从数据库中查找父级。
// 这对于并发验证一批新标头非常有用。
func (b *pbftEngine) verifyCascadingFields(blockReader protocol.BlockReader, header *models.Header, parents []*models.Header) error {
	height := header.Height
	if height == b.config.ChainConfig().GenesisHeight {
		return nil
	}

	var (
		parent *models.Header
	)
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = blockReader.GetHeader(header.ParentHash, height-1)
	}
	if parent == nil || parent.Height != height-1 || parent.Hash() != header.ParentHash {
		return pbftProtocol.ErrUnknownAncestor
	}

	// 保证两个区块之间的时间间隔不能太近
	period := b.selfConfig.BlockPeriod
	if period == 0 {
		period = 1
	}
	if parent.Timestamp+period > header.Timestamp {
		return errInvalidTimestamp
	}

	// 验证区块的签名地址为Validator
	if err := b.verifySigner(blockReader, header, parents); err != nil {
		return err
	}

	// 验证commit签名
	return b.verifyCommittedSeals(blockReader, header, parents)
}

// verifySigner 验证区块的签名地址为父区块中的Validator
func (b *pbftEngine) verifySigner(blockReader protocol.BlockReader, header *models.Header, parents []*models.Header) error {
	height := header.Height
	if height == b.config.ChainConfig().GenesisHeight {
		// todo 创世区块不支持校验
		return errUnknownBlock
	}

	// 获取上一个区块的快照
	snap, err := b.snapshot(blockReader, height-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// 从签名中获取签名者
	signer, err := b.ecrecover(b.nodeKey, header)
	if err != nil {
		return err
	}

	// 签名者应该是父快照中的validator
	if _, v := snap.ValSet.GetById(signer); v == nil {
		return pbftProtocol.ErrUnauthorized
	}
	return nil
}

// verifyCommittedSeals 验证committed的签名地址为父区块中的Validator
func (b *pbftEngine) verifyCommittedSeals(blockReader protocol.BlockReader, header *models.Header, parents []*models.Header) error {
	height := header.Height
	if height == b.config.ChainConfig().GenesisHeight {
		// 创世块不用处理
		return nil
	}

	// 解析共识内容
	consensusData := new(ConsensusData)
	if err := consensusData.FromConsensus(header.Consensus); err != nil {
		return err
	}

	if len(consensusData.CommittedSeal) == 0 {
		return errEmptyCommittedSeals
	}

	if consensusData.MVote != nil {
		enc, _ := codec.Coder().Encode([]interface{}{
			"propose",
			consensusData.MVote.Candidate,
			consensusData.MVote.Authorize,
			consensusData.MVote.DeadlineHeight,
		})
		signResult := new(signature.SignResult)
		err := signResult.Deserialize(consensusData.MVote.Signature)
		if err != nil {
			return err
		}
		manager, err := ecrecoverVote(b.nodeKey, enc, signResult)
		if err != nil {
			return err
		}

		if manager != consensusData.MVote.Manager {
			return errInvalidSignature
		}
	}

	if consensusData.VVote != nil {
		enc, _ := codec.Coder().Encode([]interface{}{
			"propose",
			consensusData.VVote.Validator,
			consensusData.VVote.Authorize,
			consensusData.VVote.DeadlineHeight,
		})
		signResult := new(signature.SignResult)
		err := signResult.Deserialize(consensusData.VVote.Signature)
		if err != nil {
			return err
		}
		manager, err := ecrecoverVote(b.nodeKey, enc, signResult)
		if err != nil {
			return err
		}

		if manager != consensusData.VVote.Manager {
			return errInvalidSignature
		}
	}

	// 获取上一个区块的快照
	snap, err := b.snapshot(blockReader, height-1, header.ParentHash, parents)
	if err != nil {
		return err
	}
	validators := snap.ValSet.Copy()

	validSeal := 0
	proposalSeal := pbftProtocol.PrepareCommittedSeal(header.Hash())
	// 1. 从当前区块中获取committed seals
	for _, seal := range consensusData.CommittedSeal {
		// 2. 从签名中获取签名地址
		id, err := pbftProtocol.GetSignatureValidator(b.nodeKey, proposalSeal, seal)
		if err != nil {
			b.log.Error("not a valid address", "err", err)
			return errInvalidSignature
		}
		// 每个validator只能有一个签名
		// 如果validator签署了多个印章，则无法找到validator，
		// 并返回errInvalidCommittedSeals。
		if validators.RemoveValidator(id) {
			validSeal += 1
		} else {
			return errInvalidCommittedSeals
		}
	}

	// validSeal的长度应大于故障节点数+1
	if validSeal <= 2*snap.ValSet.FaultTolerantNum() {
		return errInvalidCommittedSeals
	}

	return nil
}

// Prepare 根据规则初始化header的字段
func (b *pbftEngine) Prepare(blockReader protocol.BlockReader, header *models.Header) error {
	height := header.Height
	parent := blockReader.GetHeader(header.ParentHash, height-1)
	if parent == nil {
		return pbftProtocol.ErrUnknownAncestor
	}

	// 获取上一个区块的快照
	snap, err := b.snapshot(blockReader, height-1, header.ParentHash, nil)
	if err != nil {
		return err
	}

	mVote, vVote := b.voteManager.getVotes(height, snap.checkMVote, snap.checkVVote)
	cData := &ConsensusData{
		MVote: mVote,
		VVote: vVote,
	}
	header.Consensus, err = cData.ToConsensus()
	if err != nil {
		return err
	}
	period := b.selfConfig.BlockPeriod
	if period == 0 {
		period = 1
	}
	// 设置header的时间戳
	header.Timestamp = parent.Timestamp + period
	if header.Timestamp < uint64(dateutil.CurrentTime()) {
		header.Timestamp = uint64(dateutil.CurrentTime())
	}

	return nil
}

// Finalize 运行交易后状态修改（例如区块奖励）并组装最终区块
func (b *pbftEngine) Finalize(blockReader protocol.BlockReader, header *models.Header, txs models.Transactions) (*models.Block, error) {
	return models.NewBlock(header, txs, nil), nil
}

// Seal 密封区块
func (b *pbftEngine) Seal(ctx context.Context, blockReader protocol.BlockReader, block *models.Block, results chan<- *models.Block) error {
	t1 := time.Now()
	header := block.Header()
	height := header.Height
	// 获取父区块的快照
	snap, err := b.snapshot(blockReader, height-1, header.ParentHash, nil)
	if err != nil {
		b.log.Warn("seal-1) get snapshot err", "height", height-1, "bHash", header.ParentHash)
		return err
	}
	if b.config.ConsensusConfig().IsMetrics(1) {
		b.log.Debug("seal-1) get snapshot end", "height", height-1, "bHash", header.ParentHash, "elapsed", dateutil.PrettyDuration(time.Since(t1)))
	}
	var t2 time.Time
	{
		t2 = time.Now()
		// 判断当前peerId是否为验证者
		if _, v := snap.ValSet.GetById(b.nodeId.String()); v == nil {
			b.log.Warn("seal-2) get validator err", "nodeId", b.nodeId, "err", pbftProtocol.ErrUnauthorized)
			return pbftProtocol.ErrUnauthorized
		}
		if b.config.ConsensusConfig().IsMetrics(1) {
			b.log.Debug("seal-2) get validator end", "nodeId", b.nodeId, "elapsed", dateutil.PrettyDuration(time.Since(t2)))
		}
	}
	t2 = time.Now()
	// 获取父区块header
	parent := blockReader.GetHeader(header.ParentHash, height-1)
	if parent == nil {
		b.log.Warn("seal-3) get header by parent err", "height", height-1, "bHash", header.ParentHash)
		return pbftProtocol.ErrUnknownAncestor
	}
	if b.config.ConsensusConfig().IsMetrics(1) {
		b.log.Debug("seal-3) get header by parent end", "height", height-1, "bHash", header.ParentHash, "elapsed", dateutil.PrettyDuration(time.Since(t2)))
	}

	// 更新时间戳和区块签名
	t2 = time.Now()
	block, err = b.updateBlock(parent, block)
	if err != nil {
		b.log.Warn("seal-4) update block err", "parentHeight", parent.Height, "parentHash", parent.Hash(), "blockHeight", block.Height(), "blockHash", block.Hash())
		return err
	}
	if b.config.ConsensusConfig().IsMetrics(1) {
		b.log.Debug("seal-4) update block end", "parentHeight", parent.Height, "parentHash", parent.Hash(), "blockHeight", block.Height(), "blockHash", block.Hash(), "elapsed", dateutil.PrettyDuration(time.Since(t2)))
	}

	delay := time.Unix(int64(block.Header().Timestamp), 0).Sub(time.Now())
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		results <- nil
		return nil
	}

	t2 = time.Now()
	// 将区块提交给core
	b.sealLock.Lock()
	b.proposedBlockHash = block.Hash()
	b.sealLock.Unlock()
	if b.config.ConsensusConfig().IsMetrics(1) {
		b.log.Debug("seal-5) proposed block hash end", "proposedHeight", block.Height(), "proposedHash", block.Hash(), "elapsed", dateutil.PrettyDuration(time.Since(t2)))
	}

	t2 = time.Now()
	err = b.pbftCore.Request(&pbftProtocol.Request{
		Proposal: block,
	})
	if err != nil {
		results <- nil
		b.log.Error("seal-6) pbft core request err", "err", err)
		return err
	}
	if b.config.ConsensusConfig().IsMetrics(1) {
		b.log.Debug("seal-6) pbft core request end", "proposedHeight", block.Height(), "proposedHash", block.Hash(), "elapsed", dateutil.PrettyDuration(time.Since(t2)))
	}

	go func() {
		t2 := time.Now()
		b.clearCommitChannel()
		if b.config.ConsensusConfig().IsMetrics(1) {
			b.log.Debug("seal-7) clear commit channel end", "elapsed", dateutil.PrettyDuration(time.Since(t2)))
		}
		t2 = time.Now()
		// 如果seal完成后能够得到提案hash，那么清空提案区块hash
		b.sealLock.Lock()
		clear := func() {
			b.proposedBlockHash = types.Hash{}
			b.sealLock.Unlock()
		}
		defer clear()
		if b.config.ConsensusConfig().IsMetrics(1) {
			b.log.Debug("seal-8) seal lock end", "elapsed", dateutil.PrettyDuration(time.Since(t2)))
		}

		select {
		case result := <-b.commitCh:
			b.log.Debug("seal-9) commit result", "hash", result.Hash(), "height", result.Height())
			// 如果commitCh的结果中，区块hash和提案的hash一致，那么返回结果。否则等待下一个结果
			if result != nil && block.Hash() == result.Hash() {
				results <- result
				b.log.Debug("seal-10) send result to chan", "hash", result.Hash(), "height", result.Height())
			} else {
				if result != nil {
					b.log.Error("seal-10) commitCh block hash diff", "block.Hash", block.Hash().Hex(), "block.Height", block.Height(), "result.Hash", result.Hash().Hex(), "result.Height", result.Height())
				} else {
					b.log.Error("seal-10) commitCh block hash diff", "block.Hash", block.Hash().Hex(), "block.Height", block.Height(), "result", "nil")
				}
				results <- nil
			}
		case <-ctx.Done():
			if ctx.Err() != context.Canceled {
				b.log.Trace("seal-11) pbft engine request timeout")
				//go b.pbftCore.RequestTimeout()
			} else {
				b.log.Trace("seal-11) seal canceled by new header")
			}
			results <- nil
		}
	}()

	return nil
}

// clearCommitChannel 清空commitCh
func (b *pbftEngine) clearCommitChannel() {
	for {
		select {
		case <-b.commitCh:
			b.log.Debug("seal-7_1) clear commit chan")
		default:
			return
		}
	}
}

// updateBlock 更新时间戳和区块签名
func (b *pbftEngine) updateBlock(parent *models.Header, block *models.Block) (*models.Block, error) {
	header := block.Header()
	t2 := time.Now()
	// 签名区块
	seal, err := b.Sign(header.HashNoSign().Bytes())
	if err != nil {
		if b.config.ConsensusConfig().IsMetrics(2) {
			b.log.Error("seal-4_1) sign header err", "err", err)
		}
		return nil, err
	}
	if b.config.ConsensusConfig().IsMetrics(2) {
		b.log.Debug("seal-4_1) sign header end", "elapsed", dateutil.PrettyDuration(time.Since(t2)))
	}
	err = writeSeal(header, seal)
	if err != nil {
		if b.config.ConsensusConfig().IsMetrics(2) {
			b.log.Error("seal-4_2) write seal err", "err", err)
		}
		return nil, err
	}

	return block.WithSeal(header), nil
}

// writeSeal 给header进行签名赋值
func writeSeal(h *models.Header, seal *signature.SignResult) error {
	if seal == nil {
		return errInvalidSignature
	}
	h.Signature = seal
	return nil
}

// writeCommittedSeals 根据committedSeals添加到extra-data中
func (b *pbftEngine) writeCommittedSeals(h *models.Header, committedSeals []*signature.SignResult) error {
	if len(committedSeals) == 0 {
		return errInvalidCommittedSeals
	}

	//for _, seal := range committedSeals {
	//	_, err := signature.ParseSign(seal)
	//	if err != nil {
	//		return err
	//	}
	//}

	consensusData := new(ConsensusData)
	if err := consensusData.FromConsensus(h.Consensus); err != nil {
		return err
	}

	consensusData.CommittedSeal = committedSeals

	payload, err := consensusData.Serialize()
	if err != nil {
		return err
	}

	h.Consensus = &models.Consensus{
		Name:      "pbft",
		Consensus: payload,
	}
	return nil
}

// snapshot 快照
func (b *pbftEngine) snapshot(blockReader protocol.BlockReader, height uint64, hash types.Hash, parents []*models.Header) (*Snapshot, error) {
	// 在内存或磁盘上搜索快照以查找检查点
	var (
		headers []*models.Header
		snap    *Snapshot
	)

	for snap == nil {
		// 如果内存中已经存在，那么直接返回
		if s, ok := b.recentSnapshotCache.Get(hash); ok {
			snap = s.(*Snapshot)
			break
		}

		// 内存不存在，那么从磁盘中获取
		if height%checkpointInterval == 0 {
			s, err := loadSnapshot(b.selfConfig.Epoch, b.db, hash)
			if err == nil {
				b.log.Trace("seal-1_1) loaded voting snapshot form disk", "height", height, "hash", hash)
				snap = s
				break
			}
		}
		// 磁盘中也没有的时候
		// 如果num小于创世区块，那么返回错误
		genesisHeight := b.config.ChainConfig().GenesisHeight
		if height < genesisHeight {
			return nil, errBlockSmall
		}
		// number等于创世块，创建新的快照
		if height == genesisHeight {
			// 获取创世块
			genesis := blockReader.GetHeaderByNumber(genesisHeight)
			// 验证header（时间戳、字段、签名等）
			if err := b.VerifyHeader(blockReader, genesis); err != nil {
				if err != errBlockIsGenesis {
					return nil, err
				}
			}

			// 创建新的快照
			snap = newSnapshot(b.selfConfig.Epoch, genesisHeight, genesis.Hash(),
				validator.NewSet(b.selfConfig.Validators, b.selfConfig.ProposerPolicy),
				b.selfConfig.Managers)

			// 快照入库
			if err := snap.Store(b.db); err != nil {
				return nil, err
			}
			b.log.Trace("seal-1_2) stored genesis voting snapshot to disk")
			break
		}

		// No snapshot for this header, gather the header and move backward
		var header *models.Header
		if len(parents) > 0 {
			// If we have explicit parents, pick from there (enforced)
			header = parents[len(parents)-1]
			if header.Hash() != hash || header.Height != height {
				return nil, pbftProtocol.ErrUnknownAncestor
			}
			parents = parents[:len(parents)-1]
		} else {
			// No explicit parents (or no more left), reach out to the database
			header = blockReader.GetHeader(hash, height)
			if header == nil {
				return nil, pbftProtocol.ErrUnknownAncestor
			}
		}
		headers = append(headers, header)
		height, hash = height-1, header.ParentHash
	}

	// Previous snapshot found, apply any pending headers on top of it
	for i := 0; i < len(headers)/2; i++ {
		headers[i], headers[len(headers)-1-i] = headers[len(headers)-1-i], headers[i]
	}

	snap, err := snap.Apply(headers)
	if err != nil {
		return nil, err
	}
	b.recentSnapshotCache.Add(snap.Hash, snap)

	// If we've generated a new checkpoint snapshot, save to disk
	if snap.Height%checkpointInterval == 0 && len(headers) > 0 {
		if err := snap.Store(b.db); err != nil {
			return nil, err
		}
		b.log.Trace("seal-1_3) stored voting snapshot to disk", "height", snap.Height, "hash", snap.Hash)
	}

	return snap, nil
}
