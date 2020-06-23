package spin

import (
	"os"
	"os/signal"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/wallet"
)

const (
	period = 5 * time.Minute
)

type walletWrap struct {
	*helper.ContextParams
}

func NewWalletWrap(params *helper.ContextParams) *walletWrap {
	return &walletWrap{params}
}

func (wt *walletWrap) UpdateStatus() {
	ex := make(chan os.Signal, 1)
	signal.Notify(ex, os.Interrupt)
	go func() {
		tick := time.NewTicker(period)
		defer tick.Stop()
		for {
			sc, ec, err := wallet.UpdatePendingTransactions(wt.Ctx, wt.N.Repo.Datastore(), wt.Cfg, wt.N.Identity.String())
			log.Debugf("update pending tx, success: %v, error: %v, err: %v", sc, ec, err)
			select {
			case <-tick.C:
				continue
			case <-ex:
				break
			}
		}
	}()
}
