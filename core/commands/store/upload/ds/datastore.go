package ds

import (
	"github.com/tron-us/protobuf/proto"

	ds "github.com/ipfs/go-datastore"
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
