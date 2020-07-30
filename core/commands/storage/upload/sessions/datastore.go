package sessions

import (
	"strings"

	"github.com/tron-us/protobuf/proto"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

func Batch(d ds.Datastore, keys []string, vals []proto.Message) error {
	batch := ds.NewBasicBatch(d)
	for i, k := range keys {
		if vals[i] == nil {
			batch.Delete(ds.NewKey(k))
			continue
		}
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

func Remove(d ds.Datastore, key string) error {
	return d.Delete(ds.NewKey(key))
}

func List(d ds.Datastore, prefix string, substrInKey ...string) ([][]byte, error) {
	vs := make([][]byte, 0)
	results, err := d.Query(query.Query{
		Prefix:  prefix,
		Filters: []query.Filter{},
	})
	if err != nil {
		return nil, err
	}
	for entry := range results.Next() {
		contains := true
		for _, substr := range substrInKey {
			contains = contains && strings.Contains(entry.Key, substr)
		}
		if contains {
			value := entry.Value
			vs = append(vs, value)
		}
	}
	return vs, nil
}

func ListKeys(d ds.Datastore, prefix string, substrInKey ...string) ([]string, error) {
	ks := make([]string, 0)
	results, err := d.Query(query.Query{
		Prefix:   prefix,
		Filters:  []query.Filter{},
		KeysOnly: true,
	})
	if err != nil {
		return nil, err
	}
	for entry := range results.Next() {
		contains := true
		for _, substr := range substrInKey {
			contains = contains && strings.Contains(entry.Key, substr)
		}
		if contains {
			ks = append(ks, entry.Key)
		}
	}
	return ks, nil
}
