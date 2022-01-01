// Package core
//
// @author: xwc1125
package core

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/codec"
	cmath "github.com/chain5j/chain5j-pkg/math"
	"github.com/chain5j/chain5j-pkg/types"
	"math"
	"math/big"
	"sync"
	"time"
)

// StartNewRound 开启一个新的轮换
// 如果round=0，代表开启一个新的序列
func (c *core) StartNewRound(round *big.Int) {
	roundChange := false
	// 获取最新的提议
	lastProposal, lastProposer := c.backend.LastProposal()
	if c.currentRoundState == nil {
		c.log.Trace("start to the initial round")
	} else if lastProposal == nil {
		c.log.Error("last proposal is nil")
		return
	} else if lastProposal.Height() >= c.currentRoundState.Sequence() {
		if !c.consensusTimestamp.IsZero() {
			c.consensusTimestamp = time.Time{}
		}
		c.log.Trace("catch up latest proposal", "height", lastProposal.Height(), "hash", lastProposal.Hash())
	} else if lastProposal.Height() == uint64(c.currentRoundState.Sequence()-1) {
		if round.Cmp(cmath.Big0) == 0 {
			// 如果sequence相等，并且round为0，那么无需开启新的round
			return
		} else if round.Cmp(c.currentRoundState.Round()) < 0 {
			c.log.Warn("new round should not be smaller than currentRoundState round", "seq", lastProposal.Height(), "new_round", round, "old_round", c.currentRoundState.Round())
			return
		}
		roundChange = true
	} else {
		c.log.Warn("new sequence should be larger than currentRoundState sequence", "new_seq", lastProposal.Height())
		return
	}

	var newView *pbftProtocol.View
	if roundChange {
		newView = &pbftProtocol.View{
			Sequence: big.NewInt(int64(c.currentRoundState.Sequence())),
			Round:    new(big.Int).Set(round),
		}
	} else {
		newView = &pbftProtocol.View{
			Sequence: new(big.Int).Add(big.NewInt(int64(lastProposal.Height())), cmath.Big1),
			Round:    new(big.Int),
		}
		c.valSet = c.backend.Validators(lastProposal)
	}

	// 清理无效的轮换消息
	c.roundChangeSet = newRoundChangeSet(c.valSet)
	// 为新轮换创建快照
	c.updateRoundState(newView, c.valSet, roundChange)
	// 计算提案者
	c.valSet.CalcProposer(lastProposer, newView.Round.Uint64())
	c.waitingForRoundChange.Store(false)
	c.setState(pbftProtocol.StateAcceptRequest) // 状态接收请求
	if roundChange && c.isProposer() && c.currentRoundState != nil {
		// If it is locked, propose the old proposal
		// If we have pending request, propose pending request
		if c.currentRoundState.IsHashLocked() {
			r := &pbftProtocol.Request{
				Proposal: c.currentRoundState.Proposal(), //c.currentRoundState.Proposal would be the locked proposal by previous proposer, see updateRoundState
			}
			c.SendPrePrepare(r)
		} else if c.currentRoundState.pendingRequest != nil {
			c.SendPrePrepare(c.currentRoundState.pendingRequest)
		}
	}
	c.newRoundChangeTimer()

	c.log.Debug("new round", "new_seq", newView.Sequence, "new_round", newView.Round, "new_proposer", c.valSet.GetProposer(), "valSet", c.valSet.List(), "size", c.valSet.Size(), "isProposer", c.isProposer())
}

// catchUpRound 碰到轮换
func (c *core) catchUpRound(view *pbftProtocol.View) {
	c.waitingForRoundChange.Store(true) // 设置为等待轮换

	c.updateRoundState(view, c.valSet, true) // 更新轮换状态
	c.roundChangeSet.Clear(view.Round)
	c.newRoundChangeTimer()

	c.log.Trace("catch up round", "new_seq", view.Sequence, "new_round", view.Round, "new_proposer", c.valSet.GetProposer())
}

// updateRoundState 更新轮换状态。
func (c *core) updateRoundState(view *pbftProtocol.View, validatorSet pbftProtocol.ValidatorSet, roundChange bool) {
	if roundChange && c.currentRoundState != nil {
		// 如果当前状态已经锁定，更新数据
		if c.currentRoundState.IsHashLocked() {
			c.currentRoundState = newRoundState(view, validatorSet, c.currentRoundState.GetLockedHash(), c.currentRoundState.PrePrepare, c.currentRoundState.pendingRequest)
		} else {
			// 创建未锁定状态
			c.currentRoundState = newRoundState(view, validatorSet, types.Hash{}, nil, c.currentRoundState.pendingRequest)
		}
	} else {
		c.currentRoundState = newRoundState(view, validatorSet, types.Hash{}, nil, nil)
	}
}

// stopTimer 停止轮换的计时器
func (c *core) stopTimer() {
	if c.roundChangeTimer != nil {
		c.roundChangeTimer.Stop()
	}
}

// newRoundChangeTimer 创建新的轮换计时器
func (c *core) newRoundChangeTimer() {
	c.stopTimer()

	//var timeout time.Duration
	// set timeout based on the round number
	timeout := roundChangeTimeout

	if c.waitingForRoundChange.Load() {
		timeout = initRoundTimeout
	}

	round := c.currentRoundState.Round().Uint64()
	if round > 0 {
		timeout += time.Duration(math.Pow(2, float64(round))) * time.Second
	}

	c.roundChangeTimer = time.AfterFunc(timeout, func() {
		c.sendTimeoutEvent()
	})
}

// sendNextRoundChange 开启下一个轮换 round + 1
func (c *core) sendNextRoundChange() {
	cv := c.currentView()
	c.log.Trace("send next round change", "sequence", cv.Sequence, "round", cv.Round)
	c.sendRoundChange(new(big.Int).Add(cv.Round, big.NewInt(1)))
}

// sendRoundChange 设置轮换次数
func (c *core) sendRoundChange(round *big.Int) {
	cv := c.currentView()
	if cv.Round.Cmp(round) >= 0 {
		c.log.Error("Cannot send out the round change", "current round", cv.Round, "target round", round)
		return
	}

	c.catchUpRound(&pbftProtocol.View{
		Round:    new(big.Int).Set(round),
		Sequence: new(big.Int).Set(cv.Sequence),
	})

	cv = c.currentView()
	rc := &pbftProtocol.Subject{
		View:   cv,
		Digest: types.Hash{},
	}

	payload, err := codec.Coder().Encode(rc)
	if err != nil {
		c.log.Error("Failed to encode ROUND CHANGE", "rc", rc, "err", err)
		return
	}
	c.log.Debug("broadcast round change", "sequence", rc.View.Sequence, "round", round)
	c.Broadcast(&pbftProtocol.Message{
		Code: pbftProtocol.MsgRoundChange,
		Msg:  payload,
	})
}

// HandleRoundChange 处理round变更处理
func (c *core) HandleRoundChange(msg *pbftProtocol.Message, val pbftProtocol.Validator) error {
	var rc *pbftProtocol.Subject
	if err := msg.Decode(&rc); err != nil {
		c.log.Error("Failed to decode ROUND CHANGE", "err", err)
		return errInvalidMessage
	}

	if err := c.checkMessage(pbftProtocol.MsgRoundChange, rc.View); err != nil {
		return err
	}

	cv := c.currentView()
	roundView := rc.View

	c.log.Trace("handle round change", "waitingForRoundChange", c.waitingForRoundChange.Load(), "current", c.currentView(), "recv", roundView)
	// 将轮换消息添加进轮换变更集合中，并返回其对应的消息个数
	num, err := c.roundChangeSet.Add(roundView.Round, msg)
	if err != nil {
		c.log.Warn("Failed to add round change Message", "from", val, "msg", msg, "err", err)
		return err
	}

	// 当轮换消息达到门限，并且，状态为等待轮换或是当前的轮换次数小于传入消息的轮换次数时，开启新的轮换
	if c.reachRoundChangeThreshold(num) && (c.waitingForRoundChange.Load() || cv.Round.Cmp(roundView.Round) < 0) {
		// 轮换消息达到2f+1时，开启新的轮换
		c.StartNewRound(roundView.Round)
		return nil
	} else if c.waitingForRoundChange.Load() && num == int(c.valSet.FaultTolerantNum()+1) {
		// 等待轮换并且，轮换信息个数等于容错数
		if cv.Round.Cmp(roundView.Round) < 0 {
			c.log.Trace("handle round change, send round change", "round", roundView.Round)
			c.sendRoundChange(roundView.Round)
		}
		return nil
	} else if cv.Round.Cmp(roundView.Round) < 0 {
		// 将当前的message广播给其他验证者
		return errIgnored
	}
	return nil
}

// reachRoundChangeThreshold 是否达到轮换门限
func (c *core) reachRoundChangeThreshold(num int) bool {
	return reachThreshold(c.valSet, num)
}

// ----------------------------------------------------------------------------

// roundChangeSet 轮换变更集
type roundChangeSet struct {
	validatorSet pbftProtocol.ValidatorSet
	roundChanges map[uint64]*pbftProtocol.MessageSet
	mu           *sync.Mutex
}

// newRoundChangeSet 创建新的轮换变更集
func newRoundChangeSet(valSet pbftProtocol.ValidatorSet) *roundChangeSet {
	return &roundChangeSet{
		validatorSet: valSet,
		roundChanges: make(map[uint64]*pbftProtocol.MessageSet),
		mu:           new(sync.Mutex),
	}
}

// Add 将round和message添加map中，并返回message的个数
func (rcs *roundChangeSet) Add(r *big.Int, msg *pbftProtocol.Message) (int, error) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := r.Uint64()
	if rcs.roundChanges[round] == nil {
		rcs.roundChanges[round] = pbftProtocol.NewMessageSet(rcs.validatorSet)
	}
	err := rcs.roundChanges[round].Add(msg)
	if err != nil {
		return 0, err
	}
	return rcs.roundChanges[round].Size(), nil
}

// Clear 删除小于round的所有消息
func (rcs *roundChangeSet) Clear(round *big.Int) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	for k, rms := range rcs.roundChanges {
		if len(rms.Values()) == 0 || k < round.Uint64() {
			delete(rcs.roundChanges, k)
		}
	}
}

// MaxRound 获取大于等于num的消息个数的最大round
func (rcs *roundChangeSet) MaxRound(num int) *big.Int {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	var maxRound *big.Int
	for k, rms := range rcs.roundChanges {
		if rms.Size() < num {
			continue
		}
		r := big.NewInt(int64(k))
		if maxRound == nil || maxRound.Cmp(r) < 0 {
			maxRound = r
		}
	}
	return maxRound
}
