package upload

import (
	"context"
	"errors"
	"fmt"
	cmap "github.com/orcaman/concurrent-map"
	"time"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/cenkalti/backoff/v3"
)

const (
	thresholdContractsNums = 20
)

var (
	waitUploadChanMap = cmap.New()
)

func waitUpload(rss *RenterSession, offlineSigning bool) {
	rss.to(rssToWaitUploadEvent)
	req := &guardpb.CheckFileStoreMetaRequest{
		FileHash:     rss.hash,
		RenterPid:    rss.ctxParams.n.Identity.Pretty(),
		RequesterPid: rss.ctxParams.n.Identity.Pretty(),
		RequestTime:  time.Now().UTC(),
	}
	payerPrivKey, err := rss.ctxParams.cfg.Identity.DecodePrivateKey("")
	if err != nil {
		//TODO
		return
	}
	cb := make(chan []byte)
	waitUploadChanMap.Set(rss.ssId, cb)
	if !offlineSigning {
		go func() {
			sign, err := crypto.Sign(payerPrivKey, req)
			if err != nil {
				//TODO
				return
			}
			cb <- sign
		}()
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
		//TODO
		return
	}
	rss.to(rssToCompleteEvent)
}
