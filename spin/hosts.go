package spin

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage/challenge"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/hosts"
	"github.com/TRON-US/go-btfs/core/commands/storage/stats"

	cmds "github.com/TRON-US/go-btfs-cmds"
)

const (
	hostSyncPeriod         = 60 * time.Minute
	hostStatsSyncPeriod    = 30 * time.Minute
	hostSettingsSyncPeriod = 60 * time.Minute
	hostSyncTimeout        = 30 * time.Second
	hostSortTimeout        = 5 * time.Minute
)

func Hosts(node *core.IpfsNode, req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) {
	cfg, err := node.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}

	if cfg.Experimental.HostsSyncEnabled {
		m := cfg.Experimental.HostsSyncMode
		fmt.Printf("Storage host info will be synced at [%s] mode\n", m)
		go periodicHostSync(hostSyncPeriod, hostSyncTimeout+hostSortTimeout, "hosts",
			func(ctx context.Context) error {
				_, err := hosts.SyncHosts(ctx, node, m)
				return err
			})
	}
	if cfg.Experimental.StorageHostEnabled {
		fmt.Println("Current host stats will be synced")
		go periodicHostSync(hostStatsSyncPeriod, hostSyncTimeout, "host stats",
			func(ctx context.Context) error {
				return stats.SyncStats(ctx, cfg, node, env)
			})
		fmt.Println("Current host settings will be synced")
		go periodicHostSync(hostSettingsSyncPeriod, hostSyncTimeout, "host settings",
			func(ctx context.Context) error {
				_, err = helper.GetHostStorageConfigHelper(ctx, node, true)
				return err
			})
	}
	if cfg.Experimental.HostChallengeEnabled {
		fmt.Println("Current host is requesting for challenge")
		go periodicChallengeReq(hostSettingsSyncPeriod, "challenge requests",
			func(ctx context.Context) error {
				return challenge.RequestChallenge(req, res, env, cfg)
			})
	}
}

func periodicHostSync(period, timeout time.Duration, msg string, syncFunc func(context.Context) error) {
	tick := time.NewTicker(period)
	defer tick.Stop()
	// Force tick on immediate start
	for ; true; <-tick.C {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		err := syncFunc(ctx)
		if err != nil {
			log.Errorf("Failed to sync %s: %s", msg, err)
		}
	}
}

func periodicChallengeReq(period time.Duration, msg string, syncFunc func(context.Context) error) {
	tick := time.NewTicker(period)
	defer tick.Stop()
	for ; true; <-tick.C {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		err := syncFunc(ctx)
		if err != nil {
			log.Errorf("Failed to challenge %s: %s", msg, err)
		}
	}
}