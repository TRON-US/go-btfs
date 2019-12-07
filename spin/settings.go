package spin

import (
	"context"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands"
)

const (
	syncSettingsPeriod = 60 * time.Minute
	// TODO: change this after Alvin done the pricing strategy
	defaultStorageAsk = 50.0
)

func Settings(node *core.IpfsNode) {
	configuration, err := node.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}

	if configuration.Experimental.StorageHostEnabled {
		s := configuration.Experimental.StoragePrice
		if s == 0 {
			go perodicSyncSettings(node)
		}
	}
}

func perodicSyncSettings(node *core.IpfsNode) {
	tick := time.NewTicker(syncSettingsPeriod)
	defer tick.Stop()
	// Force tick on immediate start
	for ; true; <-tick.C {
		config, err := node.Repo.Config()
		if err != nil {
			log.Errorf("Failed to get configuration: %s", err)
			continue
		}
		resp, err := commands.GetSettings(context.Background(), node)
		if err != nil {
			log.Errorf("Failed to get hostsSettings %s", err)
			config.Experimental.StoragePrice = defaultStorageAsk
			continue
		}
		config.Experimental.StoragePrice = resp.GetSettingsData().StoragePriceAsk
	}
}
