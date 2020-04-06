package ds

import (
	"context"
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/escrow"
	shardpb "github.com/TRON-US/go-btfs/protos/shard"

	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/tron-us/protobuf/proto"
)

const (
	shardsKey               = sessionsPrefix + "%s/shards/"
	shardKeyPrefix          = shardsKey + "%s/"
	shardInMemKey           = shardKeyPrefix
	shardStatusKey          = shardKeyPrefix + "status"
	shardSignedContractsKey = shardKeyPrefix + "signed-contracts"
)

var (
	shardFsmEvents = fsm.Events{
		{Name: "e-contract", Src: []string{"init"}, Dst: "contract"},
		{Name: "e-complete", Src: []string{"contract"}, Dst: "complete"},
	}
	shardsInMem = cmap.New()
)

type Shard struct {
	peerId    string
	sessionId string
	shardHash string
	role      string
	ds        datastore.Datastore
	ctx       context.Context
	fsm       *fsm.FSM
}

type ShardInitParams struct {
	Context   context.Context
	Datastore datastore.Datastore
}

func GetShard(peerId string, role string, sessionId string, shardHash string, params *ShardInitParams) (*Shard, error) {
	k := fmt.Sprintf(shardInMemKey, peerId, role, sessionId, shardHash)
	var s *Shard
	if tmp, ok := shardsInMem.Get(k); ok {
		s = tmp.(*Shard)
		status, err := s.Status()
		if err != nil {
			return nil, err
		}
		s.fsm.SetState(status.Status)
	} else {
		ctx, _ := storage.NewGoContext(params.Context)
		s = &Shard{
			ctx:       ctx,
			ds:        params.Datastore,
			role:      role,
			peerId:    peerId,
			sessionId: sessionId,
			shardHash: shardHash,
		}
		s.fsm = fsm.NewFSM("init",
			shardFsmEvents,
			fsm.Callbacks{
				"enter_state": s.enterState,
			})
		s.Init()
		shardsInMem.Set(k, s)
	}
	return s, nil
}

func (s *Shard) Init() error {
	ks := []string{
		fmt.Sprintf(shardStatusKey, s.peerId, s.role, s.sessionId, s.shardHash),
	}
	vs := []proto.Message{
		&shardpb.Status{
			Status:  "init",
			Message: "",
		},
	}
	return Batch(s.ds, ks, vs)
}

func (s *Shard) enterState(e *fsm.Event) {
	var err error
	switch e.Dst {
	case "contract":
		s.doContract(e.Args[0].(*shardpb.SignedContracts))
	default:
		err = Save(s.ds, fmt.Sprintf(shardStatusKey, s.peerId, s.role, s.sessionId, s.shardHash), &shardpb.Status{
			Status: e.Dst,
		})
	}
	if err != nil {
		log.Error(err)
	}
}

func (s *Shard) Contract(sc *shardpb.SignedContracts) {
	s.fsm.Event("e-contract", sc)
}

func (s *Shard) Complete() {
	s.fsm.Event("e-complete")
}

func (s *Shard) doContract(sc *shardpb.SignedContracts) error {
	ks := []string{
		fmt.Sprintf(shardStatusKey, s.peerId, s.role, s.sessionId, s.shardHash),
		fmt.Sprintf(shardSignedContractsKey, s.peerId, s.role, s.sessionId, s.shardHash),
	}
	vs := []proto.Message{
		&shardpb.Status{
			Status:  "contract",
			Message: "",
		}, sc,
	}
	return Batch(s.ds, ks, vs)
}

// SaveShardsContracts persists updated guard contracts from upstream, if an existing entry
// is not available, then an empty signed escrow contract is inserted along with the
// new guard contract.
func SaveShardsContracts(ds datastore.Datastore, scs []*shardpb.SignedContracts,
	gcs []*guardpb.Contract, peerID, role string) ([]*shardpb.SignedContracts, error) {
	var ks []string
	var vs []proto.Message
	gmap := map[string]*guardpb.Contract{}
	for _, g := range gcs {
		gmap[g.ContractId] = g
	}
	for _, c := range scs {
		// only append the updated contracts
		if gc, ok := gmap[c.GuardContract.ContractId]; ok {
			sessionID, err := escrow.ExtractSessionIDFromContractID(c.GuardContract.ContractId)
			if err != nil {
				return nil, err
			}
			ks = append(ks, fmt.Sprintf(shardSignedContractsKey, peerID, role, sessionID, gc.ShardHash))
			// update
			c.GuardContract = gc
			vs = append(vs, c)
			delete(gmap, c.GuardContract.ContractId)
		}
	}
	// append what's left in guard map as new contracts
	for contractID, gc := range gmap {
		sessionID, err := escrow.ExtractSessionIDFromContractID(contractID)
		if err != nil {
			return nil, err
		}
		ks = append(ks, fmt.Sprintf(shardSignedContractsKey, peerID, role, sessionID, gc.ShardHash))
		// add a new (guard contract only) signed contracts
		c := &shardpb.SignedContracts{GuardContract: gc}
		scs = append(scs, c)
		vs = append(vs, c)
	}
	if len(ks) > 0 {
		err := Batch(ds, ks, vs)
		if err != nil {
			return nil, err
		}
	}
	return scs, nil
}

func (s *Shard) Status() (*shardpb.Status, error) {
	st := &shardpb.Status{}
	err := Get(s.ds, fmt.Sprintf(shardStatusKey, s.peerId, s.role, s.sessionId, s.shardHash), st)
	if err == datastore.ErrNotFound {
		return st, nil
	}
	return st, err
}

func (s *Shard) SignedCongtracts() (*shardpb.SignedContracts, error) {
	cg := &shardpb.SignedContracts{}
	err := Get(s.ds, fmt.Sprintf(shardSignedContractsKey, s.peerId, s.role, s.sessionId, s.shardHash), cg)
	if err == datastore.ErrNotFound {
		return cg, nil
	}
	return cg, err
}

func ListShardsContracts(d datastore.Datastore, peerId string, role string) ([]*shardpb.SignedContracts, error) {
	vs, err := List(d, fmt.Sprintf(sessionsPrefix, peerId, role), "/shards/", "/signed-contracts")
	if err != nil {
		return nil, err
	}
	contracts := make([]*shardpb.SignedContracts, 0)
	for _, v := range vs {
		sc := &shardpb.SignedContracts{}
		err := proto.Unmarshal(v, sc)
		if err != nil {
			log.Error(err)
			continue
		}
		contracts = append(contracts, sc)
	}
	return contracts, nil
}
