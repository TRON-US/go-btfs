package upload

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/commands/store/upload/ds"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"
	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/tron-us/protobuf/proto"
	"strings"
	"time"
)

const (
	renterShardKey          = "/btfs/%s/renter/shards/%s/"
	renterShardsInMemKey    = renterShardKey
	renterShardStatusKey    = renterShardKey + "status"
	renterShardContractsKey = renterShardKey + "contracts"

	rshInitStatus      = "init"
	rshContractStatus  = "contract"
	rshErrorStatus     = "error"
	rshToContractEvent = "to-contract"
)

var (
	renterShardFsmEvents = fsm.Events{
		{Name: rshToContractEvent, Src: []string{rshInitStatus}, Dst: rshContractStatus},
	}
	renterShardsInMem = cmap.New()
)

type RenterShard struct {
	peerId string
	ssId   string
	hash   string
	fsm    *fsm.FSM
	ctx    context.Context
	ds     datastore.Datastore
}

func GetRenterShard(ctxParams *ContextParams, ssId string, hash string) (*RenterShard, error) {
	contractId := getContractId(ssId, hash)
	k := fmt.Sprintf(renterShardsInMemKey, ctxParams.n.Identity.Pretty(), contractId)
	var rs *RenterShard
	if tmp, ok := renterShardsInMem.Get(k); ok {
		rs = tmp.(*RenterShard)
	} else {
		ctx, _ := storage.NewGoContext(ctxParams.ctx)
		rs = &RenterShard{
			peerId: ctxParams.n.Identity.Pretty(),
			ssId:   ssId,
			hash:   hash,
			ctx:    ctx,
			ds:     ctxParams.n.Repo.Datastore(),
		}
		renterShardsInMem.Set(k, rs)
	}
	status, err := rs.status()
	if err != nil {
		return nil, err
	}
	if rs.fsm == nil && status.Status == rshInitStatus {
		rs.fsm = fsm.NewFSM(status.Status, renterShardFsmEvents, fsm.Callbacks{
			"enter_state": rs.enterState,
		})
	}
	return rs, nil
}

func (rs *RenterShard) enterState(e *fsm.Event) {
	//log.Info("shard: %s:%s enter status: %s\n", rs.ssId, rs.hash, e.Dst)
	fmt.Printf("shard: %s:%s enter status: %s\n", rs.ssId, rs.hash, e.Dst)
	switch e.Dst {
	case rshContractStatus:
		rs.doContract(e.Args[0].([]byte), e.Args[1].([]byte))
	}
}

func (rs *RenterShard) status() (*renterpb.RenterShardStatus, error) {
	status := new(renterpb.RenterShardStatus)
	contractId := getContractId(rs.ssId, rs.hash)
	k := fmt.Sprintf(renterShardStatusKey, rs.peerId, contractId)
	err := ds.Get(rs.ds, k, status)
	if err == datastore.ErrNotFound {
		status = &renterpb.RenterShardStatus{
			Status: rssInitStatus,
		}
		//ignore error
		_ = ds.Save(rs.ds, k, status)
	} else if err != nil {
		return nil, err
	}
	return status, nil
}

func getContractId(ssId string, shardHash string) (contractId string) {
	return fmt.Sprintf("%s:%s", ssId, shardHash)
}

func splitContractId(contractId string) (ssId string, shardHash string) {
	splits := strings.Split(contractId, ":")
	ssId = splits[0]
	shardHash = splits[1]
	return
}

func (rs *RenterShard) doContract(signedEscrowContract []byte, signedGuardContract []byte) error {
	status := &renterpb.RenterShardStatus{
		Status:      rshContractStatus,
		LastUpdated: time.Now().UTC(),
	}
	signedContracts := &renterpb.RenterShardSignedContracts{
		SignedEscrowContract: signedEscrowContract,
		SignedGuardContract:  signedGuardContract,
	}
	contractId := getContractId(rs.ssId, rs.hash)
	return ds.Batch(rs.ds, []string{
		fmt.Sprintf(renterShardStatusKey, rs.peerId, contractId),
		fmt.Sprintf(renterShardContractsKey, rs.peerId, contractId),
	}, []proto.Message{
		status, signedContracts,
	})
}

func (rs *RenterShard) contract(signedEscrowContract []byte, signedGuardContract []byte) error {
	return rs.fsm.Event(rshToContractEvent, signedEscrowContract, signedGuardContract)
}

func (rs *RenterShard) contracts() (*renterpb.RenterShardSignedContracts, error) {
	contracts := &renterpb.RenterShardSignedContracts{}
	err := ds.Get(rs.ds, fmt.Sprintf(renterShardContractsKey, rs.peerId, getContractId(rs.ssId, rs.hash)), contracts)
	if err == datastore.ErrNotFound {
		return contracts, nil
	}
	return contracts, err
}
