package renter

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/storage/ds"
	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	"github.com/orcaman/concurrent-map"
	"github.com/tron-us/go-btfs-common/protos/guard"
	shardpb "github.com/tron-us/go-btfs-common/protos/storage/shard"
	"github.com/tron-us/protobuf/proto"
)

const (
	shardInMemKey           = "/btfs/%s/v0.0.1/renter/sessions/%s/shards/%s"
	shardStatusKey          = "/btfs/%s/v0.0.1/renter/sessions/%s/shards/%s/status"
	shardMetadataKey        = "/btfs/%s/v0.0.1/renter/sessions/%s/shards/%s/metadata"
	shardContractsKey       = "/btfs/%s/v0.0.1/renter/sessions/%s/shards/%s/contracts"
	shardSignedContractsKey = "/btfs/%s/v0.0.1/renter/sessions/%s/shards/%s/signed-contracts"
)

var (
	shardFsmEvents = fsm.Events{
		{Name: "toInit", Src: []string{""}, Dst: "init"},
		{Name: "toContract", Src: []string{"init"}, Dst: "contract"},
		{Name: "toComplete", Src: []string{"contract"}, Dst: "complete"},
		{Name: "toError", Src: []string{"init", "contract"}, Dst: "error"},
	}
	shardsInMem = cmap.New()
)

type Shard struct {
	ctx       context.Context
	step      chan interface{}
	fsm       *fsm.FSM
	peerId    string
	sessionId string
	shardHash string
	ds        datastore.Datastore
}

func GetShard(ctx context.Context, ds datastore.Datastore, peerId string, sessionId string,
	shardHash string) (*Shard, error) {
	k := fmt.Sprintf(shardInMemKey, peerId, sessionId, shardHash)
	var s *Shard
	if tmp, ok := shardsInMem.Get(k); ok {
		s = tmp.(*Shard)
	} else {
		s = &Shard{
			ctx:       ctx,
			ds:        ds,
			peerId:    peerId,
			sessionId: sessionId,
			shardHash: shardHash,
		}
		s.fsm = fsm.NewFSM("",
			shardFsmEvents,
			fsm.Callbacks{
				"enter_state": s.enterState,
			})
		shardsInMem.Set(k, s)
	}
	status, err := s.Status()
	if err != nil {
		return nil, err
	}
	s.fsm.SetState(status.Status)
	return s, nil
}

func (s *Shard) enterState(e *fsm.Event) {
	fmt.Println("e.Dst", e.Dst)
	switch e.Dst {
	case "init":
		s.doInit(e.Args[0].(*shardpb.Metadata))
	case "contract":
		s.doContract(e.Args[0].(*shardpb.Contracts))
	case "complete":
		s.doComplete(e.Args[0].([]byte), e.Args[1].(*guard.Contract))
	case "error":
		s.doError(e.Args[0].(error))
	}
}

func (s *Shard) ToInit(md *shardpb.Metadata) error {
	return s.fsm.Event("toInit", md)
}

func (s *Shard) ToContract(sc *shardpb.Contracts) error {
	return s.fsm.Event("toContract", sc)
}

func (s *Shard) ToComplete(escrowContractBytes []byte, guardContract *guard.Contract) error {
	return s.fsm.Event("toComplete", escrowContractBytes, guardContract)
}

func (s *Shard) ToError(err error) error {
	return s.fsm.Event("toError", err)
}

func (s *Shard) Timeout() error {
	return s.fsm.Event("toError")
}

func (s *Shard) doInit(md *shardpb.Metadata) error {
	ks := []string{
		fmt.Sprintf(shardStatusKey, s.peerId, s.sessionId, s.shardHash),
		fmt.Sprintf(shardMetadataKey, s.peerId, s.sessionId, s.shardHash),
	}
	vs := []proto.Message{
		&shardpb.Status{
			Status:  "init",
			Message: "",
		}, md,
	}
	return ds.Batch(s.ds, ks, vs)
}

func (s *Shard) doContract(sc *shardpb.Contracts) error {
	ks := []string{
		fmt.Sprintf(shardStatusKey, s.peerId, s.sessionId, s.shardHash),
		fmt.Sprintf(shardContractsKey, s.peerId, s.sessionId, s.shardHash),
	}
	vs := []proto.Message{
		&shardpb.Status{
			Status:  "contract",
			Message: "",
		}, sc,
	}
	return ds.Batch(s.ds, ks, vs)
}

func (s *Shard) doComplete(signedEscrowContractBytes []byte, gc *guard.Contract) error {
	status := &shardpb.Status{
		Status:  "complete",
		Message: "",
	}
	ks := []string{
		fmt.Sprintf(shardStatusKey, s.peerId, s.sessionId, s.shardHash),
		fmt.Sprintf(shardSignedContractsKey, s.peerId, s.sessionId, s.shardHash),
	}
	vs := []proto.Message{
		status,
		&shardpb.SingedContracts{
			SignedEscrowContract: signedEscrowContractBytes,
			GuardContract:        gc,
		},
	}
	return ds.Batch(s.ds, ks, vs)
}

func (s *Shard) doError(err error) error {
	status := &shardpb.Status{
		Status:  "error",
		Message: err.Error(),
	}
	return ds.Save(s.ds, fmt.Sprintf(shardStatusKey, s.peerId, s.sessionId, s.shardHash), status)
}

func (s *Shard) Status() (*shardpb.Status, error) {
	st := &shardpb.Status{}
	err := ds.Get(s.ds, fmt.Sprintf(shardStatusKey, s.peerId, s.sessionId, s.shardHash), st)
	if err == datastore.ErrNotFound {
		return st, nil
	}
	return st, err
}

func (s *Shard) Metadata() (*shardpb.Metadata, error) {
	md := &shardpb.Metadata{}
	err := ds.Get(s.ds, fmt.Sprintf(shardMetadataKey, s.peerId, s.sessionId, s.shardHash), md)
	if err == datastore.ErrNotFound {
		return md, nil
	}
	return md, err
}

func (s *Shard) Congtracts() (*shardpb.Contracts, error) {
	cg := &shardpb.Contracts{}
	err := ds.Get(s.ds, fmt.Sprintf(shardContractsKey, s.peerId, s.sessionId, s.shardHash), cg)
	if err == datastore.ErrNotFound {
		return cg, nil
	}
	return cg, err
}

func (s *Shard) SignedCongtracts() (*shardpb.SingedContracts, error) {
	cg := &shardpb.SingedContracts{}
	err := ds.Get(s.ds, fmt.Sprintf(shardSignedContractsKey, s.peerId, s.sessionId, s.shardHash), cg)
	if err == datastore.ErrNotFound {
		return cg, nil
	}
	return cg, err
}
