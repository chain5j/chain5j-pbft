// Package protocol
//
// @author: xwc1125
package protocol

import "github.com/chain5j/chain5j-pkg/types"

// ProposerPolicy 提议策略
type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota // 在每个区块或者round更换时，更换proposer
	Sticky                           // 在round更换时更换proposer
)

// PBFTConfig pbft的配置
type PBFTConfig struct {
	Epoch          uint64          `json:"epoch" mapstructure:"epoch"`           // 期数
	RequestTimeout uint64          `json:"timeout" mapstructure:"timeout"`       // round 超时时间， 在BlockPeriod不为0时有效
	BlockPeriod    uint64          `json:"period" mapstructure:"period"`         // 区块产生间隔(毫秒)
	ProposerPolicy ProposerPolicy  `json:"policy" mapstructure:"policy"`         // Proposer 策略
	Managers       []types.Address `json:"managers" mapstructure:"managers"`     // 创世管理员地址
	Validators     []string        `json:"validators" mapstructure:"validators"` // 创世验证者地址
}
