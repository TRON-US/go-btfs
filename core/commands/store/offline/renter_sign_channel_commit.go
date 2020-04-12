package upload

import (
	"fmt"
	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/protobuf/proto"

	config "github.com/TRON-US/go-btfs-config"

	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"

	ic "github.com/libp2p/go-libp2p-core/crypto"
	cmap "github.com/orcaman/concurrent-map"
)

var (
	unsignedChannelCommitChanMaps = cmap.New()
	signedChannelCommitChanMaps   = cmap.New()
)

func NewContractRequest(rss *RenterSession, signedContracts []*escrowpb.SignedEscrowContract,
	totalPrice int64, offlineSign bool) (*escrowpb.EscrowContractRequest, error) {
	conf := rss.ctxParams.cfg
	ssId := rss.ssId
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
	unsignedChannelCommitChanMaps.Set(ssId, cc)
	cb := make(chan []byte)
	signedChannelCommitChanMaps.Set(ssId, cb)
	if !offlineSign {
		go func() {
			var chanCommit *ledgerpb.ChannelCommit
			var buyerChanSig []byte
			buyerPrivKey, err := conf.Identity.DecodePrivateKey("")
			if err != nil {
				return
			}
			chanCommit, err = ledger.NewChannelCommit(buyerPrivKey.GetPublic(), escrowPubKey, totalPrice)
			if err != nil {
				return
			}
			buyerChanSig, err = crypto.Sign(buyerPrivKey, chanCommit)
			if err != nil {
				return
			}
			commit := ledger.NewSignedChannelCommit(chanCommit, buyerChanSig)
			bs, err := proto.Marshal(commit)
			if err != nil {
				return
			}
			cb <- bs
		}()
	}
	signedBytes := <-cb
	rss.to(rssToSubmitLedgerChannelCommitSignedEvent)
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
	escrowPubKey, err := convertToPubKey(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return nil, err
	}
	return escrowPubKey, nil
}
