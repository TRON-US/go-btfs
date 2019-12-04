package spin

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands"
)

const (
	syncPeriod = 60 * time.Minute
)

func Hosts(node *core.IpfsNode) {
	configuration, err := node.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}

	if configuration.Experimental.HostsSyncEnabled {
		m := configuration.Experimental.HostsSyncMode
		fmt.Printf("Hosts info will be synced at [%s] mode\n", m)

		go perodicSyncHosts(node, m)
	}
}

func perodicSyncHosts(node *core.IpfsNode, mode string) {
	tick := time.NewTicker(syncPeriod)
	defer tick.Stop()
	// Force tick on immediate start
	for ; true; <-tick.C {
		err := commands.SyncHosts(context.Background(), node, mode)
		if err != nil {
			log.Errorf("Failed to sync hosts: %s", err)
		}
	}
}
