// Package core
//
// @author: xwc1125
package core

import pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"

// decodeMsg 解析消息
func (c *core) decodeMsg(payload []byte) (*pbftProtocol.Message, pbftProtocol.Validator, error) {
	// 反编码消息，并验证签名
	msg := new(pbftProtocol.Message)
	if err := msg.FromPayload(payload, c.validateFn); err != nil {
		c.log.Error("Failed to decode Message from payload", "err", err)
		return nil, nil, err
	}

	// 只接收合法消息
	_, val := c.valSet.GetById(msg.Validator)
	if val == nil {
		c.log.Error("Invalid validator in Message", "msg", msg)
		return nil, nil, pbftProtocol.ErrUnauthorized
	}

	return msg, val, nil
}

// checkMessage 消息状态检测
// @params msgCode 消息类型
// @view view 视图
// return errInvalidMessage if the message is invalid
// return errFutureMessage if the message view is larger than current view
// return pbftProtocol.ErrOldMessage if the message view is smaller than current view
func (c *core) checkMessage(msgCode uint64, view *pbftProtocol.View) error {
	if view == nil || view.Sequence == nil || view.Round == nil {
		return errInvalidMessage
	}

	// 轮换，sequence必须相等
	if msgCode == pbftProtocol.MsgRoundChange {
		if view.Sequence.Cmp(c.currentView().Sequence) > 0 {
			return errFutureMessage
		} else if view.Cmp(c.currentView()) < 0 {
			return pbftProtocol.ErrOldMessage
		}
		return nil
	}

	// 视图必须相等
	if view.Cmp(c.currentView()) > 0 {
		return errFutureMessage
	} else if view.Cmp(c.currentView()) < 0 {
		return pbftProtocol.ErrOldMessage
	}

	if c.waitingForRoundChange.Load() {
		return errFutureMessage
	}

	// StateAcceptRequest只能通过MsgPrePrepare
	// 其他的都是future
	if c.state == pbftProtocol.StateAcceptRequest {
		if msgCode > pbftProtocol.MsgPrePrepare {
			return errFutureMessage
		}
		return nil
	}

	// 对于状态（StatePrepared、StatePrepared、StateCommitted），
	// 如果使用相同的视图进行处理，则可以接受所有消息类型
	return nil
}

// setState 设置状态
func (c *core) setState(state pbftProtocol.State) {
	if c.state != state {
		c.state = state
	}
	if state == pbftProtocol.StateAcceptRequest {
		c.ProcessPendingRequests()
	}
	c.ProcessBacklog()
}

// reachThreshold 是否达到轮换门限
func reachThreshold(valSet pbftProtocol.ValidatorSet, num int) bool {
	f := valSet.FaultTolerantNum() // 获取容错节点数
	if f == 0 {
		// 如果是0容忍，那么必须相等
		return num == valSet.Size()
	} else if num > 2*f {
		// 否则需要满足2f+1
		return true
	}
	return false
}
