package upload

import (
	"context"
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/escrow"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"
)

func checkBalance(rss *sessions.RenterSession, offlineSigning bool, totalPay int64) error {
	bc := make(chan []byte)
	uh.BalanceChanMaps.Set(rss.SsId, bc)
	if offlineSigning {
		err := rss.SaveOfflineSigning(&renterpb.OfflineSigning{
			Raw: make([]byte, 0),
		})
		if err != nil {
			return err
		}
	} else {
		go func() {
			if err := func() error {
				privKey, err := rss.CtxParams.Cfg.Identity.DecodePrivateKey("")
				if err != nil {
					return err
				}
				lgSignedPubKey, err := ledger.NewSignedPublicKey(privKey, privKey.GetPublic())
				if err != nil {
					return err
				}
				signedBytes, err := proto.Marshal(lgSignedPubKey)
				if err != nil {
					return err
				}
				bc <- signedBytes
				return nil
			}(); err != nil {
				_ = rss.To(sessions.RssToErrorEvent, err)
				return
			}
		}()
	}
	signedBytes := <-bc
	if err := rss.To(sessions.RssToSubmitBalanceReqSignedEvent); err != nil {
		return err
	}
	balance, err := balanceHelper(rss.CtxParams.Ctx, rss.CtxParams.Cfg, signedBytes)
	if err != nil {
		return err
	}
	if balance < totalPay {
		return fmt.Errorf("not enough balance to submit contract, current balance is [%v]", balance)
	}
	return nil
}

func balanceHelper(ctx context.Context, configuration *config.Config, signedBytes []byte) (int64, error) {
	var ledgerSignedPubKey ledgerpb.SignedPublicKey
	err := proto.Unmarshal(signedBytes, &ledgerSignedPubKey)
	if err != nil {
		return 0, err
	}
	var balance int64 = 0
	ctx, _ = helper.NewGoContext(ctx)
	err = grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.BalanceOf(ctx, ledger.NewSignedCreateAccountRequest(ledgerSignedPubKey.Key, ledgerSignedPubKey.Signature))
			if err != nil {
				return err
			}
			err = escrow.VerifyEscrowRes(configuration, res.Result, res.EscrowSignature)
			if err != nil {
				return err
			}
			balance = res.Result.Balance
			return nil
		})
	if err != nil {
		return 0, err
	}
	return balance, nil
}
