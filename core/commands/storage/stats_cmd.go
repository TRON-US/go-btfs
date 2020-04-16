package storage

import (
	"context"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/corerepo"
	"github.com/TRON-US/go-btfs/core/hub"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"

	"github.com/shirou/gopsutil/disk"
)

const (
	localInfoOnlyOptionName = "local-only"
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
		Online:               cfg.Experimental.StorageHostEnabled,
		Uptime:               sr.Uptime,
		Score:                sr.Score,
		StorageUsed:          int64(stat.RepoSize),
		StorageCap:           int64(stat.StorageMax),
		StorageDiskTotal:     int64(du.Total),
		StorageDiskAvailable: int64(du.Free),
	}
	return SaveHostStatsIntoDatastore(ctx, node, node.Identity.Pretty(), hs)
}

// sub-commands: btfs storage stats info
var storageStatsInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get node stats.",
		ShortDescription: `
This command get node stats in the network from the local node data store.`,
	},
	Arguments: []cmds.Argument{},
	Options: []cmds.Option{
		cmds.BoolOption(localInfoOnlyOptionName, "l", "Return only the locally available disk stats without querying/returning the network stats.").WithDefault(false),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		local, _ := req.Options[localInfoOnlyOptionName].(bool)
		var hs *nodepb.StorageStat_Host
		if !local {
			hs, err = GetHostStatsFromDatastore(req.Context, n, n.Identity.Pretty())
			if err != nil {
				return err
			}
		} else {
			hs = &nodepb.StorageStat_Host{}
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
		hs.StorageDiskAvailable = int64(du.Free)

		// Only host stats for now
		return cmds.EmitOnce(res, &nodepb.StorageStat{
			HostStats: *hs,
		})
	},
	Type: nodepb.StorageStat{},
}
