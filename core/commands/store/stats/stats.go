package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/corerepo"
	"github.com/TRON-US/go-btfs/core/hub"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"

	"github.com/shirou/gopsutil/disk"
)

// Storage Stats
//
// Includes sub-commands: info, sync
var StorageStatsCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get node storage stats.",
		ShortDescription: `
This command get node storage stats in the network.`,
	},
	Subcommands: map[string]*cmds.Command{
		"sync": storageStatsSyncCmd,
		"info": storageStatsInfoCmd,
	},
}

// sub-commands: btfs storage stats sync
var storageStatsSyncCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Synchronize node stats.",
		ShortDescription: `
This command synchronize node stats from network(hub) to local node data store.`,
	},
	Arguments: []cmds.Argument{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		return SyncStats(req.Context, cfg, n, env)
	},
}

func SyncStats(ctx context.Context, cfg *config.Config, node *core.IpfsNode, env cmds.Environment) error {
	sr, err := hub.QueryStats(ctx, node)
	if err != nil {
		return err
	}
	stat, err := corerepo.RepoStat(ctx, node)
	if err != nil {
		return err
	}
	cfgRoot, err := cmdenv.GetConfigRoot(env)
	if err != nil {
		return err
	}
	du, err := disk.Usage(cfgRoot)
	if err != nil {
		return err
	}
	hs := &nodepb.StorageStat_Host{
		Online:           cfg.Experimental.StorageHostEnabled,
		Uptime:           sr.Uptime,
		Score:            sr.Score,
		StorageUsed:      int64(stat.RepoSize),
		StorageCap:       int64(stat.StorageMax),
		StorageDiskTotal: int64(du.Total),
	}
	return storage.SaveHostStatsIntoDatastore(ctx, node, node.Identity.Pretty(), hs)
}

// sub-commands: btfs storage stats info
var storageStatsInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get node stats.",
		ShortDescription: `
This command get node stats in the network from the local node data store.`,
	},
	Arguments:  []cmds.Argument{},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		hs, err := storage.GetHostStatsFromDatastore(req.Context, n, n.Identity.Pretty())
		if err != nil {
			return err
		}

		// Refresh latest repo stats
		stat, err := corerepo.RepoStat(req.Context, n)
		if err != nil {
			return err
		}

		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}
		du, err := disk.Usage(cfgRoot)
		if err != nil {
			return err
		}

		hs.Online = cfg.Experimental.StorageHostEnabled
		hs.StorageUsed = int64(stat.RepoSize)
		hs.StorageCap = int64(stat.StorageMax)
		hs.StorageDiskTotal = int64(du.Total)

		// Only host stats for now
		return cmds.EmitOnce(res, &nodepb.StorageStat{
			HostStats: *hs,
		})
	},
	Type: nodepb.StorageStat{},
}
