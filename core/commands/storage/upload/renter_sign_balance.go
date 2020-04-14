package upload

import (
	"context"
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"

	cmap "github.com/orcaman/concurrent-map"
)

var (
	balanceChanMaps = cmap.New()
)

func checkBalance(rss *RenterSession, offlineSigning bool, totalPay int64) error {
	bc := make(chan []byte)
	balanceChanMaps.Set(rss.ssId, bc)
	if offlineSigning {
		rss.saveOfflineSigning(&renterpb.OfflineSigning{
			Raw: make([]byte, 0),
		})
	} else {
		go func() {
			privKey, err := rss.ctxParams.cfg.Identity.DecodePrivateKey("")
			if err != nil {
				if err != nil {
					// TODO: error
					return
				}
			}
			lgSignedPubKey, err := ledger.NewSignedPublicKey(privKey, privKey.GetPublic())
			if err != nil {
				if err != nil {
					// TODO: error
					return
				}
			}
			signedBytes, err := proto.Marshal(lgSignedPubKey)
			if err != nil {
				// TODO: error
				return
			}
			bc <- signedBytes
		}()
	}
	signedBytes := <-bc
	rss.to(rssToSubmitBalanceReqSignedEvent)
	balance, err := balanceHelper(rss.ctxParams.ctx, rss.ctxParams.cfg, signedBytes)
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
			res, err := client.BalanceOf(ctx, &ledgerpb.SignedCreateAccountRequest{
				Key:       ledgerSignedPubKey.Key,
				Signature: ledgerSignedPubKey.Signature,
			})
			if err != nil {
				return err
			}
			err = VerifyEscrowRes(configuration, res.Result, res.EscrowSignature)
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

func VerifyEscrowRes(configuration *config.Config, message proto.Message, sig []byte) error {
	escrowPubkey, err := convertPubKeyFromString(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return err
	}
	ok, err := crypto.Verify(escrowPubkey, message, sig)
	if err != nil || !ok {
		return fmt.Errorf("verify escrow failed %v", err)
	}
	return nil
}
