package upload

import (
	"math/big"

	"github.com/TRON-US/go-btfs/chain"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
)

func Submit(rss *sessions.RenterSession, fileSize int64, offlineSigning bool) error {
	if err := rss.To(sessions.RssToSubmitEvent); err != nil {
		return err
	}

	err := doSubmit(rss)
	if err != nil {
		return err
	}
	return doGuardAndPay(rss, nil, fileSize, offlineSigning)
}

func prepareAmount(rss *sessions.RenterSession, shardHashes []string) (int64, error) {
	var totalPrice int64
	for i, hash := range shardHashes {
		shard, err := sessions.GetRenterShard(rss.CtxParams, rss.SsId, hash, i)
		if err != nil {
			return 0, err
		}
		c, err := shard.Contracts()
		if err != nil {
			return 0, err
		}
		totalPrice += c.SignedGuardContract.Amount
	}
	return totalPrice, nil
}

func doSubmit(rss *sessions.RenterSession) error {
	amount, err := prepareAmount(rss, rss.ShardHashes)
	if err != nil {
		return err
	}

	err = chain.SettleObject.ChequebookService.CheckBalance(big.NewInt(amount))
	if err != nil {
		return err
	}
	return nil
}
