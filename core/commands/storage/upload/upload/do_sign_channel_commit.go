package upload

import (
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/protobuf/proto"

	ic "github.com/libp2p/go-libp2p-core/crypto"
	pb "github.com/libp2p/go-libp2p-core/crypto/pb"
)

// fork from ic, use RawFull rather than Raw
func marshalPublicKey(k ic.PubKey) ([]byte, error) {
	pbmes := new(pb.PublicKey)
	pbmes.Type = k.Type()
	data, err := ic.RawFull(k)
	if err != nil {
		return nil, err
	}
	pbmes.Data = data

	return proto.Marshal(pbmes)
}

func NewContractRequest(rss *sessions.RenterSession, signedContracts []*escrowpb.SignedEscrowContract,
	totalPrice int64, offlineSigning bool) (*escrowpb.EscrowContractRequest, error) {
	conf := rss.CtxParams.Cfg
	ssId := rss.SsId
	escrowPubKey, err := newContractRequestHelper(conf)
	if err != nil {
		return nil, err
	}
	pubK, err := ic.MarshalPublicKey(escrowPubKey)
	if err != nil {
		return nil, err
	}
	cc := &ledgerpb.ChannelCommit{
		Payer:     nil,
		Recipient: &ledgerpb.PublicKey{Key: pubK},
		Amount:    0,
		PayerId:   0,
	}
	uh.UnsignedChannelCommitMaps.Set(ssId, cc)
	cb := make(chan []byte)
	uh.SignedChannelCommitChanMaps.Set(ssId, cb)
	if offlineSigning {
		raw, err := marshalPublicKey(escrowPubKey)
		if err != nil {
			return nil, err
		}
		err = rss.SaveOfflineSigning(&renterpb.OfflineSigning{
			Raw:   raw,
			Price: totalPrice,
		})
		if err != nil {
			return nil, err
		}
	} else {
		go func() {
			if err := func() error {
				var chanCommit *ledgerpb.ChannelCommit
				var buyerChanSig []byte
				buyerPrivKey, err := conf.Identity.DecodePrivateKey("")
				if err != nil {
					return err
				}
				chanCommit, err = ledger.NewChannelCommit(buyerPrivKey.GetPublic(), escrowPubKey, totalPrice)
				if err != nil {
					return err
				}
				buyerChanSig, err = crypto.Sign(buyerPrivKey, chanCommit)
				if err != nil {
					return err
				}
				commit := ledger.NewSignedChannelCommit(chanCommit, buyerChanSig)
				bs, err := proto.Marshal(commit)
				if err != nil {
					return err
				}
				cb <- bs
				return nil
			}(); err != nil {
				_ = rss.To(sessions.RssErrorStatus, err)
			}
		}()
	}
	signedBytes := <-cb
	if err := rss.To(sessions.RssToSubmitLedgerChannelCommitSignedEvent); err != nil {
		return nil, err
	}
	var signedChannelCommit ledgerpb.SignedChannelCommit
	err = proto.Unmarshal(signedBytes, &signedChannelCommit)
	if err != nil {
		return nil, err
	}
	return &escrowpb.EscrowContractRequest{
		Contract:     signedContracts,
		BuyerChannel: &signedChannelCommit,
	}, nil
}

func newContractRequestHelper(configuration *config.Config) (ic.PubKey, error) {
	// prepare channel commit
	if len(configuration.Services.EscrowPubKeys) == 0 {
		return nil, fmt.Errorf("No Services.EscrowPubKeys are set in config")
	}
	var escrowPubKey ic.PubKey
	escrowPubKey, err := helper.ConvertToPubKey(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return nil, err
	}
	return escrowPubKey, nil
}
