// Package protocol
//
// @author: xwc1125
package protocol

import (
	"strings"
)

// Validator 验证者
type Validator interface {
	ID() string     // ID 返回PBFT节点标识
	String() string // 校验者字符串
}

// Validators 校验者集合
type Validators []Validator

func (slice Validators) Len() int {
	return len(slice)
}
func (slice Validators) Less(i, j int) bool {
	return strings.Compare(slice[i].ID(), slice[j].ID()) < 0
}
func (slice Validators) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

// ValidatorSet 验证者集
type ValidatorSet interface {
	CalcProposer(lastProposer string, round uint64) // 根据Proposer计算下一个Proposer, 并记录到ValidatorSet, 通过GetProposer查询
	GetProposer() Validator                         // 返回当前proposer
	IsProposer(id string) bool                      // 查询指定的id是否为proposer
	Policy() ProposerPolicy                         // 提议选举策略

	GetByIndex(index uint64) Validator                  // 通过索引查找validator
	GetById(id string) (index int, validator Validator) // 通过ID查找validator, 并返回其索引
	AddValidator(id string) bool                        // 添加validator
	RemoveValidator(id string) bool                     // 删除validator
	List() []Validator                                  // 返回 validator 数组
	Size() int                                          // 返回验证者集合的长度

	Copy() ValidatorSet    // 复制
	FaultTolerantNum() int // 容错节点数
}

// ProposalSelector 提议选举策略
type ProposalSelector func(ValidatorSet, string, uint64) Validator
