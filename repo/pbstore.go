package repo

import (
	"fmt"
	"github.com/ipfs/go-datastore"
	"github.com/tron-us/protobuf/proto"
)

func Get(d datastore.Datastore, k string, m interface{}) (interface{}, error) {
	v, err := d.Get(datastore.NewKey(k))
	if err != nil {
		return nil, err
	}
	if msg, ok := m.(proto.Message); ok {
		if err := proto.Unmarshal(v, msg); err != nil {
			return nil, err
		}
		return msg, nil
	}
	return nil, fmt.Errorf("invalid param:%v", m)
}

func Put(d datastore.Datastore, k string, v proto.Message) error {
	bytes, err := proto.Marshal(v)
	if err != nil {
		return err
	}
	return d.Put(datastore.NewKey(k), bytes)
}
