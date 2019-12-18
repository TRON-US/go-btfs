package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/hub"

	hubpb "github.com/tron-us/go-btfs-common/protos/hub"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

const (
	HostStorePrefix       = "/hosts/"        // from btfs-hub
	HostStorageInfoPrefix = "/host_storage/" // self or from network

	HostModeDefault = hub.HubModeScore
)

// GetHostsFromDatastore retrieves `num` hosts from the datastore, if not enough hosts are
// available, return an error instead of partial return.
// When num=0 it means unlimited.
func GetHostsFromDatastore(ctx context.Context, node *core.IpfsNode, mode string, num int) ([]*hubpb.Host, error) {
	// check mode: all = display everything
	if mode == hub.HubModeAll {
		mode = ""
	}
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
	var hosts []*hubpb.Host
	for r := range qr.Next() {
		if r.Error != nil {
			return nil, r.Error
		}
		var h hubpb.Host
		err := proto.Unmarshal(r.Entry.Value, &h)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, &h)
	}
	// we can re-use hosts, but for higher availability, we choose to have the
	// greater than `num assumption
	if num > 0 && len(hosts) < num {
		return nil, fmt.Errorf("there are not enough locally stored hosts")
	}
	fmt.Println("hosts:", len(hosts))
	return hosts, nil
}

func GetHostStorageKey(pid string) ds.Key {
	return newKeyHelper(HostStorageInfoPrefix, pid)
}

func newKeyHelper(kss ...string) ds.Key {
	return ds.NewKey(strings.Join(kss, ""))
}

// SaveHostsIntoDatastore overwrites (removes all existing) hosts and saves the updated
// hosts according to mode.
func SaveHostsIntoDatastore(ctx context.Context, node *core.IpfsNode, mode string, nodes []*hubpb.Host) error {
	rds := node.Repo.Datastore()

	// Dumb strategy right now: remove all existing and add the new ones
	// TODO: Update by timestamp and only overwrite updated
	qr, err := rds.Query(query.Query{Prefix: HostStorePrefix + mode})
	if err != nil {
		return err
	}

	for r := range qr.Next() {
		if r.Error != nil {
			return r.Error
		}
		err := rds.Delete(newKeyHelper(r.Entry.Key))
		if err != nil {
			return err
		}
	}

	for i, ni := range nodes {
		b, err := proto.Marshal(ni)
		if err != nil {
			return err
		}
		err = rds.Put(newKeyHelper(HostStorePrefix, mode, "/", fmt.Sprintf("%04d", i), "/", ni.NodeId), b)
		if err != nil {
			return err
		}
	}

	return nil
}
