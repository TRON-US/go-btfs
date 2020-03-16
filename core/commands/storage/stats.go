package storage

import (
	"context"

	"github.com/TRON-US/go-btfs/core"

	nodepb "github.com/tron-us/go-btfs-common/protos/node"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"
)

const (
	HostStatStorePrefix = "/host_stats/" // from btfs-hub
)

// GetHostStatsFromDatastore retrieves host storage stats based on node id
func GetHostStatsFromDatastore(ctx context.Context, node *core.IpfsNode, nodeId string) (*nodepb.StorageStat_Host, error) {
	rds := node.Repo.Datastore()
	qr, err := rds.Get(GetHostStatStorageKey(nodeId))
	if err != nil {
		return nil, err
	}
	var hs nodepb.StorageStat_Host
	err = proto.Unmarshal(qr, &hs)
	if err != nil {
		return nil, err
	}
	return &hs, nil
}

func GetHostStatStorageKey(pid string) ds.Key {
	return newKeyHelper(HostStatStorePrefix, pid)
}

// SaveHostStatsIntoDatastore overwrites host storage stats based on node id
func SaveHostStatsIntoDatastore(ctx context.Context, node *core.IpfsNode, nodeId string,
	stats *nodepb.StorageStat_Host) error {
	rds := node.Repo.Datastore()
	b, err := proto.Marshal(stats)
	if err != nil {
		return err
	}
	err = rds.Put(GetHostStatStorageKey(nodeId), b)
	if err != nil {
		return err
	}
	return nil
}
