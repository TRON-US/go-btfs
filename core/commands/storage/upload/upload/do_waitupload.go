package upload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v4"
	"github.com/gogo/protobuf/proto"
)

const (
	thresholdContractsNums = 20
)

func getSuccessThreshold(totalShards int) int {
	return int(math.Min(float64(totalShards), thresholdContractsNums))
}

func ResumeWaitUploadOnSigning(rss *sessions.RenterSession) error {
	return waitUpload(rss, false, &guardpb.FileStoreStatus{
		FileStoreMeta: guardpb.FileStoreMeta{
			RenterPid: rss.CtxParams.N.Identity.String(),
			FileSize:  math.MaxInt64,
		},
	}, true)
}

func waitUpload(rss *sessions.RenterSession, offlineSigning bool, fsStatus *guardpb.FileStoreStatus, resume bool) error {
	threshold := getSuccessThreshold(len(rss.ShardHashes))
	if !resume {
		if err := rss.To(sessions.RssToWaitUploadEvent); err != nil {
			return err
		}
	}
	req := &guardpb.CheckFileStoreMetaRequest{
		FileHash:     rss.Hash,
		RenterPid:    fsStatus.RenterPid,
		RequesterPid: fsStatus.RenterPid,
		RequestTime:  time.Now().UTC(),
	}
	payerPrivKey, err := rss.CtxParams.Cfg.Identity.DecodePrivateKey("")
	if err != nil {
		return err
	}
	cb := make(chan []byte)
	helper.WaitUploadChanMap.Set(rss.SsId, cb)
	if offlineSigning {
		raw, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		err = rss.SaveOfflineSigning(&renterpb.OfflineSigning{
			Raw: raw,
		})
		if err != nil {
			return err
		}
	} else {
		go func() {
			sign, err := crypto.Sign(payerPrivKey, req)
			if err != nil {
				_ = rss.To(sessions.RssToErrorEvent, err)
				return
			}
			cb <- sign
		}()
	}
	sign := <-cb
	helper.WaitUploadChanMap.Remove(rss.SsId)
	if !resume {
		if err := rss.To(sessions.RssToWaitUploadReqSignedEvent); err != nil {
			return err
		}
	}
	req.Signature = sign
	lowRetry := 30 * time.Minute
	highRetry := 24 * time.Hour
	scaledRetry := time.Duration(float64(fsStatus.FileSize) / float64(units.GiB) * float64(highRetry))
	if scaledRetry < lowRetry {
		scaledRetry = lowRetry
	} else if scaledRetry > highRetry {
		scaledRetry = highRetry
	}
	err = backoff.Retry(func() error {
		err = grpc.GuardClient(rss.CtxParams.Cfg.Services.GuardDomain).WithContext(rss.Ctx,
			func(ctx context.Context, client guardpb.GuardServiceClient) error {
				meta, err := client.CheckFileStoreMeta(ctx, req)
				fmt.Println("meta", meta, "err", err)
				if err != nil {
					return err
				}
				num := 0
				m := make(map[string]int)
				for _, c := range meta.Contracts {
					fmt.Println("c.status", c.State.String())
					m[c.State.String()]++
					if c.State == guardpb.Contract_UPLOADED {
						num++
					}
					shard, err := sessions.GetRenterShard(rss.CtxParams, rss.SsId, c.ShardHash, int(c.ShardIndex))
					if err != nil {
						return err
					}
					err = shard.UpdateAdditionalInfo(c.State.String())
					if err != nil {
						return err
					}
				}
				bytes, err := json.Marshal(m)
				if err == nil {
					rss.UpdateAdditionalInfo(string(bytes))
				}
				log.Infof("%d shards uploaded.", num)
				if num >= threshold {
					return nil
				}
				return errors.New("uploading")
			})
		return err
	}, helper.WaitUploadBo(highRetry))
	if err != nil {
		return err
	}
	if err := rss.To(sessions.RssToCompleteEvent); err != nil {
		return err
	}
	return nil
}
