// Package core
//
// @author: xwc1125
package core

import (
	"github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/codec"
	"reflect"
)

// SendPrepare 发送prepare消息
func (c *core) SendPrepare() {
	sub := c.currentRoundState.Subject()
	c.log.Debug("prepare-1) send prepare subject", "hash", sub.Digest.Hex(), "sequence", sub.View.Sequence, "round", sub.View.Round)
	prepareBytes, err := codec.Coder().Encode(sub)
	if err != nil {
		c.log.Error("prepare-1) failed to encode", "subject", sub, "err", err)
		return
	}
	c.Broadcast(&protocol.Message{
		Code: protocol.MsgPrepare,
		Msg:  prepareBytes,
	})
}

// HandlePrepare 处理prepare消息
func (c *core) HandlePrepare(msg *protocol.Message, val protocol.Validator) error {
	var prepare *protocol.Subject
	err := msg.Decode(&prepare)
	if err != nil {
		return errFailedDecodePrepare
	}
	c.log.Debug("prepare-2) handle prepare message", "hash", prepare.Digest.Hex(), "sequence", prepare.View.Sequence, "round", prepare.View.Round)

	if err := c.checkMessage(protocol.MsgPrepare, prepare.View); err != nil {
		c.log.Debug("prepare-2) handle check prepare message", "hash", prepare.Digest.Hex(), "sequence", prepare.View.Sequence, "round", prepare.View.Round, "err", err)
		return err
	}

	// 如果已锁定，则只能在锁定的块上进行处理
	if err := c.verifyPrepare(prepare, val); err != nil {
		c.log.Debug("prepare-2) verify prepare message", "err", err)
		return err
	}
	// 接受prepare消息
	c.acceptPrepare(msg, val)

	// Change to Prepared state if we've received enough PREPARE messages or it is locked
	// and we are in earlier state before Prepared state.
	if ((c.currentRoundState.IsHashLocked() && prepare.Digest == c.currentRoundState.GetLockedHash()) || c.reachPrepareThreshold()) && c.state.Cmp(protocol.StatePrepared) < 0 {
		c.currentRoundState.LockHash()     // 锁定hash
		c.setState(protocol.StatePrepared) // 更改状态
		c.SendCommit()                     // 发送commit
	}

	return nil
}

// reachPrepareThreshold 达到prepare的门限
func (c *core) reachPrepareThreshold() bool {
	prepareSize := c.currentRoundState.GetPrepareOrCommitSize()
	return reachThreshold(c.valSet, prepareSize)
}

// verifyPrepare 验证prepare是否等于当前的prepare
func (c *core) verifyPrepare(prepare *protocol.Subject, val protocol.Validator) error {
	sub := c.currentRoundState.Subject()
	if !reflect.DeepEqual(prepare, sub) {
		c.log.Warn("prepare-2_1) inconsistent subjects between PREPARE and proposal", "expected", sub, "got", prepare)
		return errInconsistentSubject
	}

	return nil
}

// acceptPrepare 接受prepare消息
func (c *core) acceptPrepare(msg *protocol.Message, val protocol.Validator) error {
	// 将prepare消息添加到当前的状态中
	if err := c.currentRoundState.Prepares.Add(msg); err != nil {
		c.log.Error("Failed to add PREPARE Message to round state", "msg", msg, "err", err)
		return err
	}

	return nil
}
