// Package core
//
// @author: xwc1125
package core

import (
	"github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/codec/rlp"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-pkg/util/hexutil"
	"io"
	"math/big"
	"sync"
)

// roundState 共识状态
type roundState struct {
	round          *big.Int             `json:"round"`           // 轮换次数
	sequence       uint64               `json:"sequence"`        // 序列号
	PrePrepare     *protocol.PrePrepare `json:"pre_prepare"`     // preprepare消息
	Prepares       *protocol.MessageSet `json:"prepares"`        // prepare消息集
	Commits        *protocol.MessageSet `json:"commits"`         // commit消息集
	lockedHash     types.Hash           `json:"locked_hash"`     // 锁定的hash
	pendingRequest *protocol.Request    `json:"pending_request"` // 处于pending的请求
	mu             *sync.RWMutex
}

// newRoundState 创建新的state
func newRoundState(view *protocol.View, validatorSet protocol.ValidatorSet, lockedHash types.Hash, prePrepare *protocol.PrePrepare, pendingRequest *protocol.Request) *roundState {
	return &roundState{
		round:          view.Round,
		sequence:       view.Sequence.Uint64(),
		PrePrepare:     prePrepare,
		Prepares:       protocol.NewMessageSet(validatorSet),
		Commits:        protocol.NewMessageSet(validatorSet),
		lockedHash:     lockedHash,
		mu:             new(sync.RWMutex),
		pendingRequest: pendingRequest,
	}
}

// GetPrepareOrCommitSize 获取prepare和commit的总size。去掉重复部分
func (s *roundState) GetPrepareOrCommitSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := s.Prepares.Size() + s.Commits.Size()

	// 去掉重复数据
	for _, m := range s.Prepares.Values() {
		if s.Commits.Get(m.Validator) != nil {
			result--
		}
	}
	return result
}

// Subject 主题。主要是prepare和commit
func (s *roundState) Subject() *protocol.Subject {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.PrePrepare == nil {
		return nil
	}

	return &protocol.Subject{
		View: &protocol.View{
			Sequence: big.NewInt(int64(s.sequence)),
			Round:    new(big.Int).Set(s.round),
		},
		Digest: s.PrePrepare.Proposal.Hash(),
	}
}

// SetPrePrepare 设置pre_prepare状态
func (s *roundState) SetPrePrepare(prePrepare *protocol.PrePrepare) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.PrePrepare = prePrepare
}
func (s *roundState) Proposal() protocol.Proposal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.PrePrepare != nil {
		return s.PrePrepare.Proposal
	}

	return nil
}
func (s *roundState) SetRound(r *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.round = new(big.Int).Set(r)
}
func (s *roundState) Round() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.round
}
func (s *roundState) SetSequence(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence = seq
}
func (s *roundState) Sequence() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sequence
}

// LockHash 锁定hash
func (s *roundState) LockHash() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// prePrepare不为空时，锁定提议hash
	if s.PrePrepare != nil {
		s.lockedHash = s.PrePrepare.Proposal.Hash()
	}
}

// UnlockHash 解锁hash，将lockedHash设置为空
func (s *roundState) UnlockHash() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lockedHash = types.Hash{}
}

// IsHashLocked 判断hash是否被锁定
func (s *roundState) IsHashLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.lockedHash.Nil() {
		return false
	}

	return true
}

// GetLockedHash 获取提议hash
func (s *roundState) GetLockedHash() types.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.lockedHash
}

// DecodeRLP rlp反编码
func (s *roundState) DecodeRLP(stream *rlp.Stream) error {
	var ss struct {
		Round          *big.Int
		Sequence       hexutil.Uint64
		PrePrepare     *protocol.PrePrepare
		Prepares       *protocol.MessageSet
		Commits        *protocol.MessageSet
		lockedHash     types.Hash
		pendingRequest *protocol.Request
	}

	if err := stream.Decode(&ss); err != nil {
		return err
	}
	s.round = ss.Round
	s.sequence = uint64(ss.Sequence)
	s.PrePrepare = ss.PrePrepare
	s.Prepares = ss.Prepares
	s.Commits = ss.Commits
	s.lockedHash = ss.lockedHash
	s.pendingRequest = ss.pendingRequest
	s.mu = new(sync.RWMutex)

	return nil
}

// EncodeRLP rlp编码
func (s *roundState) EncodeRLP(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return rlp.Encode(w, []interface{}{
		s.round,
		s.sequence,
		s.PrePrepare,
		s.Prepares,
		s.Commits,
		s.lockedHash,
		s.pendingRequest,
	})
}
