// Package validator
//
// @author: xwc1125
package validator

import (
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/logger"
	"math"
	"reflect"
	"sort"
	"sync"
)

var (
	_ pbftProtocol.ValidatorSet = new(defaultValidatorSet)
)

// defaultValidatorSet  pbft.ValidatorSet的实现
type defaultValidatorSet struct {
	log           logger.Logger
	policy        pbftProtocol.ProposerPolicy // 提议策略
	proposer      pbftProtocol.Validator      // 提议者
	validators    pbftProtocol.Validators     // 验证者集合
	validatorLock sync.RWMutex

	selector pbftProtocol.ProposalSelector // 提议选举策略
}

// NewSet 创建新的ValidatorSet
func NewSet(ids []string, policy pbftProtocol.ProposerPolicy) pbftProtocol.ValidatorSet {
	return newDefaultSet(ids, policy)
}

// newDefaultSet 创建Validator集合
func newDefaultSet(ids []string, policy pbftProtocol.ProposerPolicy) *defaultValidatorSet {
	valSet := &defaultValidatorSet{
		log:    logger.New("pfbt.valSet"),
		policy: policy,
	}

	// 初始化验证者集合
	valSet.validators = make([]pbftProtocol.Validator, len(ids))
	for i, id := range ids {
		valSet.validators[i] = &defaultValidator{
			id: id,
		}
	}
	// 排序
	sort.Sort(valSet.validators)

	// 初始化proposer,默认选择第一个
	if valSet.Size() > 0 {
		valSet.proposer = valSet.GetByIndex(0)
	}

	// 选举策略
	valSet.selector = roundRobinProposer
	if policy == pbftProtocol.Sticky {
		valSet.selector = stickyProposer
	}

	return valSet
}

// Size 获取验证者数量
func (vs *defaultValidatorSet) Size() int {
	vs.validatorLock.RLock()
	defer vs.validatorLock.RUnlock()
	return len(vs.validators)
}

// List 获取验证者列表
func (vs *defaultValidatorSet) List() []pbftProtocol.Validator {
	vs.validatorLock.RLock()
	defer vs.validatorLock.RUnlock()
	return vs.validators
}

// GetByIndex 根据索引获取验证者
func (vs *defaultValidatorSet) GetByIndex(i uint64) pbftProtocol.Validator {
	if i < uint64(vs.Size()) {
		vs.validatorLock.RLock()
		defer vs.validatorLock.RUnlock()
		return vs.validators[i]
	}
	return nil
}

// GetById 根据id获取验证器
func (vs *defaultValidatorSet) GetById(id string) (int, pbftProtocol.Validator) {
	for index, val := range vs.List() {
		if id == val.ID() {
			return index, val
		}
	}
	vs.log.Debug("get validator empty", "valSet", vs.List(), "id", id)
	return -1, nil
}

// GetProposer 获取当前提议者
func (vs *defaultValidatorSet) GetProposer() pbftProtocol.Validator {
	return vs.proposer
}

// IsProposer 判断id是否为提议者
func (vs *defaultValidatorSet) IsProposer(id string) bool {
	_, val := vs.GetById(id)
	if val == nil {
		return false
	}

	return reflect.DeepEqual(vs.GetProposer(), val)
}

// CalcProposer 计算提议节点, 并记录到ValidatorSet, 通过GetProposer查询
func (vs *defaultValidatorSet) CalcProposer(lastProposer string, round uint64) {
	vs.validatorLock.RLock()
	defer vs.validatorLock.RUnlock()
	// 存入缓存
	vs.proposer = vs.selector(vs, lastProposer, round)
}

// AddValidator 添加validator
func (vs *defaultValidatorSet) AddValidator(id string) bool {
	vs.validatorLock.Lock()
	defer vs.validatorLock.Unlock()
	for _, v := range vs.validators {
		if v.ID() == id {
			// 已经存在
			return false
		}
	}
	vs.validators = append(vs.validators, &defaultValidator{
		id: id,
	})
	// 排序
	sort.Sort(vs.validators)
	return true
}

// RemoveValidator 删除validator
func (vs *defaultValidatorSet) RemoveValidator(id string) bool {
	vs.validatorLock.Lock()
	defer vs.validatorLock.Unlock()

	for i, v := range vs.validators {
		if v.ID() == id {
			// 只有存在时才能删除
			vs.validators = append(vs.validators[:i], vs.validators[i+1:]...)
			return true
		}
	}
	return false
}

// Copy 拷贝ValidatorSet
func (vs *defaultValidatorSet) Copy() pbftProtocol.ValidatorSet {
	vs.validatorLock.RLock()
	defer vs.validatorLock.RUnlock()

	ids := make([]string, 0, len(vs.validators))
	for _, v := range vs.validators {
		ids = append(ids, v.ID())
	}
	return newDefaultSet(ids, vs.Policy())
}

// FaultTolerantNum 容错节点数
func (vs *defaultValidatorSet) FaultTolerantNum() int {
	f := int(math.Ceil(float64(vs.Size())/3)) - 1
	return f
}

// Policy 选举策略
func (vs *defaultValidatorSet) Policy() pbftProtocol.ProposerPolicy { return vs.policy }
