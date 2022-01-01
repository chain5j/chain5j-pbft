// Package core
//
// @author: xwc1125
package core

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	prque "github.com/chain5j/chain5j-pkg/collection/queues/preque"
)

var (
	// msgPriority 用于计算msg级别
	// MsgPrePrepare > MsgCommit > MsgPrepare
	msgPriority = map[uint64]int{
		pbftProtocol.MsgPrePrepare: 1,
		pbftProtocol.MsgCommit:     2,
		pbftProtocol.MsgPrepare:    3,
	}
)

// backlogEvent 积压事件
type backlogEvent struct {
	val pbftProtocol.Validator
	msg *pbftProtocol.Message
}

// StoreBacklog 保存积压数据
func (c *core) StoreBacklog(msg *pbftProtocol.Message, val pbftProtocol.Validator) {
	if val.ID() == c.peerId {
		c.log.Warn("Backlog from self")
		return
	}

	c.log.Debug("Store future message")

	c.backlogsLock.Lock()
	defer c.backlogsLock.Unlock()

	backlog := c.backlogs[val]
	if backlog == nil {
		backlog = prque.New()
	}
	switch msg.Code {
	case pbftProtocol.MsgPrePrepare:
		// 预处理的消息
		var p *pbftProtocol.PrePrepare
		err := msg.Decode(&p)
		if err == nil {
			c.log.Debug("store future pre-prepare msg", "hash", p.Proposal.Hash(), "sequence", p.View.Sequence, "round", p.View.Round)
			backlog.Push(msg, toPriority(msg.Code, p.View))
		} else {
			c.log.Error("decode pre-prepare msg err", "err", err)
		}
	default:
		// MsgRoundChange, MsgPrepare and MsgCommit 的消息
		var p *pbftProtocol.Subject
		err := msg.Decode(&p)
		if err == nil {
			c.log.Debug("store future subject msg", "hash", p.Digest, "sequence", p.View.Sequence, "round", p.View.Round)
			backlog.Push(msg, toPriority(msg.Code, p.View))
		} else {
			c.log.Error("decode subject msg err", "err", err)
		}
	}
	c.backlogs[val] = backlog
}

// toPriority 获取优先级
func toPriority(msgCode uint64, view *pbftProtocol.View) float32 {
	if msgCode == pbftProtocol.MsgRoundChange {
		// 如果是 MsgRoundChange，返回以sequence为基础的值
		return -float32(view.Sequence.Uint64() * 1000)
	}
	// FIXME: round will be reset as 0 while new sequence
	// messageCode：范围为0～9
	// round：范围为0～99
	return -float32(view.Sequence.Uint64()*1000 + view.Round.Uint64()*10 + uint64(msgPriority[msgCode]))
}

// ProcessBacklog 处理积压数据
func (c *core) ProcessBacklog() {
	c.backlogsLock.Lock()
	defer c.backlogsLock.Unlock()

	for src, backlog := range c.backlogs {
		if backlog == nil {
			continue
		}

		isFuture := false

		// 如果发生以下情况，我们将停止处理
		//   1. backlog is empty
		//   2. The first message in queue is a future message
		for !(backlog.Empty() || isFuture) {
			m, priority := backlog.Pop()
			msg := m.(*pbftProtocol.Message)
			var view *pbftProtocol.View
			switch msg.Code {
			case pbftProtocol.MsgPrePrepare:
				var m *pbftProtocol.PrePrepare
				err := msg.Decode(&m)
				if err == nil {
					view = m.View
				}
			default:
				// MsgRoundChange, MsgPrepare and MsgCommit 消息
				var sub *pbftProtocol.Subject
				err := msg.Decode(&sub)
				if err == nil {
					view = sub.View
				}
			}
			if view == nil {
				c.log.Debug("Nil view", "msg", msg)
				continue
			}
			// 如果是future信息，请往后推
			err := c.checkMessage(msg.Code, view)
			if err != nil {
				if err == errFutureMessage {
					c.log.Trace("Stop processing backlog", "msg", msg)
					backlog.Push(msg, priority)
					isFuture = true
					break
				}
				c.log.Trace("Skip the backlog event", "msg", msg, "err", err)
				continue
			}
			c.log.Trace("Post backlog event", "msg", msg)
			// 发送事件
			go c.sendBackLogEvent(src, msg)
		}
	}
}

// sendBackLogEvent 发送积压事件
func (c *core) sendBackLogEvent(val pbftProtocol.Validator, msg *pbftProtocol.Message) {
	c.backlogCh <- backlogEvent{
		val: val,
		msg: msg,
	}
}
