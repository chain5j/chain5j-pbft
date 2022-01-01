// Package pbft
//
// @author: xwc1125
package pbft

import (
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-protocol/models"
	"math/big"
	"strings"
)

// ManagementVote 管理员添加删除投票
type ManagementVote struct {
	Manager        types.Address `json:"manager"`   // 此次投票的授权validator
	Candidate      types.Address `json:"candidate"` // 需被投票的候选人地址
	Authorize      bool          `json:"authorize"` // 是否投票候选人
	DeadlineHeight *big.Int      `json:"deadline"`  // 截止到某个区块前投票有效
	Signature      []byte        `json:"signature"` // 签名数据
}

// ValidatorVote 节点验证者添加删除投票
type ValidatorVote struct {
	Manager        types.Address `json:"manager"`   // 此次投票的授权validator
	Validator      string        `json:"validator"` // 需被投票的验证者地址
	Authorize      bool          `json:"authorize"` // 是否投票验证者
	DeadlineHeight *big.Int      `json:"deadline"`  // 截止到某个区块前投票有效
	Signature      []byte        `json:"signature"` // 签名数据
}

// ConsensusData 区块头中的共识数据
// [注意]在创世文件中，初始化的managers和Validators放在hashmap中，
// 需要解析创世的内容
type ConsensusData struct {
	CommittedSeal []*signature.SignResult

	Managers   []types.Address // 管理员地址
	Validators []string        // 验证者地址

	MVote *ManagementVote `rlp:"nil"`
	VVote *ValidatorVote  `rlp:"nil"`
}

func (c *ConsensusData) Serialize() ([]byte, error) {
	return codec.Coder().Encode(c)
}
func (c *ConsensusData) Deserialize(d []byte) error {
	return codec.Coder().Decode(d, c)
}
func (c ConsensusData) ToConsensus() (*models.Consensus, error) {
	payload, err := c.Serialize()
	if err != nil {
		return nil, err
	}
	return &models.Consensus{
		Name:      "pbft",
		Consensus: payload,
	}, nil
}
func (c *ConsensusData) FromConsensus(consensus *models.Consensus) error {
	if consensus == nil {
		return errInvalidConsensusData
	}
	if !strings.EqualFold(consensus.Name, "pbft") {
		return errInvalidConsensusData
	}
	if len(consensus.Consensus) == 0 {
		return errInvalidConsensusData
	}

	// 将共识的数据转换为共识需要的对象
	if err := c.Deserialize(consensus.Consensus); err != nil {
		return err
	}
	return nil
}
