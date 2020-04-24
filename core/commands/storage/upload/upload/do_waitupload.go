package upload

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/cenkalti/backoff/v3"
	"github.com/gogo/protobuf/proto"
)

const (
	thresholdContractsNums = 20
)

func getSuccessThreshold(totalShards int) int {
	return int(math.Min(float64(totalShards), thresholdContractsNums))
}

func waitUpload(rss *sessions.RenterSession, offlineSigning bool, renterId string) error {
	threshold := getSuccessThreshold(len(rss.ShardHashes))
	if err := rss.To(sessions.RssToWaitUploadEvent); err != nil {
		return err
	}
	req := &guardpb.CheckFileStoreMetaRequest{
		FileHash:     rss.Hash,
		RenterPid:    renterId,
		RequesterPid: renterId,
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
				_ = rss.To(sessions.RssErrorStatus, err)
				return
			}
			cb <- sign
		}()
	}
	sign := <-cb
	helper.WaitUploadChanMap.Remove(rss.SsId)
	if err := rss.To(sessions.RssToWaitUploadReqSignedEvent); err != nil {
		return err
	}
	req.Signature = sign
	err = backoff.Retry(func() error {
		select {
		case <-rss.Ctx.Done():
			return errors.New("context closed")
		default:
		}
		err = grpc.GuardClient(rss.CtxParams.Cfg.Services.GuardDomain).WithContext(rss.Ctx,
			func(ctx context.Context, client guardpb.GuardServiceClient) error {
				meta, err := client.CheckFileStoreMeta(ctx, req)
				if err != nil {
					return err
				}
				num := 0
				for _, c := range meta.Contracts {
					if c.State == guardpb.Contract_UPLOADED {
						num++
					}
				}
				log.Infof("%d shards uploaded.", num)
				if num >= int(threshold) {
					return nil
				}
				return errors.New("uploading")
			})
		return err
	}, helper.WaitUploadBo)
	if err != nil {
		return err
	}
	if err := rss.To(sessions.RssToCompleteEvent); err != nil {
		return err
	}
	return nil
}
