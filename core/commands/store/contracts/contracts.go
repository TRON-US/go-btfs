package contracts

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/store/upload/ds"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/guard"

	contractspb "github.com/TRON-US/go-btfs/protos/contracts"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/ipfs/go-datastore"
)

const (
	contractsListOrderOptionName  = "order"
	contractsListStatusOptionName = "status"
	contractsListSizeOptionName   = "size"

	contractsKeyPrefix = "/btfs/%s/contracts/"
	hostContractsKey   = contractsKeyPrefix + "host"
	renterContractsKey = contractsKeyPrefix + "renter"
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
This command contracts stats based on role from network(hub) to local node data store.`,
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
		return SyncContracts(req.Context, n, role.String())
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
		activeStates := guard.ContractFilterMap["active"]
		invalidStates := guard.ContractFilterMap["invalid"]
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
		states, ok := guard.ContractFilterMap[filterOpt]
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
	return ds.Save(d, getKey(role), &contractspb.Contracts{
		Contracts: cs,
	})
}

func ListContracts(d datastore.Datastore, role string) ([]*nodepb.Contracts_Contract, error) {
	cs := &contractspb.Contracts{}
	err := ds.Get(d, getKey(role), cs)
	if err != nil && err != datastore.ErrNotFound {
		return nil, err
	}
	return cs.Contracts, nil
}

func SyncContracts(ctx context.Context, n *core.IpfsNode, role string) error {
	cs, err := ds.ListShardsContracts(n.Repo.Datastore(), n.Identity.Pretty(), role)
	if err != nil {
		return err
	}
	var latest *time.Time
	for _, c := range cs {
		if latest == nil || c.GuardContract.LastModifyTime.After(*latest) {
			latest = &c.GuardContract.LastModifyTime
		}
	}
	updated, err := guard.GetUpdatedGuardContracts(ctx, n, latest)
	if err != nil {
		return err
	}
	if len(updated) > 0 {
		// save and retrieve updated signed contracts
		cs, err = ds.SaveShardsContracts(n.Repo.Datastore(), cs, updated, n.Identity.Pretty(), role)
		if err != nil {
			return err
		}
	}
	results, err := escrow.SyncContractPayoutStatus(ctx, n, cs)
	if err != nil {
		return err
	}
	return Save(n.Repo.Datastore(), results, role)
}
