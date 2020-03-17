package spin

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/store/hosts"
	"github.com/TRON-US/go-btfs/core/commands/store/stats"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
)

const (
	hostSyncPeriod         = 60 * time.Minute
	hostStatsSyncPeriod    = 30 * time.Minute
	hostSettingsSyncPeriod = 60 * time.Minute
	hostSyncTimeout        = 30 * time.Second
)

func Hosts(node *core.IpfsNode) {
	cfg, err := node.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}

	if cfg.Experimental.HostsSyncEnabled {
		m := cfg.Experimental.HostsSyncMode
		fmt.Printf("Storage host info will be synced at [%s] mode\n", m)
		go periodicHostSync(hostSyncPeriod, hostSyncTimeout, "hosts",
			func(ctx context.Context) error {
				return hosts.SyncHosts(ctx, node, m)
			})
	}
	if cfg.Experimental.StorageHostEnabled {
		fmt.Println("Current host stats will be synced")
		go periodicHostSync(hostStatsSyncPeriod, hostSyncTimeout, "host stats",
			func(ctx context.Context) error {
				return stats.SyncStats(ctx, cfg, node)
			})
		fmt.Println("Current host settings will be synced")
		go periodicHostSync(hostSettingsSyncPeriod, hostSyncTimeout, "host settings",
			func(ctx context.Context) error {
				_, err = storage.GetHostStorageConfig(ctx, node)
				return err
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
