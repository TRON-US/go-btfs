package upload

import (
	"context"
	"errors"
	"math"
	"time"

	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/cenkalti/backoff/v3"
	"github.com/gogo/protobuf/proto"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	thresholdContractsNums = 20
)

func getSuccessThreshold(totalShards int) int {
	return int(math.Min(float64(totalShards), thresholdContractsNums))
}

var (
	waitUploadChanMap = cmap.New()
)

func waitUpload(rss *RenterSession, offlineSigning bool, renterId string) error {
	threshold := getSuccessThreshold(len(rss.shardHashes))
	if err := rss.to(rssToWaitUploadEvent); err != nil {
		return err
	}
	req := &guardpb.CheckFileStoreMetaRequest{
		FileHash:     rss.hash,
		RenterPid:    renterId,
		RequesterPid: renterId,
		RequestTime:  time.Now().UTC(),
	}
	payerPrivKey, err := rss.ctxParams.cfg.Identity.DecodePrivateKey("")
	if err != nil {
		return err
	}
	cb := make(chan []byte)
	waitUploadChanMap.Set(rss.ssId, cb)
	if offlineSigning {
		raw, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		err = rss.saveOfflineSigning(&renterpb.OfflineSigning{
			Raw: raw,
		})
		if err != nil {
			return err
		}
	} else {
		go func() {
			sign, err := crypto.Sign(payerPrivKey, req)
			if err != nil {
				_ = rss.to(rssErrorStatus, err)
				return
			}
			cb <- sign
		}()
	}
	sign := <-cb
	waitUploadChanMap.Remove(rss.ssId)
	if err := rss.to(rssToWaitUploadReqSignedEvent); err != nil {
		return err
	}
	req.Signature = sign
	err = backoff.Retry(func() error {
		select {
		case <-rss.ctx.Done():
			return errors.New("context closed")
		default:
		}
		err = grpc.GuardClient(rss.ctxParams.cfg.Services.GuardDomain).WithContext(rss.ctx,
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
	}, waitUploadBo)
	if err != nil {
		return err
	}
	if err := rss.to(rssToCompleteEvent); err != nil {
		return err
	}
	return nil
}
