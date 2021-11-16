package upload

import (
	"fmt"
	"github.com/TRON-US/go-btfs/chain"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"math/big"
)

func payInCheque(rss *sessions.RenterSession) error {
	for i, hash := range rss.ShardHashes {
		shard, err := sessions.GetRenterShard(rss.CtxParams, rss.SsId, hash, i)
		if err != nil {
			return err
		}
		c, err := shard.Contracts()
		if err != nil {
			return err
		}

		amount := c.SignedGuardContract.Amount
		host := c.SignedGuardContract.HostPid
		err = chain.SettleObject.SwapService.Settle(host, big.NewInt(amount))
		if err != nil {
			return err
		}

		fmt.Println("pay Settle, host, amount: ", host, amount)
	}

	return nil
}
