package sessions

import (
	"strconv"
	"strings"
	"time"

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

func ListFiles(d ds.Datastore, prefix string) ([][]byte, time.Time, error) {
	var tmpStr string
	var latest = time.Unix(0, 0)
	vs := make([][]byte, 0)
	results, err := d.Query(query.Query{
		Prefix:  prefix,
		Filters: []query.Filter{},
	})
	if err != nil {
		return nil, latest, err
	}
	for entry := range results.Next() {
		key := entry.Key
		index := strings.LastIndex(key, "/")
		tsStr := key[index+1:]
		if strings.Compare(tsStr, tmpStr) >= 0 {
			tmpStr = tsStr
		}
		value := entry.Value
		vs = append(vs, value)
	}
	if tmpStr == "" {
		return nil, latest, nil
	}
	ts, err := strconv.ParseInt(tmpStr, 10, 64)
	if err != nil {
		return nil, latest, err
	}
	latest = time.Unix(ts, 0)
	return vs, latest, nil
}
