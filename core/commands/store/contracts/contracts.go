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
	contractspb "github.com/TRON-US/go-btfs/protos/contracts"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/ipfs/go-datastore"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/prometheus/common/log"
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
		// TODO: return mock -- static dummy
		data := &nodepb.ContractStat{
			ActiveContractNum:       10,
			CompensationPaid:        20000,
			CompensationOutstanding: 80000,
			FirstContractStart:      time.Now(),
			LastContractEnd:         time.Now(),
			Role:                    cr,
		}
		return cmds.EmitOnce(res, data)
	},
	Type: nodepb.ContractStat{},
}

var (
	contractOrderList = []string{"escrow_time"}
	contractFilterMap = map[string][]guardpb.Contract_ContractState{
		"active": {
			guardpb.Contract_DRAFT,
			guardpb.Contract_SIGNED,
			guardpb.Contract_UPLOADED,
			guardpb.Contract_RENEWED,
			guardpb.Contract_WARN,
		},
		"finished": {
			guardpb.Contract_CLOSED,
		},
		"invalid": {
			guardpb.Contract_LOST,
			guardpb.Contract_CANCELED,
			guardpb.Contract_OBSOLETE,
		},
		"all": {
			guardpb.Contract_DRAFT,
			guardpb.Contract_SIGNED,
			guardpb.Contract_UPLOADED,
			guardpb.Contract_LOST,
			guardpb.Contract_CANCELED,
			guardpb.Contract_CLOSED,
			guardpb.Contract_RENEWED,
			guardpb.Contract_OBSOLETE,
			guardpb.Contract_WARN,
		},
	}
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
		states, ok := contractFilterMap[filterOpt]
		if !ok {
			return fmt.Errorf("invalid filter option: %s", filterOpt)
		}
		size, ok := req.Options[contractsListSizeOptionName].(int)
		if !ok {
			return fmt.Errorf("invalid size option: %s", size)
		}
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
			b := false
			for _, state := range states {
				if state == c.Status {
					b = true
					continue
				}
			}
			if !b {
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
	cfg, err := n.Repo.Config()
	if err != nil {
		return err
	}
	pk, err := n.Identity.ExtractPublicKey()
	if err != nil {
		return err
	}
	pkBytes, err := ic.RawFull(pk)
	if err != nil {
		return err
	}
	results := make([]*nodepb.Contracts_Contract, 0)
	cs, err := ds.ListShardsContracts(n.Repo.Datastore(), n.Identity.Pretty(), role)
	if err != nil {
		return err
	}
	err = grpc.EscrowClient(cfg.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context,
			client escrowpb.EscrowServiceClient) error {
			for _, c := range cs {
				ec, err := escrow.UnmarshalEscrowContract(c.SignedEscrowContract)
				if err != nil {
					log.Error("unmarshal escrow contract error:", err)
					continue
				}
				in := &escrowpb.SignedContractID{
					Data: &escrowpb.ContractID{
						ContractId: ec.Contract.ContractId,
						Address:    pkBytes,
					},
				}
				sign, err := crypto.Sign(n.PrivateKey, in.Data)
				if err != nil {
					log.Error("sign contractID error:", err)
					continue
				}
				in.Signature = sign
				s, err := client.GetPayOutStatus(ctx, in)
				if err != nil {
					log.Error("get payout status error:", err)
					//continue
					s = &escrowpb.SignedPayoutStatus{
						Status: &escrowpb.PayoutStatus{},
					}
				}
				results = append(results, &nodepb.Contracts_Contract{
					ContractId:              c.GuardContract.ContractId,
					HostId:                  c.GuardContract.HostPid,
					RenterId:                c.GuardContract.RenterPid,
					Status:                  c.GuardContract.State,
					StartTime:               c.GuardContract.RentStart,
					EndTime:                 c.GuardContract.RentEnd,
					NextEscrowTime:          s.Status.NextPayoutTime,
					CompensationPaid:        s.Status.PaidAmount,
					CompensationOutstanding: s.Status.Amount - s.Status.PaidAmount,
					UnitPrice:               c.GuardContract.Price,
					ShardSize:               c.GuardContract.ShardFileSize,
					ShardHash:               c.GuardContract.ShardHash,
					FileHash:                c.GuardContract.FileHash,
				})
			}
			return nil
		})
	if err != nil {
		return err
	}
	return Save(n.Repo.Datastore(), results, role)
}
