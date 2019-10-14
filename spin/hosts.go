package spin

import (
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
		fmt.Printf("Failed to get configuration %s\n", err)
		return
	}

	if configuration.Experimental.HostsSyncEnabled {
		m := configuration.Experimental.HostsSyncMode
		fmt.Printf("Hosts info will be synced at [%s] mode", m)

		go perodicSyncHosts(node, m)
	}
}

func perodicSyncHosts(node *core.IpfsNode, mode string) {
	tick := time.NewTicker(syncPeriod)
	defer tick.Stop()
	for range tick.C {
		err := commands.SyncHosts(node, mode)
		if err != nil {
			fmt.Println("Failed to sync hosts: ", err)
		}
	}
}
