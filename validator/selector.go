// Package validator
//
// @author: xwc1125
package validator

import pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"

// calcSeed 计算seed
func calcSeed(valSet pbftProtocol.ValidatorSet, proposer string, round uint64) uint64 {
	offset := 0
	if idx, val := valSet.GetById(proposer); val != nil {
		offset = idx
	}
	// 提议者未知+轮询的次数
	return uint64(offset) + round
}

// roundRobinProposer 轮询选举策略
func roundRobinProposer(valSet pbftProtocol.ValidatorSet, proposer string, round uint64) pbftProtocol.Validator {
	if valSet.Size() == 0 {
		return nil
	}

	seed := uint64(0)
	if proposer == "" {
		seed = round
	} else {
		// 往后移动一位
		seed = calcSeed(valSet, proposer, round) + 1
	}
	// 通过模获取新的提议者
	pick := seed % uint64(valSet.Size())
	return valSet.GetByIndex(pick)
}

// stickyProposer 在round更换时更换proposer的策略
func stickyProposer(valSet pbftProtocol.ValidatorSet, proposer string, round uint64) pbftProtocol.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	seed := uint64(0)
	if proposer == "" {
		seed = round
	} else {
		// 到固定时间段更换
		seed = calcSeed(valSet, proposer, round)
	}
	pick := seed % uint64(valSet.Size())
	return valSet.GetByIndex(pick)
}
