package upload

import (
	"github.com/TRON-US/go-btfs/core/escrow"

	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	"github.com/tron-us/protobuf/proto"

	cmap "github.com/orcaman/concurrent-map"
)

var (
	payinReqChanMaps = cmap.New()
)

func pay(rss *RenterSession, result *escrowpb.SignedSubmitContractResult, fileSize int64, offlineSign bool) error {
	rss.to(rssToPayEvent)
	bc := make(chan []byte)
	payinReqChanMaps.Set(rss.ssId, bc)
	if offlineSign {
		errChan := make(chan error)
		go func() {
			//TODO: split to 2 sub function
			chanState := result.Result.BuyerChannelState
			payerPrivKey, err := rss.ctxParams.cfg.Identity.DecodePrivateKey("")
			if err != nil {
				errChan <- err
				return
			}
			sig, err := crypto.Sign(payerPrivKey, chanState.Channel)
			if err != nil {
				errChan <- err
				return
			}
			chanState.FromSignature = sig
			payinReq, err := ledger.NewPayinRequest(result.Result.PayinId, payerPrivKey.GetPublic(), chanState)
			if err != nil {
				errChan <- err
				return
			}
			payinSig, err := crypto.Sign(payerPrivKey, payinReq)
			if err != nil {
				errChan <- err
				return
			}
			request := ledger.NewSignedPayinRequest(payinReq, payinSig)
			bs, err := proto.Marshal(request)
			if err != nil {
				errChan <- err
				return
			}
			errChan <- nil
			bc <- bs
		}()
		err := <-errChan
		if err != nil {
			return err
		}
		rss.to(rssToPayChanStateSignedEvent)
	}
	signed := <-bc
	rss.to(rssToPayPayinRequestSignedEvent)
	signedPayInRequest := new(escrowpb.SignedPayinRequest)
	err := proto.Unmarshal(signed, signedPayInRequest)
	if err != nil {
		return err
	}
	payinRes, err := escrow.PayInToEscrow(rss.ctx, rss.ctxParams.cfg, signedPayInRequest)
	if err != nil {
		//TODO
		return err
	}
	doGuard(rss, payinRes, fileSize, offlineSign)
	return nil
}
