package spin

import (
	"context"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/store/contracts"
)

const (
	hostContractsSyncPeriod  = 60 * time.Minute
	hostContractsSyncTimeout = 10 * time.Minute
)

func Contracts(n *core.IpfsNode, role string) {
	cfg, err := n.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}
	if cfg.Experimental.StorageHostEnabled {
		go periodicHostSync(hostContractsSyncPeriod, hostContractsSyncTimeout, role+" contracts",
			func(ctx context.Context) error {
				return contracts.SyncContracts(ctx, n, role)
			})
	}
}
