// Package protocol
//
// @author: xwc1125
package protocol

import (
	"fmt"
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
)

// 消息类型
const (
	MsgPrePrepare  uint64 = iota // prePrepare
	MsgPrepare                   // prepare
	MsgCommit                    // commit
	MsgRoundChange               // 轮换
)

// Message pbft消息结构
type Message struct {
	Code          uint64                `json:"code"`                     // 消息类型
	Msg           []byte                `json:"msg"`                      // 消息内容
	Validator     string                `json:"validator"`                // 验证者（peerId）
	Signature     *signature.SignResult `json:"signature" rlp:"nil"`      // 签名内容
	CommittedSeal *signature.SignResult `json:"committed_seal" rlp:"nil"` // 被提交的seal
}

// FromPayload 将msgBytes转换为message
func (m *Message) FromPayload(msg []byte, validateFn func(data []byte, signResult *signature.SignResult) (string, error)) error {
	err := codec.Coder().Decode(msg, &m)
	if err != nil {
		return err
	}

	// 验证无签名的消息内容
	if validateFn != nil {
		payload, err := m.PayloadNoSig()
		if err != nil {
			return err
		}

		_, err = validateFn(payload, m.Signature)
	}
	return err
}

// Payload message的bytes内容
func (m *Message) Payload() ([]byte, error) {
	return codec.Coder().Encode(m)
}

// PayloadNoSig Signature为空的bytes数据
func (m *Message) PayloadNoSig() ([]byte, error) {
	return codec.Coder().Encode(&Message{
		Code:          m.Code,
		Msg:           m.Msg,
		Validator:     m.Validator,
		CommittedSeal: m.CommittedSeal,
		Signature:     nil,
	})
}

// Decode 将message.Msg转换为对象，val必须为指针类型
func (m *Message) Decode(val interface{}) error {
	return codec.Coder().Decode(m.Msg, val)
}

// String 打印message字符串
func (m *Message) String() string {
	return fmt.Sprintf("{Code: %v, Id: %v}", m.Code, m.Validator)
}
