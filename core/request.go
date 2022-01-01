// Package core
//
// @author: xwc1125
package core

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/util/dateutil"
)

// Request 请求
func (c *core) Request(request *pbftProtocol.Request) error {
	c.log.Debug("request-1) send request proposal", "hash", request.Proposal.Hash(), "height", request.Proposal.Height())
	if err := c.checkRequestMsg(request); err != nil {
		if err != errFutureMessage {
			c.log.Debug("request-1) check request msg err", "hash", request.Proposal.Hash(), "height", request.Proposal.Height(), "err", err)
			return err
		}
	}
	c.requestCh <- request
	return nil
}

// HandleRequest 处理request
func (c *core) HandleRequest(request *pbftProtocol.Request) error {
	c.log.Debug("request-2) handle request proposal", "height", request.Proposal.Height(), "hash", request.Proposal.Hash(), "elapsed", uint64(dateutil.CurrentTimeSecond())-request.Proposal.Timestamp())
	if err := c.checkRequestMsg(request); err != nil {
		c.log.Error("request-2) handle check request msg err", "height", request.Proposal.Height(), "hash", request.Proposal.Hash(), "err", err)
		return err
	}

	// 如果状态是接收请求，那么来了新的请求会作为pendingRequest
	if c.state == pbftProtocol.StateAcceptRequest {
		c.currentRoundState.pendingRequest = request
	}
	//c.newRoundChangeTimer()
	if c.state == pbftProtocol.StateAcceptRequest && !c.waitingForRoundChange.Load() {
		c.SendPrePrepare(request)
	}
	return nil
}

// check 检测请求
// return errInvalidMessage if the Message is invalid
// return errFutureMessage if the sequence of proposal is larger than currentRoundState sequence
// return pbftProtocol.ErrOldMessage if the sequence of proposal is smaller than currentRoundState sequence
func (c *core) checkRequestMsg(request *pbftProtocol.Request) error {
	if request == nil || request.Proposal == nil {
		return errInvalidMessage
	}
	if c.currentRoundState.sequence > request.Proposal.Height() {
		return pbftProtocol.ErrOldMessage
	} else if c.currentRoundState.sequence < request.Proposal.Height() {
		return errFutureMessage
	} else {
		return nil
	}
}

// StoreRequestMsg 保存请求的数据
func (c *core) StoreRequestMsg(request *pbftProtocol.Request) {
	c.log.Debug("request-3) store future request into pending", "height", request.Proposal.Height(), "hash", request.Proposal.Hash())
	c.pendingRequestsMu.Lock()
	defer c.pendingRequestsMu.Unlock()

	c.pendingRequests.Push(request, float32(-request.Proposal.Height()))
}

// clearPendingRequests 清理pending的请求
func (c *core) clearPendingRequests() {
	c.pendingRequestsMu.Lock()
	defer c.pendingRequestsMu.Unlock()

	c.pendingRequests.Reset()
}

// ProcessPendingRequests 处理pending的请求
func (c *core) ProcessPendingRequests() {
	c.pendingRequestsMu.Lock()
	defer c.pendingRequestsMu.Unlock()

	for !(c.pendingRequests.Empty()) {
		m, prio := c.pendingRequests.Pop()
		r, ok := m.(*pbftProtocol.Request)
		if !ok {
			c.log.Warn("malformed request, skip", "msg", m)
			continue
		}
		// Push back if it's a future Message
		err := c.checkRequestMsg(r)
		if err != nil {
			if err == errFutureMessage {
				c.log.Trace("stop processing request", "height", r.Proposal.Height(), "hash", r.Proposal.Hash())
				c.pendingRequests.Push(m, prio)
				break
			}
			c.log.Trace("skip the pending request", "height", r.Proposal.Height(), "hash", r.Proposal.Hash(), "err", err)
			continue
		}
		c.log.Trace("post pending request", "height", r.Proposal.Height(), "hash", r.Proposal.Hash())

		go c.sendPendingRequestEvent(r)
	}
}

// sendPendingRequestEvent 发送pending请求
func (c *core) sendPendingRequestEvent(r *pbftProtocol.Request) {
	c.requestCh <- r
}
