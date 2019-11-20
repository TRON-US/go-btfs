package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TRON-US/go-btfs/core"
	"github.com/tron-us/go-btfs-common/info"

	"github.com/ipfs/go-datastore/query"
)

const (
	HostStorePrefix = "/hosts/" // from btfs-hub

	HostModeDefault = "score"
	HostModeAll     = "all"
)

// GetHostsFromDatastore retrieves `num` hosts from the datastore, if not enough hosts are
// available, return an error instead of partial return.
func GetHostsFromDatastore(ctx context.Context, node *core.IpfsNode, mode string, num int) ([]*info.Node, error) {
	// get host list from storage
	rds := node.Repo.Datastore()
	qr, err := rds.Query(query.Query{
		Prefix: HostStorePrefix + mode,
		Orders: []query.Order{query.OrderByKey{}},
	})
	if err != nil {
		return nil, err
	}
	// Add as many hosts as available
	var hosts []*info.Node
	for r := range qr.Next() {
		if r.Error != nil {
			return nil, r.Error
		}
		var ni info.Node
		err := json.Unmarshal(r.Entry.Value, &ni)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, &ni)
	}
	// we can re-use hosts, but for higher availability, we choose to have the
	// greater than `num assumption
	if len(hosts) < num {
		return nil, fmt.Errorf("there are not enough locally stored hosts")
	}
	return hosts, nil
}
