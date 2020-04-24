package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/rm"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	contractspb "github.com/TRON-US/go-btfs/protos/contracts"
	shardpb "github.com/TRON-US/go-btfs/protos/shard"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
)

var contractsLog = logging.Logger("storage/contracts")

const (
	contractsListOrderOptionName  = "order"
	contractsListStatusOptionName = "status"
	contractsListSizeOptionName   = "size"

	contractsKeyPrefix = "/btfs/%s/contracts/"
	hostContractsKey   = contractsKeyPrefix + "host"
	renterContractsKey = contractsKeyPrefix + "renter"
	payoutNotFoundErr  = "rpc error: code = Unknown desc = not found"

	guardTimeout = 360 * time.Second

	guardContractPageSize = 100
)

// Storage Contracts
//
// Includes sub-commands: sync, stat, list
var StorageContractsCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get node storage contracts info.",
		ShortDescription: `
This command get node storage contracts info respect to different roles.`,
	},
	Subcommands: map[string]*cmds.Command{
		"sync": storageContractsSyncCmd,
		"stat": storageContractsStatCmd,
		"list": storageContractsListCmd,
	},
}

// checkContractStatRole checks role argument strings against valid roles
// and returns the role type
func checkContractStatRole(roleArg string) (nodepb.ContractStat_Role, error) {
	if cr, ok := nodepb.ContractStat_Role_value[strings.ToUpper(roleArg)]; ok {
		return nodepb.ContractStat_Role(cr), nil
	}
	return 0, fmt.Errorf("invalid role: %s", roleArg)
}

// sub-commands: btfs storage contracts sync
var storageContractsSyncCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Synchronize contracts stats based on role.",
		ShortDescription: `
This command contracts stats based on role from network(hub) to local node data storage.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("role", true, false, "Role in BTFS storage network [host|renter|reserved]."),
	},
	RunTimeout: 60 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		role, err := checkContractStatRole(req.Arguments[0])
		if err != nil {
			return err
		}
		return SyncContracts(req.Context, n, req, env, role.String())
	},
}

// sub-commands: btfs storage contracts stat
var storageContractsStatCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get contracts stats based on role.",
		ShortDescription: `
This command get contracts stats based on role from the local node data store.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("role", true, false, "Role in BTFS storage network [host|renter|reserved]."),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cr, err := checkContractStatRole(req.Arguments[0])
		if err != nil {
			return err
		}
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		contracts, err := ListContracts(n.Repo.Datastore(), cr.String())
		if err != nil {
			return err
		}
		activeStates := helper.ContractFilterMap["active"]
		invalidStates := helper.ContractFilterMap["invalid"]
		var activeCount, totalPaid, totalUnpaid int64
		var first, last time.Time
		for _, c := range contracts {
			if _, ok := activeStates[c.Status]; ok {
				activeCount++
				// Count outstanding on only active ones
				totalUnpaid += c.CompensationOutstanding
			}
			// Count all paid on all contracts
			totalPaid += c.CompensationPaid
			// Count start/end for all non-invalid ones
			if _, ok := invalidStates[c.Status]; !ok {
				if (first == time.Time{}) || c.StartTime.Before(first) {
					first = c.StartTime
				}
				if (last == time.Time{}) || c.EndTime.After(last) {
					last = c.EndTime
				}
			}
		}
		data := &nodepb.ContractStat{
			ActiveContractNum:       activeCount,
			CompensationPaid:        totalPaid,
			CompensationOutstanding: totalUnpaid,
			FirstContractStart:      first,
			LastContractEnd:         last,
			Role:                    cr,
		}
		return cmds.EmitOnce(res, data)
	},
	Type: nodepb.ContractStat{},
}

var (
	contractOrderList = []string{"escrow_time"}
)

type ByTime []*nodepb.Contracts_Contract

func (a ByTime) Len() int           { return len(a) }
func (a ByTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTime) Less(i, j int) bool { return a[i].StartTime.UnixNano() < a[j].StartTime.UnixNano() }

// sub-commands: btfs storage contracts list
var storageContractsListCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get contracts list based on role.",
		ShortDescription: `
This command get contracts list based on role from the local node data store.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("role", true, false, "Role in BTFS storage network [host|renter|reserved]."),
	},
	Options: []cmds.Option{
		cmds.StringOption(contractsListOrderOptionName, "o", "Order to return the list of contracts.").WithDefault("escrow_time,asc"),
		cmds.StringOption(contractsListStatusOptionName, "st", "Filter the returned list by contract status [active|finished|invalid|all].").WithDefault("active"),
		cmds.IntOption(contractsListSizeOptionName, "s", "Number of contracts to return.").WithDefault(20),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cr, err := checkContractStatRole(req.Arguments[0])
		if err != nil {
			return err
		}
		orderOpt, _ := req.Options[contractsListOrderOptionName].(string)
		parts := strings.Split(orderOpt, ",")
		if len(parts) != 2 {
			return fmt.Errorf(`bad order format "<order-name>,<order-direction>"`)
		}
		var order string
		for _, o := range contractOrderList {
			if o == parts[0] {
				order = o
				break
			}
		}
		if order == "" {
			return fmt.Errorf("bad order name: %s", parts[0])
		}
		if parts[1] != "asc" && parts[1] != "desc" {
			return fmt.Errorf("bad order direction: %s", parts[1])
		}
		filterOpt, _ := req.Options[contractsListStatusOptionName].(string)
		states, ok := helper.ContractFilterMap[filterOpt]
		if !ok {
			return fmt.Errorf("invalid filter option: %s", filterOpt)
		}
		size, _ := req.Options[contractsListSizeOptionName].(int)
		contracts, err := ListContracts(n.Repo.Datastore(), cr.String())
		if err != nil {
			return err
		}
		sort.Sort(ByTime(contracts))
		if parts[1] == "" || parts[1] == "desc" {
			// reverse
			for i, j := 0, len(contracts)-1; i < j; i, j = i+1, j-1 {
				contracts[i], contracts[j] = contracts[j], contracts[i]
			}
		}
		result := make([]*nodepb.Contracts_Contract, 0)
		for _, c := range contracts {
			if _, ok := states[c.Status]; !ok {
				continue
			}
			result = append(result, c)
			if len(result) == size {
				break
			}
		}
		return cmds.EmitOnce(res, &nodepb.Contracts{Contracts: result})
	},
	Type: nodepb.Contracts{},
}

func getKey(role string) string {
	var k string
	if role == nodepb.ContractStat_HOST.String() {
		k = hostContractsKey
	} else if role == nodepb.ContractStat_RENTER.String() {
		k = renterContractsKey
	} else {
		return "reserved"
	}
	return k
}

func Save(d datastore.Datastore, cs []*nodepb.Contracts_Contract, role string) error {
	return sessions.Save(d, getKey(role), &contractspb.Contracts{
		Contracts: cs,
	})
}

func ListContracts(d datastore.Datastore, role string) ([]*nodepb.Contracts_Contract, error) {
	cs := &contractspb.Contracts{}
	err := sessions.Get(d, getKey(role), cs)
	if err != nil && err != datastore.ErrNotFound {
		return nil, err
	}
	return cs.Contracts, nil
}

func SyncContracts(ctx context.Context, n *core.IpfsNode, req *cmds.Request, env cmds.Environment,
	role string) error {
	cs, err := sessions.ListShardsContracts(n.Repo.Datastore(), n.Identity.Pretty(), role)
	if err != nil {
		return err
	}
	var latest *time.Time
	for _, c := range cs {
		if latest == nil || c.SignedGuardContract.LastModifyTime.After(*latest) {
			latest = &c.SignedGuardContract.LastModifyTime
		}
	}
	updated, err := GetUpdatedGuardContracts(ctx, n, latest)
	if err != nil {
		return err
	}
	if len(updated) > 0 {
		// save and retrieve updated signed contracts
		var stale []string
		cs, stale, err = sessions.SaveShardsContracts(n.Repo.Datastore(), cs, updated, n.Identity.Pretty(), role)
		if err != nil {
			return err
		}
		_, err := rm.RmDag(ctx, stale, n, req, env, true)
		if err != nil {
			return err
		}
	}
	if len(cs) > 0 {
		results, err := syncContractPayoutStatus(ctx, n, cs)
		if err != nil {
			return err
		}
		return Save(n.Repo.Datastore(), results, role)
	}
	return nil
}

// GetUpdatedGuardContracts retrieves updated guard contracts from remote based on latest timestamp
// and returns the list updated
func GetUpdatedGuardContracts(ctx context.Context, n *core.IpfsNode,
	lastUpdatedTime *time.Time) ([]*guardpb.Contract, error) {
	// Loop until all pages are obtained
	var contracts []*guardpb.Contract
	for i := 0; ; i++ {
		now := time.Now()
		req := &guardpb.ListHostContractsRequest{
			HostPid:             n.Identity.Pretty(),
			RequesterPid:        n.Identity.Pretty(),
			RequestPageSize:     guardContractPageSize,
			RequestPageIndex:    int32(i),
			LastModifyTimeSince: lastUpdatedTime,
			State:               guardpb.ListHostContractsRequest_ALL,
			RequestTime:         &now,
		}
		signedReq, err := crypto.Sign(n.PrivateKey, req)
		if err != nil {
			return nil, err
		}
		req.Signature = signedReq

		cfg, err := n.Repo.Config()
		if err != nil {
			return nil, err
		}
		cs, last, err := ListHostContracts(ctx, cfg, req)
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, cs...)
		if last {
			break
		}
	}
	return contracts, nil
}

// ListHostContracts opens a grpc connection, sends the host contract list request and
// closes (short) connection
func ListHostContracts(ctx context.Context, cfg *config.Config,
	listReq *guardpb.ListHostContractsRequest) ([]*guardpb.Contract, bool, error) {
	cb := grpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	var contracts []*guardpb.Contract
	var lastPage bool
	err := cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
		res, err := client.ListHostContracts(ctx, listReq)
		if err != nil {
			return err
		}
		contracts = res.Contracts
		lastPage = res.Count < listReq.RequestPageSize
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return contracts, lastPage, nil
}

// SyncContractPayoutStatus looks at local contracts, refreshes from Guard, and then
// syncs payout status for all of them
func syncContractPayoutStatus(ctx context.Context, n *core.IpfsNode,
	cs []*shardpb.SignedContracts) ([]*nodepb.Contracts_Contract, error) {
	cfg, err := n.Repo.Config()
	if err != nil {
		return nil, err
	}
	pk, err := n.Identity.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	pkBytes, err := ic.RawFull(pk)
	if err != nil {
		return nil, err
	}
	results := make([]*nodepb.Contracts_Contract, 0)
	err = grpc.EscrowClient(cfg.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context,
			client escrowpb.EscrowServiceClient) error {
			in := &escrowpb.SignedContractIDBatch{
				Data: &escrowpb.ContractIDBatch{
					Address: pkBytes,
				},
			}
			for _, c := range cs {
				in.Data.ContractId = append(in.Data.ContractId, c.SignedGuardContract.ContractId)
			}
			sign, err := crypto.Sign(n.PrivateKey, in.Data)
			if err != nil {
				contractsLog.Error("sign contractID error:", err)
				return err
			}
			in.Signature = sign
			sb, err := client.GetPayOutStatusBatch(ctx, in)
			if err != nil {
				contractsLog.Error("get payout status batch error:", err)
				return err
			}
			if len(sb.Status) != len(cs) {
				return fmt.Errorf("payout status batch returned wrong length of contracts, need %d, got %d",
					len(cs), len(sb.Status))
			}
			for i, c := range cs {
				s := sb.Status[i]
				// Set dummy payout status on invalid id or error msg
				// Reset to dummy status so we have a copy locally with invalid params
				if s.ContractId != c.SignedGuardContract.ContractId {
					s = &escrowpb.PayoutStatus{
						ContractId: c.SignedGuardContract.ContractId,
						ErrorMsg:   "mistmatched contract id",
					}
				} else if s.ErrorMsg != "" {
					s = &escrowpb.PayoutStatus{
						ContractId: c.SignedGuardContract.ContractId,
						ErrorMsg:   s.ErrorMsg,
					}
				}
				if s.ErrorMsg != "" {
					contractsLog.Debug("got payout status error message:", s.ErrorMsg)
				}

				results = append(results, &nodepb.Contracts_Contract{
					ContractId:              c.SignedGuardContract.ContractId,
					HostId:                  c.SignedGuardContract.HostPid,
					RenterId:                c.SignedGuardContract.RenterPid,
					Status:                  c.SignedGuardContract.State,
					StartTime:               c.SignedGuardContract.RentStart,
					EndTime:                 c.SignedGuardContract.RentEnd,
					NextEscrowTime:          s.NextPayoutTime,
					CompensationPaid:        s.PaidAmount,
					CompensationOutstanding: s.Amount - s.PaidAmount,
					UnitPrice:               c.SignedGuardContract.Price,
					ShardSize:               c.SignedGuardContract.ShardFileSize,
					ShardHash:               c.SignedGuardContract.ShardHash,
					FileHash:                c.SignedGuardContract.FileHash,
				})
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return results, nil
}
