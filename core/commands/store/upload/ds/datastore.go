package ds

import (
	"fmt"
	shardpb "github.com/TRON-US/go-btfs/protos/shard"
	"github.com/ipfs/go-datastore/query"
	"github.com/tron-us/protobuf/proto"
	"strings"

	ds "github.com/ipfs/go-datastore"
)

const (
	limitList = 100
)

func Batch(d ds.Datastore, keys []string, vals []proto.Message) error {
	batch := ds.NewBasicBatch(d)
	for i, k := range keys {
		bytes, err := proto.Marshal(vals[i])
		if err != nil {
			return err
		}
		batch.Put(ds.NewKey(k), bytes)
	}
	return batch.Commit()
}

func Save(d ds.Datastore, key string, val proto.Message) error {
	bytes, err := proto.Marshal(val)
	if err != nil {
		return err
	}
	return d.Put(ds.NewKey(key), bytes)
}

func Get(d ds.Datastore, key string, m proto.Message) error {
	bytes, err := d.Get(ds.NewKey(key))
	if err != nil {
		return err
	}
	err = proto.Unmarshal(bytes, m)
	return err
}

func List(d ds.Datastore, prefix string, limit int) ([]proto.Message, error) {
	if limit <= 0 || limit > limitList {
		limit = limitList
	}
	results, err := d.Query(query.Query{
		Prefix:  prefix,
		Filters: []query.Filter{},
		Limit:   limit,
	})
	if err != nil {
		return nil, err
	}
	for nxt, ok := results.NextSync(); ok; nxt, ok = results.NextSync() {
		if strings.Contains(nxt.Key, "/shards/") && strings.Contains(nxt.Key, "/signed-contracts") {
			bytes := nxt.Value
			sc := &shardpb.SingedContracts{}
			err := proto.Unmarshal(bytes, sc)
			if err != nil {
				continue
			}
			fmt.Println("key", nxt.Key, "value", sc.GuardContract.FileHash, sc.GuardContract.ShardHash)

		}
	}
	return ms, nil
}
