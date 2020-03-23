package contracts

import (
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/store/upload/ds"
	"strings"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
)

const (
	contractsListOrderOptionName  = "order"
	contractsListStatusOptionName = "status"
	contractsListSizeOptionName   = "size"
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
	RunTimeout: 30 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		_, err := checkContractStatRole(req.Arguments[0])
		if err != nil {
			return err
		}
		// TODO: sync
		return nil
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
		_, err := checkContractStatRole(req.Arguments[0])
		if err != nil {
			return err
		}
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		err = ds.ListShards(n.Repo.Datastore(), n.Identity.Pretty())
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
		// TODO: use filter list
		_, ok := contractFilterMap[filterOpt]
		if !ok {
			return fmt.Errorf("invalid filter option: %s", filterOpt)
		}
		// TODO: Use size
		//sizeOpt, _ := req.Options[contractsListSizeOptionName].(int)
		// TODO: return mock -- static dummy
		data := []*nodepb.Contracts_Contract{
			{
				ContractId:              "737b6d38-5af1-4023-b219-196aadf4d3f0",
				HostId:                  "16Uiu2HAmPTPCDrinViMyEzRGGcEJpc2VUH9bc46dU7sP2TzsbNnc",
				RenterId:                "16Uiu2HAmCknnNaWa44X4kLRCq33B3zvBevRTLdo27qMhsszCwqdF",
				Status:                  guardpb.Contract_DRAFT,
				StartTime:               time.Now(),
				EndTime:                 time.Now(),
				NextEscrowTime:          time.Now(),
				CompensationPaid:        0,
				CompensationOutstanding: 15000,
				UnitPrice:               10,
				ShardSize:               500,
				ShardHash:               "QmUX3GkfVQ8ARa79VE5HC6dxA5AtQQGaUTg1nbaqcAaYmp",
				FileHash:                "QmAA3GkfVQ8ARa79VE5HC6dxA5AtQQGaUTg1nbaqcAaYm1",
			},
			{
				ContractId:              "869675ac-c966-4808-83d7-1901d0449fb6",
				HostId:                  "16Uiu2HAmPTPCDrinViMyEzRGGcEJpc2VUH9bc46dU7sP2TzsbNnc",
				RenterId:                "16Uiu2HAmR6h5aamvwYDKYdp2Z3imCfHLRJnjB7VAYeab23AaZxSY",
				Status:                  guardpb.Contract_CLOSED,
				StartTime:               time.Now(),
				EndTime:                 time.Now(),
				NextEscrowTime:          time.Now(),
				CompensationPaid:        100,
				CompensationOutstanding: 300,
				UnitPrice:               10,
				ShardSize:               40,
				ShardHash:               "QmTT3GkfVQ8ARa79VE5HC6dxA5AtQQGaUTg1nbaqcAaYm2",
				FileHash:                "QmXx3GkfVQ8ARa79VE5HC6dxA5AtQQGaUTg1nbaqcAaYm3",
			},
		}
		return cmds.EmitOnce(res, &nodepb.Contracts{Contracts: data})
	},
	Type: nodepb.Contracts{},
}
