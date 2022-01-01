// Package pbft
//
// @author: xwc1125
package pbft

import (
	"encoding/json"
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/database/kvstore"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/logger"
)

const (
	dbKeySnapshotPrefix = "pbft-snapshot" // pbft快照key前缀
)

// Tally 简单的计票方法
// 保存当前的投票分数。
// 反对该提案的选票不予统计，这等于不投票。
type Tally struct {
	Authorize bool `json:"authorize"` // 是授权还是剔除的投票
	Votes     int  `json:"votes"`     // 投赞成票数
}

// Snapshot 快照
type Snapshot struct {
	log logger.Logger

	Epoch  uint64     // 需要检测和重新投票的区块个数
	Height uint64     // 创建快照的块高
	Hash   types.Hash // 创建快照的块hash

	voteSnapshot *voteSnapshot             // 投票快照
	ValSet       pbftProtocol.ValidatorSet // 快照对应的验证者集合
}

// newSnapshot 创建新的快照
func newSnapshot(epoch uint64, height uint64, hash types.Hash, valSet pbftProtocol.ValidatorSet, managers []types.Address) *Snapshot {
	snap := &Snapshot{
		log:          logger.New("pbft.snapshot"),
		Epoch:        epoch,
		Height:       height,
		Hash:         hash,
		ValSet:       valSet,
		voteSnapshot: newVoteSnapshot(valSet, managers),
	}

	return snap
}

// loadSnapshot 从数据库中加载快照
func loadSnapshot(epoch uint64, db kvstore.Database, hash types.Hash) (*Snapshot, error) {
	// 从数据库中读取快照
	blob, err := db.Get(append([]byte(dbKeySnapshotPrefix), hash[:]...))
	if err != nil {
		return nil, err
	}
	logger.Trace("load snapshot blob by hash", "hash", hash.Hex(), "blob", string(blob))

	snap := new(Snapshot)
	//if err := codec.Coder().Decode(blob, snap); err != nil {
	//	return nil, err
	//}
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.Epoch = epoch

	return snap, nil
}

// Store 保存快照
func (s *Snapshot) Store(db kvstore.Database) error {
	//blob, err := codec.Coder().Encode(s)
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}

	s.log.Trace("store snapshot blob to db", "hash", s.Hash.Hex(), "blob", string(blob))
	return db.Put(append([]byte(dbKeySnapshotPrefix), s.Hash[:]...), blob)
}

// checkMVote 判断地址是否可被投票
func (s *Snapshot) checkMVote(address types.Address, authorize bool) bool {
	return s.voteSnapshot.checkMVote(address, authorize)
}

// checkVVote 判断id是否可被投票
func (s *Snapshot) checkVVote(id string, authorize bool) bool {
	return s.voteSnapshot.checkVVote(id, authorize)
}

// Apply 通过给定的headers创建新快照
func (s *Snapshot) Apply(headers []*models.Header) (*Snapshot, error) {
	if len(headers) == 0 {
		return s, nil
	}

	// 判断headers是否是连续递增的
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Height != headers[i].Height+1 {
			return nil, errInvalidVotingChain
		}
	}

	// 判读传入的headers的最小高度=快照高度+1
	if headers[0].Height != s.Height+1 {
		return nil, errInvalidVotingChain
	}
	// 复制snapshot
	snap := s.copy()

	for _, header := range headers {
		// 删除检查点块上的所有投票
		height := header.Height
		if s.Epoch == 0 {
			s.Epoch = 1
		}
		if height%s.Epoch == 0 {
			snap.voteSnapshot.reset()
		}

		consensusData := new(ConsensusData)
		if err := consensusData.FromConsensus(header.Consensus); err != nil {
			return nil, err
		}

		s.voteSnapshot.applyMVotes(consensusData)
		s.voteSnapshot.applyVVotes(consensusData)
	}
	snap.Height += uint64(len(headers))
	snap.Hash = headers[len(headers)-1].Hash()

	return snap, nil
}

// copy 深度拷贝
func (s *Snapshot) copy() *Snapshot {
	cpy := &Snapshot{
		log:          s.log,
		Epoch:        s.Epoch,
		Height:       s.Height,
		Hash:         s.Hash,
		voteSnapshot: s.voteSnapshot.copy(),
		ValSet:       s.ValSet,
	}

	return cpy
}

type snapshotJSON struct {
	Epoch  uint64     `json:"epoch"`
	Height uint64     `json:"height"`
	Hash   types.Hash `json:"hash"`

	VoteManager *voteSnapshot `json:"vote_manager"`
}

func (s *Snapshot) toJSONStruct() *snapshotJSON {
	return &snapshotJSON{
		Epoch:       s.Epoch,
		Height:      s.Height,
		Hash:        s.Hash,
		VoteManager: s.voteSnapshot,
	}
}

// UnmarshalJSON 反序列化
func (s *Snapshot) UnmarshalJSON(b []byte) error {
	var j snapshotJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	s.Epoch = j.Epoch
	s.Height = j.Height
	s.Hash = j.Hash
	s.voteSnapshot = j.VoteManager
	return nil
}

// MarshalJSON 序列化
func (s *Snapshot) MarshalJSON() ([]byte, error) {
	j := s.toJSONStruct()
	return json.Marshal(j)
}
