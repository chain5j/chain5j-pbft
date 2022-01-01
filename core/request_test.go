// Package core
//
// @author: xwc1125
package core

import (
	"fmt"
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pbft/protocol/mockpbft"
	"github.com/chain5j/chain5j-pbft/validator"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/event"
	cmath "github.com/chain5j/chain5j-pkg/math"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-pkg/util/dateutil"
	"github.com/chain5j/chain5j-pkg/util/hexutil"
	"github.com/chain5j/chain5j-protocol/mock"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/logger"
	"github.com/chain5j/logger/zap"
	"github.com/golang/mock/gomock"
	"math/big"
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

func TestHandleRequest(t *testing.T) {
	mockCtl := gomock.NewController(t)

	mockPBFTBackend := mockpbft.NewMockPBFTBackend(mockCtl)
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
	data := hexutil.MustDecode("0xf8b980b889f887c28001f882f87fa000000000000000000000000000000000000000000000000000000000000000000180a0000000000000000000000000000000000000000000000000000000000000000080846169273f80834c4b40c080a00000000000000000000000000000000000000000000000000000000000000000808800000000000000008080c0c0aa307837433238366435373265434431353766323330444633463139413746313237414144463337414463c080")
	signResult, err := signature.SignWithECDSA(priv, data)
	if err != nil {
		t.Fatal(err)
	}
	mockPBFTBackend.EXPECT().ID().Return("0x2213830F680cD75FeA3AD4D37BBC0E244A412c04").AnyTimes()
	mockPBFTBackend.EXPECT().Sign(gomock.Any()).Return(
		signResult,
		nil,
	).AnyTimes()

	blockCount := 5
	blocks := make([]*models.Block, blockCount)
	for i := 0; i < blockCount; i++ {
		header := &models.Header{
			ParentHash:  types.EmptyHash,
			Height:      uint64(i),
			StateRoots:  nil,
			TxRoot:      types.EmptyHash,
			TxCount:     0,
			Timestamp:   uint64(dateutil.CurrentTimeSecond()),
			GasUsed:     0,
			GasLimit:    5000000,
			Consensus:   nil,
			ArchiveHash: nil,
			LogsBloom:   nil,
			Extra:       nil,
			Memo:        nil,
			Signature:   nil,
		}
		blocks[i] = models.NewBlock(header, nil, nil)
	}
	newView := &pbftProtocol.View{
		Sequence: new(big.Int).Add(big.NewInt(int64(blocks[0].Height())), cmath.Big1),
		Round:    new(big.Int),
	}
	valSet := validator.NewSet([]string{
		"0x2213830F680cD75FeA3AD4D37BBC0E244A412c04",
		//"0x7C286d572eCD157f230DF3F19A7F127AADF37ADc",
	}, pbftProtocol.RoundRobin)
	mockPBFTBackend.EXPECT().Validators(gomock.Any()).Return(valSet).AnyTimes()
	mockPBFTBackend.EXPECT().Verify(gomock.Any()).Return(nil).AnyTimes()
	mockPBFTBackend.EXPECT().Commit(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockPBFTBackend.EXPECT().LastProposal().DoAndReturn(func() (pbftProtocol.Proposal, string) {
		return blocks[0], "0x2213830F680cD75FeA3AD4D37BBC0E244A412c04"
	}).AnyTimes()
	// 设置调用顺序: gomock.InOrder()
	// Broadcaster
	mockBroadcaster := mock.NewMockBroadcaster(mockCtl)
	mockBroadcaster.EXPECT().SubscribeMsg(gomock.Any(), gomock.Any()).Return(event.NewSubscription(func(quit <-chan struct{}) error {
		return nil
	})).AnyTimes()
	mockBroadcaster.EXPECT().Broadcast(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	engine := NewEngine(mockPBFTBackend, mockBroadcaster)
	engineCore := engine.(*core)

	engineCore.currentRoundState = newRoundState(newView, valSet, types.Hash{}, nil, nil)
	engineCore.valSet = valSet
	go engineCore.handleLoop()
	{
		// oldMessage
		err := engine.Request(&pbftProtocol.Request{
			Proposal: blocks[0],
		})
		if err != nil {
			t.Error(err)
		}
	}
	for i := uint64(1); i < uint64(5); i++ {
		{
			err := engine.Request(&pbftProtocol.Request{
				Proposal: blocks[i],
			})
			if err != nil {
				t.Error(err)
			}
		}
	}
	var stop chan struct{}
	<-stop
}
