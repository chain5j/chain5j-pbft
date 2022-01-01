// Package core
//
// @author: xwc1125
package core

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/codec"
	"time"
)

// SendPrePrepare 进行prePrepare的发送
func (c *core) SendPrePrepare(request *pbftProtocol.Request) {
	c.log.Debug("prePrepare-1) send pre-prepare request", "hash", request.Proposal.Hash(), "height", request.Proposal.Height(), "isProposer", c.isProposer(), "state", c.state, "sequence", c.currentRoundState.Sequence())
	// 如果当前peer为proposer并且高度和sequence是一致的，那么就进行消息发送
	if c.currentRoundState.Sequence() == request.Proposal.Height() && c.isProposer() {
		curView := c.currentView()
		prePrepareBytes, err := codec.Coder().Encode(
			&pbftProtocol.PrePrepare{
				View:     curView,
				Proposal: request.Proposal,
			})
		if err != nil {
			c.log.Error("failed to encode pre_prepare", "state", c.state, "view", curView, "proposal.Height", request.Proposal.Height(), "proposal.Hash", request.Proposal.Hash())
			return
		}

		// 广播消息
		c.Broadcast(&pbftProtocol.Message{
			Code: pbftProtocol.MsgPrePrepare, // prePrepare类型
			Msg:  prePrepareBytes,            // 主消息内容
		})
	}
}

// HandlePrePrepare 处理接收到的prePrepare数据
func (c *core) HandlePrePrepare(msg *pbftProtocol.Message, val pbftProtocol.Validator) error {
	// 解析消息
	var prePrepare *pbftProtocol.PrePrepare
	err := msg.Decode(&prePrepare)
	if err != nil {
		c.log.Error("prePrepare-2) decode pre_prepare msg err", "err", err)
		return errFailedDecodePrePrepare
	}
	c.log.Debug("prePrepare-2) handle pre-prepare message", "hash", prePrepare.Proposal.Hash(), "sequence", prePrepare.View.Sequence, "round", prePrepare.View.Round)

	// 消息内容的检测，确保一定为prePrepare消息
	if err := c.checkMessage(pbftProtocol.MsgPrePrepare, prePrepare.View); err != nil {
		if err == pbftProtocol.ErrOldMessage {
			// 根据proposal获取validatorSet
			valSet := c.backend.ParentValidators(prePrepare.Proposal).Copy()
			// 获取前一个proposer
			previousProposer := c.backend.GetProposer(prePrepare.Proposal.Height() - 1)
			// 根据前一个Proposer计算新的Proposer
			valSet.CalcProposer(previousProposer, prePrepare.View.Round.Uint64())
			// 如果block已经存在，那么需要发送commit
			// 1. The proposer needs to be a proposer matches the given (Sequence + Round)
			// 2. The given block must exist
			if valSet.IsProposer(val.ID()) && // 当前peer为proposer
				c.backend.HasProposal(prePrepare.Proposal.Hash(), prePrepare.Proposal.Height()) { // 前一个区块已经存在
				// 发送旧的区块
				c.sendCommitForOldBlock(prePrepare.View, prePrepare.Proposal.Hash())
				return nil
			}
		}
		c.log.Error("prePrepare-2) handle pre-prepare message err", "hash", prePrepare.Proposal.Hash(), "sequence", prePrepare.View.Sequence, "round", prePrepare.View.Round, "err", err)
		return err
	}

	// 检查消息是否来自提案者
	if !c.valSet.IsProposer(val.ID()) {
		c.log.Warn("prePrepare-2) ignore pre_prepare messages from non-proposer", "hash", prePrepare.Proposal.Hash())
		return errNotFromProposer
	}

	// 验证提案
	if err := c.backend.Verify(prePrepare.Proposal); err != nil {
		c.log.Warn("prePrepare-2) failed to verify proposal", "err", err)

		//c.sendNextRoundChange()
		return err
	}

	// 关于pre_prepare的处理
	if c.state == pbftProtocol.StateAcceptRequest {
		// 如果hash已锁定
		if c.currentRoundState.IsHashLocked() {
			if prePrepare.Proposal.Hash() == c.currentRoundState.GetLockedHash() {
				// 广播commit，并且直接进入prepare阶段
				c.acceptPrePrepare(prePrepare)
				c.setState(pbftProtocol.StatePrepared)
				c.SendCommit()
			} else {
				// 提案hash不一致时，发送轮换消息
				c.sendNextRoundChange()
			}
		} else {
			// hash没有被锁定
			//   1. the locked proposal and the received proposal match
			//   2. we have no locked proposal
			c.acceptPrePrepare(prePrepare)            // 接受预处理
			c.setState(pbftProtocol.StatePrePrepared) // 更改状态
			c.SendPrepare()                           // 发送prepare
		}
	}

	return nil
}

// acceptPrePrepare 接收prePrepare
func (c *core) acceptPrePrepare(prePrepare *pbftProtocol.PrePrepare) {
	c.consensusTimestamp = time.Now()
	c.currentRoundState.SetPrePrepare(prePrepare)
}
