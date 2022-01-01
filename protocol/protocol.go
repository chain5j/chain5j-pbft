// Package protocol
//
// @author: xwc1125
package protocol

import (
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/types"
)

// PBFTEngine pbft共识引擎
type PBFTEngine interface {
	Start() error
	Stop() error
	Request(*Request) error
	RequestTimeout()
	NewChainHead() error
}

// PBFTBackend pbft核心函数接口
type PBFTBackend interface {
	ID() string                                      // 当前Validator的ID
	Validators(proposal Proposal) ValidatorSet       // validator的集合
	ParentValidators(proposal Proposal) ValidatorSet // 获取给定的提议父区块获取validator集合

	Verify(proposal Proposal) error                                // 验证提议
	Commit(proposal Proposal, seals []*signature.SignResult) error // 提交已批准的提议，提议将会写入区块链

	Sign(data []byte) (*signature.SignResult, error)                               // 使用私钥进行签名数据
	CheckSignature(data []byte, validator string, sig *signature.SignResult) error // 判断签名是否为给定的validator签署的

	LastProposal() (Proposal, string)                // 获取最新的proposal提议，及proposer提议者的地址
	HasProposal(hash types.Hash, height uint64) bool // 检查给定Hash和高度是否有匹配的提议
	GetProposer(height uint64) string                // 根据区块高度获取提议者地址
}

// Proposal 为区块在共识过程中的抽象接口。
type Proposal interface {
	Height() uint64    // 返回提议的序列号
	Hash() types.Hash  // 返回提议的hash
	Timestamp() uint64 // 返回提议时间戳
}
