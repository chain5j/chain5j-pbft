// Package pbft
//
// @author: xwc1125
package pbft

import (
	"context"
	"fmt"
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/collection/maps/hashmap"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/database/kvstore/memorydb"
	"github.com/chain5j/chain5j-pkg/event"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-pkg/util/dateutil"
	"github.com/chain5j/chain5j-pkg/util/hexutil"
	"github.com/chain5j/chain5j-protocol/mock"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/chain5j-protocol/protocol"
	"github.com/chain5j/logger"
	"github.com/chain5j/logger/zap"
	"github.com/golang/mock/gomock"
	"testing"
)

func init() {
	zap.InitWithConfig(&logger.LogConfig{
		Console: logger.ConsoleLogConfig{
			Level:    4,
			Modules:  "*",
			ShowPath: false,
			Format:   "",
			UseColor: true,
			Console:  true,
		},
		File: logger.FileLogConfig{},
	})
}

func TestPBFT(t *testing.T) {
	mockCtl := gomock.NewController(nil)
	mockConfig := mock.NewMockConfig(mockCtl)
	hashMap := hashmap.NewHashMap(true)
	hashMap.Put("epoch", 3000)
	hashMap.Put("timeout", 1000)
	hashMap.Put("period", 0)
	hashMap.Put("policy", 0)
	managersAddr := []types.Address{
		types.HexToAddress("0x2213830F680cD75FeA3AD4D37BBC0E244A412c04"),
	}
	hashMap.Put("managers", managersAddr)
	validators := []string{
		"0x2213830F680cD75FeA3AD4D37BBC0E244A412c04",
		//"0x7C286d572eCD157f230DF3F19A7F127AADF37ADc",
	}
	hashMap.Put("validators", validators)
	mockConfig.EXPECT().ChainConfig().Return(models.ChainConfig{
		ChainID:       1,
		ChainName:     "",
		VersionName:   "",
		VersionCode:   0,
		GenesisHeight: 0,
		TxSizeLimit:   0,
		Packer:        nil,
		Consensus: &models.ConsensusConfig{
			Name: "pbft",
			Data: hashMap,
		},
		StateApp: nil,
	}).AnyTimes()

	// 0x1c7dd8ac4be69e3bcc9ba2e609bf7bb767f40824894b20c4eddbe36d2461f166
	// 0x7C286d572eCD157f230DF3F19A7F127AADF37ADc
	// 0xf3bf2f58e4e369d101786024b5f5719851b9c7f39add700db3e55ff363d3a3f4
	// 0x2213830F680cD75FeA3AD4D37BBC0E244A412c04
	priv, _ := signature.HexToECDSA(signature.P256, "0xf3bf2f58e4e369d101786024b5f5719851b9c7f39add700db3e55ff363d3a3f4")
	//priv, _ := signature.GenerateKeyWithECDSA(signature.P256)
	privBytes := signature.FromECDSA(priv)
	fmt.Println(hexutil.Encode(privBytes))
	addr := signature.PubkeyToAddress(&priv.PublicKey)
	fmt.Println(addr.Hex())

	mockDbReader := mock.NewMockDatabaseReader(mockCtl)
	mockBlockReader := mock.NewMockBlockReader(mockCtl)

	blockCount := uint64(10)
	blocks := make([]*models.Block, blockCount)
	for i := uint64(0); i < blockCount; i++ {
		parentHash := types.EmptyHash
		if i > 0 {
			parentHash = blocks[i-1].Hash()
		}
		header := &models.Header{
			ParentHash: parentHash,
			Height:     uint64(i),
			StateRoots: nil,
			TxsRoot: []types.Hash{
				types.EmptyHash,
			},
			TxsCount:    0,
			Timestamp:   uint64(dateutil.CurrentTimeSecond()) + uint64(i),
			GasUsed:     0,
			GasLimit:    5000000,
			ArchiveHash: nil,
			LogsBloom:   nil,
			Extra:       nil,
			Memo:        nil,
			Signature:   nil,
		}
		committedSeal := pbftProtocol.PrepareCommittedSeal(header.HashNoSign())
		result, err := signature.SignWithECDSA(priv, committedSeal)
		if err != nil {
			t.Fatal(err)
		}
		consensusData := ConsensusData{
			CommittedSeal: []*signature.SignResult{result},
			Managers:      managersAddr,
			Validators:    validators,
			MVote:         nil,
			VVote:         nil,
		}
		consensusBytes, err := codec.Coder().Encode(consensusData)
		consensus := &models.Consensus{
			Name:      "pbft",
			Consensus: consensusBytes,
		}
		header.Consensus = consensus

		block := models.NewBlock(header, nil, nil)
		bytes, err := codec.Coder().Encode(block)
		if err != nil {
			t.Fatal(err)
		}
		signResult, err := signature.SignWithECDSA(priv, bytes)
		if err != nil {
			t.Fatal(err)
		}
		header.Signature = signResult
		blocks[i] = block.WithSeal(header)
	}

	currentBlock := blocks[0]
	mockBlockReader.EXPECT().CurrentBlock().DoAndReturn(func() *models.Block {
		return currentBlock
	}).AnyTimes()
	mockBlockReader.EXPECT().GetHeaderByNumber(gomock.Any()).DoAndReturn(func(height uint64) *models.Header {
		if height < blockCount {
			return blocks[height].Header()
		}
		return blocks[blockCount-1].Header()
	}).AnyTimes()
	mockBlockReader.EXPECT().GetHeader(gomock.Any(), gomock.Any()).DoAndReturn(func(hash types.Hash, height uint64) *models.Header {
		if height < blockCount {
			return blocks[height].Header()
		}
		return blocks[blockCount-1].Header()
	}).AnyTimes()
	mockNodeKey := mock.NewMockNodeKey(mockCtl)
	mockNodeKey.EXPECT().ID().Return(addr.Hex(), nil).AnyTimes()
	mockNodeKey.EXPECT().Sign(gomock.Any()).DoAndReturn(func(data []byte) (*signature.SignResult, error) {
		return signature.SignWithECDSA(priv, data)
	}).AnyTimes()
	mockNodeKey.EXPECT().RecoverId(gomock.Any(), gomock.Any()).Return(addr.Hex(), nil).AnyTimes()
	mockBroadcaster := mock.NewMockBroadcaster(mockCtl)
	mockBroadcaster.EXPECT().SubscribeMsg(gomock.Any(), gomock.Any()).Return(event.NewSubscription(func(quit <-chan struct{}) error {
		return nil
	})).AnyTimes()

	factory, err := NewConsensus(context.Background(),
		WithConfig(mockConfig),
		WithBlockReader(mockBlockReader),
		WithDatabaseReader(mockDbReader),
		WithNodeKey(mockNodeKey),
		//WithBroadcaster(mockBroadcaster),
		WithKVDB(memorydb.New()),
	)
	if err != nil {
		t.Fatal(err)
	}
	var resultCh = make(chan *models.Block)
	go func() {
		for {
			select {
			case block := <-resultCh:
				if block != nil {
					currentBlock = block
					height := block.Height()
					logger.Info("Insert block to DB", "height", height, "hash", block.Hash())
					if height < blockCount-1 {
						//sendRequest(t, factory, mockBlockReader, blocks[height+1], resultCh)
					} else {
						logger.Info("commit all blocks")
					}
				}
			}
		}
	}()

	if err := factory.Start(); err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i < blockCount; i++ {
		// todo 并发处理还不支持
		sendRequest(t, factory, mockBlockReader, blocks[i], resultCh)
	}
	//sendRequest(t, factory, mockBlockReader, blocks[1], resultCh)
	var stopCh chan struct{}
	<-stopCh
}

func sendRequest(t *testing.T, factory protocol.Consensus, mockBlockReader *mock.MockBlockReader, requestBlock *models.Block, resultCh chan *models.Block) {
	if err := factory.Begin(); err != nil {
		t.Fatal(err)
	}
	if err := factory.Prepare(mockBlockReader, requestBlock.Header()); err != nil {
		t.Fatal(err)
	}
	if err := factory.VerifyHeader(mockBlockReader, requestBlock.Header()); err != nil {
		if err != pbftProtocol.ErrFutureBlock {
			t.Fatal(err)
		}
	}
	block, err := factory.Finalize(mockBlockReader, requestBlock.Header(), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = factory.Seal(context.Background(), mockBlockReader, block, resultCh)
	if err != nil {
		t.Fatal(err)
	}
}
