package upload

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	shardpb "github.com/TRON-US/go-btfs/protos/shard"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	"github.com/tron-us/protobuf/proto"

	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	hostShardPrefix       = "/btfs/%s/renter/shards/"
	hostShardKey          = hostShardPrefix + "%s/"
	hostShardsInMemKey    = hostShardKey
	hostShardStatusKey    = hostShardKey + "status"
	hostShardContractsKey = hostShardKey + "contracts"

	hshInitStatus     = "init"
	hshContractStatus = "contract"
	hshCompleteStatus = "complete"
	hshErrorStatus    = "error"

	hshToContractEvent = "to-contract"
	hshToCompleteEvent = "to-complete"
	hshToErrorEvent    = "to-error"
)

var (
	hostShardFsmEvents = fsm.Events{
		{Name: hshToContractEvent, Src: []string{hshInitStatus}, Dst: hshContractStatus},
		{Name: hshToCompleteEvent, Src: []string{hshContractStatus}, Dst: hshCompleteStatus},
		{Name: hshToErrorEvent, Src: []string{hshInitStatus, hshContractStatus}, Dst: hshToErrorEvent},
	}
	hostShardsInMem = cmap.New()
)

type HostShard struct {
	peerId     string
	contractId string
	fsm        *fsm.FSM
	ctx        context.Context
	ds         datastore.Datastore
}

func GetHostShard(ctxParams *ContextParams, contractId string) (*HostShard, error) {
	k := fmt.Sprintf(hostShardsInMemKey, ctxParams.n.Identity.Pretty(), contractId)
	var hs *HostShard
	if tmp, ok := hostShardsInMem.Get(k); ok {
		hs = tmp.(*HostShard)
	} else {
		ctx, _ := storage.NewGoContext(ctxParams.ctx)
		hs = &HostShard{
			peerId:     ctxParams.n.Identity.Pretty(),
			contractId: contractId,
			ctx:        ctx,
			ds:         ctxParams.n.Repo.Datastore(),
		}
		hostShardsInMem.Set(k, hs)
	}
	status, err := hs.status()
	if err != nil {
		return nil, err
	}
	if hs.fsm == nil && status.Status == hshInitStatus {
		hs.fsm = fsm.NewFSM(status.Status, hostShardFsmEvents, fsm.Callbacks{
			"enter_state": hs.enterState,
		})
	}
	return hs, nil
}

func (hs *HostShard) enterState(e *fsm.Event) {
	//TODO
	fmt.Printf("shard: %s enter status: %s\n", hs.contractId, e.Dst)
	switch e.Dst {
	case hshContractStatus:
		hs.doContract(e.Args[0].([]byte), e.Args[1].(*guardpb.Contract))
	}
}

func (hs *HostShard) status() (*shardpb.Status, error) {
	status := new(shardpb.Status)
	k := fmt.Sprintf(hostShardStatusKey, hs.peerId, hs.contractId)
	err := Get(hs.ds, k, status)
	if err == datastore.ErrNotFound {
		status = &shardpb.Status{
			Status: hshInitStatus,
		}
		//ignore error
		_ = Save(hs.ds, k, status)
	} else if err != nil {
		return nil, err
	}
	return status, nil
}

func (hs *HostShard) doContract(signedEscrowContract []byte, signedGuardContract *guardpb.Contract) error {
	status := &shardpb.Status{
		Status: hshContractStatus,
	}
	signedContracts := &shardpb.SignedContracts{
		SignedEscrowContract: signedEscrowContract,
		SignedGuardContract:  signedGuardContract,
	}
	return Batch(hs.ds, []string{
		fmt.Sprintf(hostShardStatusKey, hs.peerId, hs.contractId),
		fmt.Sprintf(hostShardContractsKey, hs.peerId, hs.contractId),
	}, []proto.Message{
		status, signedContracts,
	})
}

func (hs *HostShard) contract(signedEscrowContract []byte, signedGuardContract *guardpb.Contract) error {
	return hs.fsm.Event(hshToContractEvent, signedEscrowContract, signedGuardContract)
}

func (hs *HostShard) complete() error {
	return hs.fsm.Event(hshToCompleteEvent)
}

func (hs *HostShard) contracts() (*shardpb.SignedContracts, error) {
	contracts := &shardpb.SignedContracts{}
	err := Get(hs.ds, fmt.Sprintf(hostShardContractsKey, hs.peerId, hs.contractId), contracts)
	if err == datastore.ErrNotFound {
		return contracts, nil
	}
	return contracts, err
}
