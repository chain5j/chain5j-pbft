// Package pbft
//
// @author: xwc1125
package pbft

import "errors"

var (
	errInvalidProposal       = errors.New("invalid proposal")                          // 无效的proposal
	errInvalidCommittedSeals = errors.New("invalid committed seals")                   // 无效的commit seal，未签名
	errEmptyCommittedSeals   = errors.New("zero committed seals")                      // commit seal为空
	errMismatchTxRoot        = errors.New("mismatch transaction root")                 // 交易root不匹配
	errBlockIsGenesis        = errors.New("is genesis block")                          // 创世区块
	errUnknownBlock          = errors.New("unknown block")                             // 未知区块
	errInvalidConsensusData  = errors.New("invalid consensus data format")             // 无效共识内容
	errInvalidTimestamp      = errors.New("invalid timestamp")                         // 无效时间戳
	errInvalidSignature      = errors.New("invalid signature")                         // 无效签名
	errInvalidVotingChain    = errors.New("invalid voting chain")                      // 无效投票链
	errBlockSmall            = errors.New("block height is small than genesis height") // 区块高度过小
)
