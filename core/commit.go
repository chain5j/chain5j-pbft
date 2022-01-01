// Package core
//
// @author: xwc1125
package core

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/types"
	"reflect"
)

// SendCommit 发送commit
func (c *core) SendCommit() {
	sub := c.currentRoundState.Subject()
	c.log.Debug("commit-1) send commit message", "hash", sub.Digest.Hex(), "sequence", sub.View.Sequence, "round", sub.View.Round)
	c.broadcastCommit(sub)
}

// sendCommitForOldBlock 发送旧的区块信息
func (c *core) sendCommitForOldBlock(view *pbftProtocol.View, bHash types.Hash) {
	sub := &pbftProtocol.Subject{
		View:   view,
		Digest: bHash,
	}
	c.broadcastCommit(sub)
}

// broadcastCommit 广播commit
func (c *core) broadcastCommit(sub *pbftProtocol.Subject) {
	// 将commit数据进行编码
	commitBytes, err := codec.Coder().Encode(sub)
	if err != nil {
		c.log.Error("commit-1_1) failed to encode commit msg", "subject", sub)
		return
	}
	c.Broadcast(&pbftProtocol.Message{
		Code: pbftProtocol.MsgCommit,
		Msg:  commitBytes,
	})
}

// HandleCommit 处理commit内容
func (c *core) HandleCommit(msg *pbftProtocol.Message, val pbftProtocol.Validator) error {
	var commit *pbftProtocol.Subject
	err := msg.Decode(&commit)
	if err != nil {
		return errFailedDecodeCommit
	}

	c.log.Debug("commit-2) handle commit message", "hash", commit.Digest.Hex(), "sequence", commit.View.Sequence, "round", commit.View.Round)
	// 消息内容的检测，确保一定为commit消息
	if err := c.checkMessage(pbftProtocol.MsgCommit, commit.View); err != nil {
		return err
	}

	// 判断commit和当前的subject是否一致
	if err := c.verifyCommit(commit, val); err != nil {
		return err
	}

	// 接受commit并存入共识状态中
	c.acceptCommit(msg, val)

	// 判断commit是否达到门限
	// 如果已达到门限，并且当前状态不为committed，那么就需要固定hash，进行最终的commit
	if c.reachCommitThreshold() && c.state.Cmp(pbftProtocol.StateCommitted) < 0 {
		// 【注意】这里一定需要调用LockHash
		// 因为state可以跳过Prepared并直接跳到Committed。
		c.currentRoundState.LockHash()
		c.commit()
	}

	return nil
}

// commit 提交
func (c *core) commit() {
	c.setState(pbftProtocol.StateCommitted) // 修改状态

	proposal := c.currentRoundState.Proposal()
	if proposal != nil {
		committedSeals := make([]*signature.SignResult, c.currentRoundState.Commits.Size())
		for i, v := range c.currentRoundState.Commits.Values() {
			committedSeals[i] = v.CommittedSeal
		}

		if err := c.backend.Commit(proposal, committedSeals); err != nil {
			c.currentRoundState.UnlockHash() //Unlock block when insertion fails
			c.sendNextRoundChange()
			return
		}
	}
}

// reachCommitThreshold 判断commit是否达到门限
func (c *core) reachCommitThreshold() bool {
	// 获取当前commit个数
	return reachThreshold(c.valSet, c.currentRoundState.Commits.Size())
}

// verifyCommit 判断commit和当前的subject是否一致
func (c *core) verifyCommit(commit *pbftProtocol.Subject, val pbftProtocol.Validator) error {
	// commit和当前的消息不一致（subject包含prepare和commit）
	sub := c.currentRoundState.Subject()
	if !reflect.DeepEqual(commit, sub) {
		c.log.Warn("commit-2_1) Inconsistent subjects between commit and proposal", "expected", sub, "got", commit)
		return errInconsistentSubject
	}

	return nil
}

// acceptCommit 接受commit消息
func (c *core) acceptCommit(msg *pbftProtocol.Message, val pbftProtocol.Validator) error {
	// 将commit消息添加到commits中
	if err := c.currentRoundState.Commits.Add(msg); err != nil {
		c.log.Error("Failed to record commit Message", "msg", msg, "err", err)
		return err
	}

	return nil
}
