package upload

import (
	"context"
	"errors"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	"github.com/cenkalti/backoff/v4"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/sync/errgroup"
)

func UploadShard(rss *sessions.RenterSession, hp helper.IHostsProvider, price int64, shardSize int64,
	storageLength int,
	offlineSigning bool, renterId peer.ID, fileSize int64, shardIndexes []int, rp *RepairParams) {
	for index, shardHash := range rss.ShardHashes {
		go func(i int, h string) {
			err := backoff.Retry(func() error {
				select {
				case <-rss.Ctx.Done():
					return nil
				default:
					break
				}
				host, err := hp.NextValidHost(price)
				if err != nil {
					terr := rss.To(sessions.RssToErrorEvent, err)
					if terr != nil {
						// Ignore err, just print error log
						log.Debugf("original err: %s, transition err: %s", err.Error(), terr.Error())
					}
					return nil
				}
				contractId := helper.NewContractID(rss.SsId)
				cb := make(chan error)
				ShardErrChanMap.Set(contractId, cb)
				tp := helper.TotalPay(shardSize, price, storageLength)
				var escrowCotractBytes []byte

				eg, _ := errgroup.WithContext(rss.Ctx)
				eg.Go(func() error {
					escrowCotractBytes, err = renterSignEscrowContract(rss, h, i, host, tp, offlineSigning, renterId, contractId, storageLength)
					if err != nil {
						log.Errorf("shard %s signs escrow_contract error: %s", h, err.Error())
						return err
					}
					return nil
				})
				var guardContractBytes []byte
				eg.Go(func() error {
					guardContractBytes, err = RenterSignGuardContract(rss, &ContractParams{
						ContractId:    contractId,
						RenterPid:     renterId.Pretty(),
						HostPid:       host,
						ShardIndex:    int32(i),
						ShardHash:     h,
						ShardSize:     shardSize,
						FileHash:      rss.Hash,
						StartTime:     time.Now(),
						StorageLength: int64(storageLength),
						Price:         price,
						TotalPay:      tp,
					}, offlineSigning, rp)
					if err != nil {
						log.Errorf("shard %s signs guard_contract error: %s", h, err.Error())
						return err
					}
					return nil
				})
				if err := eg.Wait(); err != nil {
					return err
				}

				hostPid, err := peer.IDB58Decode(host)
				if err != nil {
					log.Errorf("shard %s decodes host_pid error: %s", h, err.Error())
					return err
				}
				go func() {
					ctx, _ := context.WithTimeout(rss.Ctx, 10*time.Second)
					_, err := remote.P2PCall(ctx, rss.CtxParams.N, rss.CtxParams.Api, hostPid, "/storage/upload/init",
						rss.SsId,
						rss.Hash,
						h,
						price,
						escrowCotractBytes,
						guardContractBytes,
						storageLength,
						shardSize,
						i,
						renterId,
					)
					if err != nil {
						cb <- err
					}
				}()
				// host needs to send recv in 30 seconds, or the contract will be invalid.
				tick := time.Tick(30 * time.Second)
				select {
				case err = <-cb:
					ShardErrChanMap.Remove(contractId)
					return err
				case <-tick:
					return errors.New("host timeout")
				}
			}, helper.HandleShardBo)
			if err != nil {
				_ = rss.To(sessions.RssToErrorEvent,
					errors.New("timeout: failed to setup contract in "+helper.HandleShardBo.MaxElapsedTime.String()))
			}
		}(shardIndexes[index], shardHash)
	}
	// waiting for contracts of 30(n) shards
	go func(rss *sessions.RenterSession, numShards int) {
		tick := time.Tick(5 * time.Second)
		for true {
			select {
			case <-tick:
				completeNum, errorNum, err := rss.GetCompleteShardsNum()
				if err != nil {
					continue
				}
				log.Info("session", rss.SsId, "contractNum", completeNum, "errorNum", errorNum)
				if completeNum == numShards {
					err := Submit(rss, fileSize, offlineSigning)
					if err != nil {
						_ = rss.To(sessions.RssToErrorEvent, err)
					}
					return
				} else if errorNum > 0 {
					_ = rss.To(sessions.RssToErrorEvent, errors.New("there are some error shards"))
					log.Error("session:", rss.SsId, ",errorNum:", errorNum)
					return
				}
			case <-rss.Ctx.Done():
				log.Infof("session %s done", rss.SsId)
				return
			}
		}
	}(rss, len(rss.ShardHashes))
}
