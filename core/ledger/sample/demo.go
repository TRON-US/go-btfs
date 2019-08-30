package main

import (
	"context"
	"encoding/base64"
	"log"

	"github.com/TRON-US/go-btfs/core/ledger"

	ic "github.com/libp2p/go-libp2p-core/crypto"
)

const (
	PayerPrivKeyString = "CAISIARweyYgg5rglWg3mYmOue0/ekWwl1TwT7nDIzb4MjUm"
	ReceiverPrivKeyString = "CAISIDm/qF5f98Jh8FGBUcFUhQvJPU8uEah1SZrR1BrGekC0"
)

func main()  {
	ctx := context.Background()
	// build connection with ledger
	clientConn, err := ledger.LedgerConnection()
	if err != nil {
		log.Panic("fail to connect", err)
	}
	defer ledger.CloseConnection(clientConn)
	// new ledger client
	ledgerClient := ledger.NewClient(clientConn)
	// create payer Account
	payerPrivKey, err := convertToPrivKey(PayerPrivKeyString)
	if err != nil {
		log.Panic(err)
	}
	payerPubKey := payerPrivKey.GetPublic()
	fromAcc, err := ledger.ImportAccount(ctx, payerPubKey, ledgerClient)
	if err != nil {
		log.Panic(err)
	}
	// create receiver account
	recvPrivKey, err := convertToPrivKey(ReceiverPrivKeyString)
	if err != nil {
		log.Panic(err)
	}
	recvPubKey := recvPrivKey.GetPublic()
	toAcc, err := ledger.ImportAccount(ctx, recvPubKey, ledgerClient)
	if err != nil {
		log.Panic(err)
	}
	// prepare channel commit
	amount := int64(1)
	channelCommit, err := ledger.NewChannelCommit(payerPubKey, recvPubKey, amount)
	if err != nil {
		log.Panic(err)
	}
	// sign for the channel commit
	fromSig, err := ledger.Sign(payerPrivKey, channelCommit)
	if err != nil {
		log.Panic("fail to sign channel commit", err)
	}
	// create channel: payer start the channel
	channelID, err := ledger.CreateChannel(ctx, ledgerClient, channelCommit, fromSig)
	if err != nil {
		log.Panic("fail to create channel", err)
	}
	// channel state: transfer money from -> to
	fromAcc, err = ledger.NewAccount(payerPubKey, 0)
	if err != nil {
		log.Panic(err)
	}
	toAcc, err = ledger.NewAccount(recvPubKey, amount)
	if err != nil {
		log.Panic(err)
	}
	channelState := ledger.NewChannelState(channelID, 1, fromAcc, toAcc)
	// need permission from both account, get signature from both
	fromSigState, err := ledger.Sign(payerPrivKey, channelState)
	toSigState, err := ledger.Sign(recvPrivKey, channelState)
	if err != nil {
		log.Panic("error when signing the channel state", err)
	}
	signedChannelState := ledger.NewSignedChannelState(channelState, fromSigState, toSigState)
	// close channel
	err = ledger.CloseChannel(ctx, signedChannelState)
	if err != nil {
		log.Panic("fail to close channel", err)
	}
}

func convertToPrivKey(key string) (ic.PrivKey, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, err
	}
	return ic.UnmarshalPrivateKey(raw)
}