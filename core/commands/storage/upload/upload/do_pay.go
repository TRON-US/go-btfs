package upload

import (
	"context"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/escrow"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"
)

func pay(rss *sessions.RenterSession, result *escrowpb.SignedSubmitContractResult, fileSize int64, offlineSigning bool, isRenewContract bool) error {
	if err := rss.To(sessions.RssToPayEvent); err != nil {
		return err
	}
	bc := make(chan []byte)
	uh.PayinReqChanMaps.Set(rss.SsId, bc)
	if offlineSigning {
		raw, err := proto.Marshal(result)
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
			if err := func() error {
				chanState := result.Result.BuyerChannelState
				payerPrivKey, err := rss.CtxParams.Cfg.Identity.DecodePrivateKey("")
				if err != nil {
					return err
				}
				sig, err := crypto.Sign(payerPrivKey, chanState.Channel)
				if err != nil {
					return err
				}
				chanState.FromSignature = sig
				payinReq, err := ledger.NewPayinRequest(result.Result.PayinId, payerPrivKey.GetPublic(), chanState)
				if err != nil {
					return err
				}
				payinSig, err := crypto.Sign(payerPrivKey, payinReq)
				if err != nil {
					return err
				}
				request := ledger.NewSignedPayinRequest(payinReq, payinSig)
				bs, err := proto.Marshal(request)
				if err != nil {
					return err
				}
				bc <- bs
				return nil
			}(); err != nil {
				_ = rss.To(sessions.RssToErrorEvent, err)
				return
			}
		}()
	}
	signed := <-bc
	uh.PayinReqChanMaps.Remove(rss.SsId)
	signedPayInRequest := new(escrowpb.SignedPayinRequest)
	err := proto.Unmarshal(signed, signedPayInRequest)
	if err != nil {
		return err
	}
	if err := rss.To(sessions.RssToPayPayinRequestSignedEvent); err != nil {
		return err
	}
	payinRes, err := payInToEscrow(rss.Ctx, rss.CtxParams.Cfg, signedPayInRequest)
	if err != nil {
		return err
	}
	return doGuard(rss, payinRes, fileSize, offlineSigning, isRenewContract)
}

func payInToEscrow(ctx context.Context, configuration *config.Config, signedPayinReq *escrowpb.SignedPayinRequest) (*escrowpb.SignedPayinResult, error) {
	var signedPayinRes *escrowpb.SignedPayinResult
	err := grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.PayIn(ctx, signedPayinReq)
			if err != nil {
				log.Error(err)
				return err
			}
			err = escrow.VerifyEscrowRes(configuration, res.Result, res.EscrowSignature)
			if err != nil {
				log.Error(err)
				return err
			}
			signedPayinRes = res
			return nil
		})
	if err != nil {
		return nil, err
	}
	return signedPayinRes, nil
}
