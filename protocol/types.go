// Package protocol
//
// @author: xwc1125
package protocol

import (
	"fmt"
	"github.com/chain5j/chain5j-pkg/codec/rlp"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-protocol/models"
	"io"
	"math/big"
)

// View 轮询次数及区块高度
type View struct {
	Sequence *big.Int // 提议的区块高度。每个周期都有一个高度。周期为prePrepare, prepare and commit
	Round    *big.Int // 轮询次数。如果给定的区块未被验证器接受，将发生新一轮更改，并且Round=Round+1
}

func (v *View) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{v.Round, v.Sequence})
}
func (v *View) DecodeRLP(s *rlp.Stream) error {
	var view struct {
		Round    *big.Int
		Sequence *big.Int
	}

	if err := s.Decode(&view); err != nil {
		return err
	}
	v.Round, v.Sequence = view.Round, view.Sequence
	return nil
}
func (v *View) String() string {
	return fmt.Sprintf("{Sequence: %d, Round: %d}", v.Sequence.Uint64(), v.Round.Uint64())
}
func (v *View) Cmp(y *View) int {
	if v.Sequence.Cmp(y.Sequence) != 0 {
		return v.Sequence.Cmp(y.Sequence)
	}
	if v.Round.Cmp(y.Round) != 0 {
		return v.Round.Cmp(y.Round)
	}
	return 0
}

// Request 请求
type Request struct {
	Proposal Proposal
}

// PrePrepare 预处理消息
type PrePrepare struct {
	View     *View
	Proposal Proposal
}

func (b *PrePrepare) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		b.View,
		b.Proposal.(*models.Block),
	})
}
func (b *PrePrepare) DecodeRLP(s *rlp.Stream) error {
	var prePrepare struct {
		View     *View
		Proposal *models.Block
	}

	if err := s.Decode(&prePrepare); err != nil {
		return err
	}
	b.View, b.Proposal = prePrepare.View, prePrepare.Proposal

	return nil
}

// Subject prepare消息和commit消息
type Subject struct {
	View   *View      // 视图
	Digest types.Hash // 摘要（使用的区块hash）
}

func (b *Subject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.View, b.Digest})
}
func (b *Subject) DecodeRLP(s *rlp.Stream) error {
	var subject struct {
		View   *View
		Digest types.Hash
	}

	if err := s.Decode(&subject); err != nil {
		return err
	}
	b.View, b.Digest = subject.View, subject.Digest
	return nil
}
func (b *Subject) String() string {
	return fmt.Sprintf("{View: %v, Digest: %v}", b.View, b.Digest.String())
}

// State pbft共识状态
type State uint64

const (
	StateAcceptRequest State = iota // 接收请求
	StatePrePrepared                // 预处理
	StatePrepared                   // 准备
	StateCommitted                  // 确认
)

// String 字符串打印
func (s State) String() string {
	if s == StateAcceptRequest {
		return "Accept request"
	} else if s == StatePrePrepared {
		return "PrePrepared"
	} else if s == StatePrepared {
		return "Prepared"
	} else if s == StateCommitted {
		return "Committed"
	} else {
		return "Unknown"
	}
}

// Cmp 状态比较
//   -1 s状态先于y
//    0 状态一致
//   +1 s状态后于y
func (s State) Cmp(y State) int {
	if uint64(s) < uint64(y) {
		return -1
	}
	if uint64(s) > uint64(y) {
		return 1
	}
	return 0
}
