// Package core
//
// @author: xwc1125
package core

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/crypto/hashalg/sha3"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-protocol/models"
	lru "github.com/hashicorp/golang-lru"
)

// Broadcast 本地消息签名后广播, 从p2p签名过来的消息，不要调用此方法
func (c *core) Broadcast(msg *pbftProtocol.Message) {
	payload, err := c.finalizeMessage(msg)
	if err != nil {
		c.log.Error("Failed to finalize Message", "state", c.state, "msg", msg, "err", err)
		return
	}

	// 发送给本地接收程序
	go func() {
		c.localMsgCh <- payload
	}()

	if err := c.GossipPayload(payload); err != nil {
		c.log.Error("Failed to broadcast Message", "state", c.state, "msg", msg, "err", err)
	}
}

// finalizeMessage 组装最终的消息
func (c *core) finalizeMessage(msg *pbftProtocol.Message) ([]byte, error) {
	var err error
	msg.Validator = c.peerId // 当前peer作为验证者

	// 如果是CommittedSeal类型的交易，那么proposal不能为空
	if msg.Code == pbftProtocol.MsgCommit && c.currentRoundState.Proposal() != nil {
		seal := pbftProtocol.PrepareCommittedSeal(c.currentRoundState.Proposal().Hash())
		signResult, err := c.backend.Sign(seal)
		if err != nil {
			return nil, err
		} // todo 对区块hash进行签名处理
		msg.CommittedSeal = signResult
	}

	// 获取message未签名的数据
	data, err := msg.PayloadNoSig()
	if err != nil {
		return nil, err
	}
	// c.log.Debug("no sign data", "data", hexutil.Encode(data))
	signResult, err := c.backend.Sign(data)
	if err != nil {
		return nil, err
	}
	msg.Signature = signResult
	// 将签名后的msg转换为bytes
	payload, err := msg.Payload()
	if err != nil {
		return nil, err
	}

	return payload, nil
}

// GossipPayload 广播传送消息
func (c *core) GossipPayload(payload []byte) error {
	hash := types.BytesToHash(sha3.Keccak256(payload))
	var validators []models.P2PID
	for _, val := range c.valSet.List() {
		if val.ID() != c.peerId {
			peerId := val.ID()
			pid, _ := models.IDFromString(peerId)

			ms, ok := c.peersMessages.Get(peerId)
			var m *lru.ARCCache
			if ok {
				m, _ = ms.(*lru.ARCCache)
				if _, k := m.Get(hash); k {
					continue
				}
			} else {
				m, _ = lru.NewARC(inmemoryMessages)
			}

			m.Add(hash, true)
			c.peersMessages.Add(peerId, m)

			validators = append(validators, pid)
		}
	}

	if len(validators) > 0 {
		// 广播内容
		if c.broadcaster != nil {
			if err := c.broadcaster.Broadcast(validators, models.ConsensusMsg, payload); err != nil {
				return err
			}
		}
	}

	return nil
}
