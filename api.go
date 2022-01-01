// Package pbft
//
// @author: xwc1125
package pbft

import (
	"errors"
	pbftProtocol "github.com/chain5j/chain5j-pbft/protocol"
	"github.com/chain5j/chain5j-pkg/codec"
	"github.com/chain5j/chain5j-pkg/crypto/signature"
	"github.com/chain5j/chain5j-pkg/network/rpc"
	"github.com/chain5j/chain5j-pkg/types"
	"github.com/chain5j/chain5j-pkg/util/hexutil"
	"github.com/chain5j/chain5j-protocol/models"
	"github.com/chain5j/chain5j-protocol/protocol"
	"github.com/chain5j/logger"
	"math/big"
)

// API api对象
type API struct {
	b     *pbftEngine
	chain protocol.BlockReader
}

func (api *API) GetSnapshot(number *rpc.BlockNumber) (*Snapshot, error) {
	// Retrieve the requested block number (or current if none requested)
	var header *models.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentBlock().Header()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	// Ensure we have an actually valid block and return its snapshot
	if header == nil {
		return nil, errUnknownBlock
	}
	return api.b.snapshot(api.chain, header.Height, header.Hash(), nil)
}

// ProposeManager 投票添加或者删除管理员
func (api *API) ProposeManager(address types.Address, auth bool, deadline *hexutil.Big, sig hexutil.Bytes) error {
	enc, _ := codec.Coder().Encode([]interface{}{
		"propose",
		address,
		auth,
		deadline.ToInt(),
	})

	manager, err := api.checkApiVote(enc, sig, deadline)
	if err != nil {
		return err
	}

	api.b.voteManager.setMVote(address, &ManagementVote{
		Manager:        manager,
		Candidate:      address,
		Authorize:      auth,
		DeadlineHeight: deadline.ToInt(),
		Signature:      sig,
	})

	return nil
}

// ProposeValidator 投票添加或者删除共识节点
func (api *API) ProposeValidator(id string, auth bool, deadline *hexutil.Big, sig hexutil.Bytes) error {
	_, err := models.IDFromString(id)
	if err != nil {
		return err
	}

	enc, _ := codec.Coder().Encode([]interface{}{
		"propose",
		id,
		auth,
		deadline.ToInt(),
	})

	manager, err := api.checkApiVote(enc, sig, deadline)
	if err != nil {
		return err
	}

	api.b.voteManager.setVVote(id, &ValidatorVote{
		Manager:        manager,
		Validator:      id,
		Authorize:      auth,
		DeadlineHeight: deadline.ToInt(),
		Signature:      sig,
	})

	return nil
}

// CheckApiVote 检查投票信息，如果正确，返回签名的管理员账户，否则返回错误
func (api *API) checkApiVote(data []byte, sig []byte, deadline *hexutil.Big) (types.Address, error) {
	signResult := new(signature.SignResult)
	err := signResult.Deserialize(sig)
	if err != nil {
		return types.Address{}, err
	}
	manager, err := ecrecoverVote(api.b.nodeKey, data, signResult)
	if err != nil {
		return types.Address{}, err
	}

	logger.Trace("vote by manager", "manager", manager.Hex())

	number := rpc.LatestBlockNumber
	snap, err := api.GetSnapshot(&number)
	if err != nil {
		return types.Address{}, err
	}

	// 小于当前区块，直接返回错误
	if deadline.ToInt().Cmp(new(big.Int).SetInt64(number.Int64())) < 0 {
		return types.Address{}, errors.New("invalid deadline block number")
	}

	if !snap.voteSnapshot.isManager(manager) {
		return types.Address{}, pbftProtocol.ErrUnauthorized
	}

	return manager, nil
}
