package spin

import (
	"context"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/wallet"

	cmds "github.com/TRON-US/go-btfs-cmds"
)

const (
	bttTxSyncPeriod  = 3 * time.Minute
	bttTxSyncTimeout = 1 * time.Minute
)

func BttTransactions(node *core.IpfsNode, env cmds.Environment) {
	cfg, err := node.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}

	go periodicBttTxsSync(bttTxSyncPeriod, bttTxSyncTimeout, "btt-tx",
		func(ctx context.Context) error {
			_, err := wallet.SyncTxFromTronGrid(ctx, cfg, node.Repo.Datastore())
			return err
		})
}

func periodicBttTxsSync(period, timeout time.Duration, msg string, syncFunc func(context.Context) error) {
	tick := time.NewTicker(period)
	defer tick.Stop()
	// Force tick on immediate start
	for ; true; <-tick.C {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		err := syncFunc(ctx)
		if err != nil {
			log.Debugf("Failed to sync %s: %s", msg, err)
		}
	}
}
