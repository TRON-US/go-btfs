package upload

import (
	"context"
	"errors"
	"fmt"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"
	"github.com/gogo/protobuf/proto"
	"time"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/cenkalti/backoff/v3"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	thresholdContractsNums = 20
)

var (
	waitUploadChanMap = cmap.New()
)

func waitUpload(rss *RenterSession, offlineSigning bool) error {
	rss.to(rssToWaitUploadEvent)
	req := &guardpb.CheckFileStoreMetaRequest{
		FileHash:     rss.hash,
		RenterPid:    rss.ctxParams.n.Identity.Pretty(),
		RequesterPid: rss.ctxParams.n.Identity.Pretty(),
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
		errChan := make(chan error)
		go func() {
			sign, err := crypto.Sign(payerPrivKey, req)
			if err != nil {
				errChan <- err
				return
			}
			errChan <- nil
			cb <- sign
		}()
		if err := <-errChan; err != nil {
			return err
		}
	}
	sign := <-cb
	rss.to(rssToWaitUploadReqSignedEvent)
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
				//TODO: log.Info
				fmt.Printf("%d shards uploaded.\n", num)
				log.Infof("%d shards uploaded.", num)
				if num >= thresholdContractsNums {
					return nil
				}
				return errors.New("uploading")
			})
		return err
	}, waitUploadBo)
	if err != nil {
		return err
	}
	rss.to(rssToCompleteEvent)
	return nil
}
