package upload

import (
	"context"
	"fmt"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"strings"

	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/guard"
	shardpb "github.com/TRON-US/go-btfs/protos/shard"

	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/protobuf/proto"

	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	renterShardPrefix       = "/btfs/%s/renter/shards/"
	renterShardKey          = renterShardPrefix + "%s/"
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
	log.Infof("shard: %s:%s enter status: %s", rs.ssId, rs.hash, e.Dst)
	switch e.Dst {
	case rshContractStatus:
		rs.doContract(e.Args[0].([]byte), e.Args[1].(*guardpb.Contract))
	}
}

func (rs *RenterShard) status() (*shardpb.Status, error) {
	status := new(shardpb.Status)
	contractId := getContractId(rs.ssId, rs.hash)
	k := fmt.Sprintf(renterShardStatusKey, rs.peerId, contractId)
	err := Get(rs.ds, k, status)
	if err == datastore.ErrNotFound {
		status = &shardpb.Status{
			Status: rshInitStatus,
		}
		//ignore error
		_ = Save(rs.ds, k, status)
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

// ExtractSessionIDFromContractID takes the first segment separated by ","
// and returns it as the session id. If in an old format, i.e. did not
// have a session id, return an error.
func extractSessionIDFromContractID(contractID string) (string, error) {
	ids := strings.Split(contractID, ":")
	if len(ids) != 2 {
		return "", fmt.Errorf("bad contract id: fewer than 2 segments")
	}
	if len(ids[0]) != 36 {
		return "", fmt.Errorf("invalid session id within contract id")
	}
	return ids[0], nil
}

func (rs *RenterShard) doContract(signedEscrowContract []byte, signedGuardContract *guardpb.Contract) error {
	status := &shardpb.Status{
		Status: rshContractStatus,
	}
	signedContracts := &shardpb.SignedContracts{
		SignedEscrowContract: signedEscrowContract,
		SignedGuardContract:  signedGuardContract,
	}
	contractId := getContractId(rs.ssId, rs.hash)
	return Batch(rs.ds, []string{
		fmt.Sprintf(renterShardStatusKey, rs.peerId, contractId),
		fmt.Sprintf(renterShardContractsKey, rs.peerId, contractId),
	}, []proto.Message{
		status, signedContracts,
	})
}

func (rs *RenterShard) contract(signedEscrowContract []byte, signedGuardContract *guardpb.Contract) error {
	return rs.fsm.Event(rshToContractEvent, signedEscrowContract, signedGuardContract)
}

func (rs *RenterShard) contracts() (*shardpb.SignedContracts, error) {
	contracts := &shardpb.SignedContracts{}
	err := Get(rs.ds, fmt.Sprintf(renterShardContractsKey, rs.peerId, getContractId(rs.ssId, rs.hash)), contracts)
	if err == datastore.ErrNotFound {
		return contracts, nil
	}
	return contracts, err
}

func ListShardsContracts(d datastore.Datastore, peerId string, role string) ([]*shardpb.SignedContracts, error) {
	var k string
	if k = fmt.Sprintf(renterShardPrefix, peerId); role == nodepb.ContractStat_HOST.String() {
		k = fmt.Sprintf(hostShardPrefix, peerId)
	}
	vs, err := List(d, k, "/signed-contracts")
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

// SaveShardsContracts persists updated guard contracts from upstream, if an existing entry
// is not available, then an empty signed escrow contract is inserted along with the
// new guard contract.
func SaveShardsContracts(ds datastore.Datastore, scs []*shardpb.SignedContracts,
	gcs []*guardpb.Contract, peerID, role string) ([]*shardpb.SignedContracts, []string, error) {
	var ks []string
	var vs []proto.Message
	gmap := map[string]*guardpb.Contract{}
	for _, g := range gcs {
		gmap[g.ContractId] = g
	}
	activeShards := map[string]bool{}      // active shard hash -> has one file hash (bool)
	activeFiles := map[string]bool{}       // active file hash -> has one shard hash (bool)
	invalidShards := map[string][]string{} // invalid shard hash -> (maybe) invalid file hash list
	for _, c := range scs {
		// only append the updated contracts
		if gc, ok := gmap[c.SignedGuardContract.ContractId]; ok {
			ks = append(ks, fmt.Sprintf(renterShardContractsKey, peerID, c.SignedGuardContract.ContractId))
			// update
			c.SignedGuardContract = gc
			vs = append(vs, c)
			delete(gmap, c.SignedGuardContract.ContractId)

			// mark stale files if no longer active (must be synced to become inactive)
			if _, ok := guard.ContractFilterMap["active"][gc.State]; !ok {
				invalidShards[gc.ShardHash] = append(invalidShards[gc.ShardHash], gc.FileHash)
			}
		} else {
			activeShards[c.SignedGuardContract.ShardHash] = true
			activeFiles[c.SignedGuardContract.FileHash] = true
		}
	}
	// append what's left in guard map as new contracts
	for contractID, gc := range gmap {
		ks = append(ks, fmt.Sprintf(renterShardContractsKey, peerID, contractID))
		// add a new (guard contract only) signed contracts
		c := &shardpb.SignedContracts{SignedGuardContract: gc}
		scs = append(scs, c)
		vs = append(vs, c)

		// mark stale files if no longer active (must be synced to become inactive)
		if _, ok := guard.ContractFilterMap["active"][gc.State]; !ok {
			invalidShards[gc.ShardHash] = append(invalidShards[gc.ShardHash], gc.FileHash)
		} else {
			activeShards[gc.ShardHash] = true
			activeFiles[gc.FileHash] = true
		}
	}
	if len(ks) > 0 {
		err := Batch(ds, ks, vs)
		if err != nil {
			return nil, nil, err
		}
	}
	var staleHashes []string
	// compute what's stale
	for ish, fhs := range invalidShards {
		if _, ok := activeShards[ish]; ok {
			// other files are referring to this hash, skip
			continue
		}
		for _, fh := range fhs {
			if _, ok := activeFiles[fh]; !ok {
				// file does not have other active shards
				staleHashes = append(staleHashes, fh)
			}
		}
		// TODO: Cannot prematurally remove shard because it's indirectly pinned
		// Need a way to disassociated indirect pins from parent...
		// remove hash anyway even if no file is getting removed
		//staleHashes = append(staleHashes, ish)
	}
	return scs, staleHashes, nil
}
