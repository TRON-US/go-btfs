package spin

import (
	"context"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core"
)

const (
	hostContractsSyncPeriod  = 60 * time.Minute
	hostContractsSyncTimeout = 10 * time.Minute
)

func Contracts(n *core.IpfsNode, req *cmds.Request, env cmds.Environment, role string) {
	cfg, err := n.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}
	if cfg.Experimental.StorageHostEnabled {
		go periodicHostSync(hostContractsSyncPeriod, hostContractsSyncTimeout, role+" contracts",
			func(ctx context.Context) error {
				return storage.SyncContracts(ctx, n, req, env, role)
			})
	}
}
