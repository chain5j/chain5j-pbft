// Package pbft
//
// @author: xwc1125
package pbft

import (
	"fmt"
	"github.com/chain5j/chain5j-pkg/database/kvstore"
	"github.com/chain5j/chain5j-protocol/protocol"
)

type option func(f *pbftEngine) error

func apply(f *pbftEngine, opts ...option) error {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(f); err != nil {
			return fmt.Errorf("option apply err:%v", err)
		}
	}
	return nil
}

func WithConfig(config protocol.Config) option {
	return func(f *pbftEngine) error {
		f.config = config
		return nil
	}
}

func WithNodeKey(nodeKey protocol.NodeKey) option {
	return func(f *pbftEngine) error {
		f.nodeKey = nodeKey
		return nil
	}
}

func WithBroadcaster(broadcaster protocol.Broadcaster) option {
	return func(f *pbftEngine) error {
		f.broadcaster = broadcaster
		return nil
	}
}

func WithBlockReader(blockReader protocol.BlockReader) option {
	return func(f *pbftEngine) error {
		f.blockReader = blockReader
		return nil
	}
}

func WithDatabaseReader(dbReader protocol.DatabaseReader) option {
	return func(f *pbftEngine) error {
		f.dbReader = dbReader
		return nil
	}
}

func WithKVDB(db kvstore.Database) option {
	return func(f *pbftEngine) error {
		f.db = db
		return nil
	}
}
