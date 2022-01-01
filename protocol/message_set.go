// Package protocol
//
// @author: xwc1125
package protocol

import (
	"fmt"
	"math/big"
	"strings"
	"sync"
)

// MessageSet message集合
type MessageSet struct {
	view   *View        // 视图
	valSet ValidatorSet // 验证者集合

	messagesLock sync.Mutex
	messages     map[string]*Message // validator-->Message
}

// NewMessageSet 根据validatorSet构造messageSet
func NewMessageSet(valSet ValidatorSet) *MessageSet {
	return &MessageSet{
		view: &View{
			Round:    new(big.Int),
			Sequence: new(big.Int),
		},
		valSet:   valSet,
		messages: make(map[string]*Message),
	}
}

// View 获取集合中的view
func (ms *MessageSet) View() *View {
	return ms.view
}

// Add 添加消息
func (ms *MessageSet) Add(msg *Message) error {
	ms.messagesLock.Lock()
	defer ms.messagesLock.Unlock()

	// 验证消息
	if err := ms.verify(msg); err != nil {
		return err
	}
	ms.messages[msg.Validator] = msg
	return nil
}

// verify 验证消息
func (ms *MessageSet) verify(msg *Message) error {
	// 判断消息的validator是否来自于validators
	if _, v := ms.valSet.GetById(msg.Validator); v == nil {
		return ErrUnauthorized
	}
	// TODO: check view number and sequence number

	return nil
}

// Values 获取所有的message
func (ms *MessageSet) Values() (result []*Message) {
	ms.messagesLock.Lock()
	defer ms.messagesLock.Unlock()

	for _, v := range ms.messages {
		result = append(result, v)
	}

	return result
}

// Size 获取messages的个数
func (ms *MessageSet) Size() int {
	ms.messagesLock.Lock()
	defer ms.messagesLock.Unlock()
	return len(ms.messages)
}

// Get 根据peerId获取消息
func (ms *MessageSet) Get(val string) *Message {
	ms.messagesLock.Lock()
	defer ms.messagesLock.Unlock()
	return ms.messages[val]
}

// String 打印所有消息
func (ms *MessageSet) String() string {
	ms.messagesLock.Lock()
	defer ms.messagesLock.Unlock()
	addresses := make([]string, 0, len(ms.messages))
	for _, v := range ms.messages {
		addresses = append(addresses, v.Validator)
	}
	return fmt.Sprintf("[%v]", strings.Join(addresses, ", "))
}
