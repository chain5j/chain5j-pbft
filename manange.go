// Package pbft
//
// @author: xwc1125
package pbft

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pbft/validator"
	"github.com/chain5j/chain5j-pkg/codec/json"
	"github.com/chain5j/chain5j-pkg/types"
	"strings"
	"sync"
)

type manager struct {
	voteLock   sync.RWMutex
	mCandidate map[types.Address]*ManagementVote // management map
	vCandidate map[string]*ValidatorVote         // validator map
}

func newManager() *manager {
	return &manager{
		mCandidate: make(map[types.Address]*ManagementVote),
		vCandidate: make(map[string]*ValidatorVote),
	}
}

func (m *manager) getVotes(height uint64, checkMVote func(addr types.Address, authorize bool) bool, checkVVote func(addr string, authorize bool) bool) (*ManagementVote, *ValidatorVote) {
	var (
		mVote *ManagementVote
		vVote *ValidatorVote
	)
	m.voteLock.Lock()
	defer m.voteLock.Unlock()
	for address, vote := range m.mCandidate {
		if checkMVote(address, vote.Authorize) && vote.DeadlineHeight.Uint64() > height {
			mVote = vote
			break
		} else {
			delete(m.mCandidate, address)
		}
	}
	for id, vote := range m.vCandidate {
		if checkVVote(id, vote.Authorize) && vote.DeadlineHeight.Uint64() > height {
			vVote = vote
			break
		} else {
			delete(m.vCandidate, id)
		}
	}
	return mVote, vVote
}

func (m *manager) setMVote(addr types.Address, vote *ManagementVote) {
	m.voteLock.Lock()
	defer m.voteLock.Unlock()
	m.mCandidate[addr] = vote
}

func (m *manager) setVVote(val string, vote *ValidatorVote) {
	m.voteLock.Lock()
	defer m.voteLock.Unlock()
	m.vCandidate[val] = vote
}

type voteSnapshot struct {
	ValSet   pbftProtocol.ValidatorSet  // 快照对应的验证者集合
	Managers map[types.Address]struct{} // 管理员地址

	vVotes []*ValidatorVote  // 节点验证者的投票
	mVotes []*ManagementVote // 管理员的投票

	vTally map[string]Tally        // validator投票情况，id==>统计
	mTally map[types.Address]Tally // management投票情况，addr==>统计
}

func newVoteSnapshot(valSet pbftProtocol.ValidatorSet, managers []types.Address) *voteSnapshot {
	snap := &voteSnapshot{
		ValSet:   valSet,
		Managers: make(map[types.Address]struct{}),
	}
	for _, address := range managers {
		snap.Managers[address] = struct{}{}
	}
	return snap
}

func (s *voteSnapshot) isManager(addr types.Address) bool {
	_, ok := s.Managers[addr]
	return ok
}

// checkMVote 判断地址是否可被投票
func (s *voteSnapshot) checkMVote(address types.Address, authorize bool) bool {
	_, ok := s.Managers[address]
	// 1）快照中已经处在，但是处于无权限的时候
	// 2）快照中不存在，但是需要授权的时候
	return (ok && !authorize) || (!ok && authorize)
}

// applyMVotes 处理管理员投票
func (s *voteSnapshot) applyMVotes(data *ConsensusData) {
	mVote := data.MVote
	if mVote == nil {
		return
	}

	candidate := mVote.Candidate // 候选人
	manager := mVote.Manager     // 管理员

	// 如果管理员对于候选人的投票已经投过了，那么就先将其投票删除，然后重新投票
	for i, vote := range s.mVotes {
		if vote.Manager == manager && vote.Candidate == candidate {
			// 如果管理员和候选人是一样的，那么减少一次投票
			s.uncastM(vote.Candidate, vote.Authorize)

			s.mVotes = append(s.mVotes[:i], s.mVotes[i+1:]...)
			break // 只能投一次票
		}
	}

	// 添加一次投票
	if s.castM(candidate, mVote.Authorize) {
		s.mVotes = append(s.mVotes, mVote)
	}

	// 如果关于候选人的投票数超过了1/2，那么就更新
	if tally := s.mTally[candidate]; tally.Votes > len(s.Managers)/2 {
		if tally.Authorize {
			// 授权，那么直接添加
			s.Managers[candidate] = struct{}{}
		} else {
			// 剔除，那么需要删除
			delete(s.Managers, candidate)

			// 剔除，那么需要删除该候选人的所有投票
			// Discard any previous votes the deauthorized validator cast
			for i := 0; i < len(s.mVotes); i++ {
				if s.mVotes[i].Candidate == candidate {
					// 删除M的投票
					s.uncastM(s.mVotes[i].Candidate, s.mVotes[i].Authorize)

					s.mVotes = append(s.mVotes[:i], s.mVotes[i+1:]...)
					i--
				}
			}
		}
		//// Discard any previous votes around the just changed account
		//for i := 0; i < len(s.mVotes); i++ {
		//	if s.mVotes[i].Candidate == candidate {
		//		s.mVotes = append(s.mVotes[:i], s.mVotes[i+1:]...)
		//		i--
		//	}
		//}
		delete(s.mTally, candidate)
	}
}

// cast 添加新投票到统计中，即投票数+1
func (s *voteSnapshot) castM(address types.Address, authorize bool) bool {
	// 确定id可被投票
	if !s.checkMVote(address, authorize) {
		return false
	}
	if old, ok := s.mTally[address]; ok {
		// 已经有投票，投票数+1
		old.Votes++
		s.mTally[address] = old
	} else {
		// 创建新的投票
		s.mTally[address] = Tally{Authorize: authorize, Votes: 1}
	}
	return true
}

// uncastM 从投票统计中删除id之前的投票，即投票数-1
func (s *voteSnapshot) uncastM(address types.Address, authorize bool) bool {
	// 如果address的投票统计中没有，那么无需操作
	tally, ok := s.mTally[address]
	if !ok {
		return false
	}
	// 如果授权的权限与原权限不一致，那么也无需处理
	if tally.Authorize != authorize {
		return false
	}
	if tally.Votes > 1 {
		// 减少一次投票
		tally.Votes--
		s.mTally[address] = tally
	} else {
		// 刚好只有一个，那么就删除，相当于投票为0
		delete(s.mTally, address)
	}
	return true
}

// checkVVote 判断id是否可被投票
func (s *voteSnapshot) checkVVote(id string, authorize bool) bool {
	_, validator := s.ValSet.GetById(id)
	// 1）快照中已经处在，但是处于无权限的时候
	// 2）快照中不存在，但是需要授权的时候
	return (validator != nil && !authorize) || (validator == nil && authorize)
}

// applyVVotes 处理节点投票
func (s *voteSnapshot) applyVVotes(data *ConsensusData) {
	vVote := data.VVote
	if vVote == nil {
		return
	}

	validator := vVote.Validator
	manager := vVote.Manager

	// 如果管理员对于候选人的投票已经投过了，那么就先将其投票删除，然后重新投票
	for i, vote := range s.vVotes {
		if vote.Manager == manager && vote.Validator == validator {
			s.uncastV(vote.Validator, vote.Authorize)

			s.mVotes = append(s.mVotes[:i], s.mVotes[i+1:]...)
			break // 只能投一次票
		}
	}

	// 添加一次投票
	if s.castV(validator, vVote.Authorize) {
		s.vVotes = append(s.vVotes, vVote)
	}

	// 如果关于validator的投票数超过了1/2，那么就更新
	if tally := s.vTally[validator]; tally.Votes > s.ValSet.Size()/2 {
		if tally.Authorize {
			// 授权，那么直接添加
			s.ValSet.AddValidator(validator)
		} else {
			// 剔除，那么需要删除
			s.ValSet.RemoveValidator(validator)

			// 剔除，那么需要删除该validator的所有投票
			// Discard any previous votes the deauthorized validator cast
			for i := 0; i < len(s.vVotes); i++ {
				if s.vVotes[i].Validator == validator {
					// 删除M的投票
					s.uncastV(validator, tally.Authorize)

					s.vVotes = append(s.vVotes[:i], s.vVotes[i+1:]...)
					i--
				}
			}
		}
		//// Discard any previous votes around the just changed account
		//for i := 0; i < len(s.mVotes); i++ {
		//	if s.vVotes[i].Validator == validator {
		//		s.vVotes = append(s.vVotes[:i], s.vVotes[i+1:]...)
		//		i--
		//	}
		//}
		delete(s.vTally, validator)
	}
}

// castV 添加新投票到统计中，即投票数+1
func (s *voteSnapshot) castV(id string, authorize bool) bool {
	// 确定id可被投票
	if !s.checkVVote(id, authorize) {
		return false
	}
	if old, ok := s.vTally[id]; ok {
		// 已经有投票，投票数+1
		old.Votes++
		s.vTally[id] = old
	} else {
		// 创建新的投票
		s.vTally[id] = Tally{Authorize: authorize, Votes: 1}
	}
	return true
}

// uncastV 从投票统计中删除id之前的投票，即投票数-1
func (s *voteSnapshot) uncastV(id string, authorize bool) bool {
	// 如果id的投票统计中没有，那么无需操作
	tally, ok := s.vTally[id]
	if !ok {
		return false
	}
	// 如果授权的权限与原权限不一致，那么也无需处理
	if tally.Authorize != authorize {
		return false
	}
	// 否则，恢复表决
	if tally.Votes > 1 {
		// 减少一次投票
		tally.Votes--
		s.vTally[id] = tally
	} else {
		// 刚好只有一个，那么就删除，相当于投票为0
		delete(s.vTally, id)
	}
	return true
}

func (s *voteSnapshot) reset() {
	s.mVotes = nil
	s.vVotes = nil
	s.mTally = make(map[types.Address]Tally)
	s.vTally = make(map[string]Tally)
}

func (s *voteSnapshot) copy() *voteSnapshot {
	cpy := &voteSnapshot{
		ValSet:   s.ValSet.Copy(),
		Managers: make(map[types.Address]struct{}),
		vVotes:   make([]*ValidatorVote, len(s.vVotes)),
		mVotes:   make([]*ManagementVote, len(s.mVotes)),
		vTally:   make(map[string]Tally),
		mTally:   make(map[types.Address]Tally),
	}
	for address := range s.Managers {
		cpy.Managers[address] = struct{}{}
	}
	copy(cpy.vVotes, s.vVotes)
	copy(cpy.mVotes, s.mVotes)

	for id, tally := range s.vTally {
		cpy.vTally[id] = tally
	}

	for address, tally := range s.mTally {
		cpy.mTally[address] = tally
	}
	return cpy
}

type extVoteSnapshot struct {
	Validators []string                   `json:"validators,omitempty" rlp:"nil"`
	Managers   map[types.Address]struct{} `json:"managers,omitempty rlp:"nil""`

	VVotes []*ValidatorVote  `json:"v_votes,omitempty" rlp:"nil"`
	MVotes []*ManagementVote `json:"m_votes,omitempty" rlp:"nil"`

	MTally map[types.Address]Tally `json:"m_tally,omitempty" rlp:"nil"`
	VTally map[string]Tally        `json:"v_tally,omitempty" rlp:"nil"`

	Policy pbftProtocol.ProposerPolicy `json:"policy"`
}

func (s *voteSnapshot) toJSONStruct() *extVoteSnapshot {
	return &extVoteSnapshot{
		Validators: s.validators(),
		Managers:   s.Managers,
		VVotes:     s.vVotes,
		MVotes:     s.mVotes,
		MTally:     s.mTally,
		VTally:     s.vTally,
		Policy:     s.ValSet.Policy(),
	}
}

// validators 获取排序后的validator的ID数组
func (s *voteSnapshot) validators() []string {
	validators := make([]string, 0, s.ValSet.Size())
	for _, validator := range s.ValSet.List() {
		validators = append(validators, validator.ID())
	}
	// 排序
	for i := 0; i < len(validators); i++ {
		for j := i + 1; j < len(validators); j++ {
			if strings.Compare(validators[i], validators[j]) > 0 {
				validators[i], validators[j] = validators[j], validators[i]
			}
		}
	}

	return validators
}

// UnmarshalJSON 反序列化
func (s *voteSnapshot) UnmarshalJSON(b []byte) error {
	var j extVoteSnapshot
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	s.Managers = j.Managers
	s.vVotes = j.VVotes
	s.mVotes = j.MVotes
	s.mTally = j.MTally
	s.vTally = j.VTally
	s.ValSet = validator.NewSet(j.Validators, j.Policy)

	return nil
}

// MarshalJSON 序列化
func (s *voteSnapshot) MarshalJSON() ([]byte, error) {
	j := s.toJSONStruct()
	return json.Marshal(j)
}
