package main

import (
	"context"
	"log"

	"github.com/TRON-US/go-btfs/core/ledger"
)

func main()  {
	ctx := context.Background()
	// build connection with ledger
	clientConn, err := ledger.LedgerConnection()
	if err != nil {
		log.Panic("fail to connect")
	}
	defer ledger.CloseConnection(clientConn)
	// new ledger client
	ledgerClient := ledger.NewClient(clientConn)
	// create Account
	fromPrivKey, fromAcc, err := ledger.CreateAccount(ctx, ledgerClient)
	toPrivKey, toAcc, err := ledger.CreateAccount(ctx, ledgerClient)
	if err != nil {
		log.Panic("fail to create account")
	}
	// prepare channel commit
	amount := fromAcc.GetBalance() / 10
	channelCommit := ledger.NewChannelCommit(fromAcc.Address.Key, toAcc.Address.Key, amount)
	// sign for the channel commit
	fromSig, err := ledger.Sign(*fromPrivKey, channelCommit)
	if err != nil {
		log.Panic("fail to sign channel commit")
	}
	// create channel: payer start the channel
	channelID, err := ledger.CreateChannel(ctx, ledgerClient, channelCommit, fromSig)
	if err != nil {
		log.Panic("fail to create channel")
	}
	// channel state: transfer money from -> to
	fromAcc = ledger.NewAccount(fromAcc.Address.Key, 0)
	toAcc = ledger.NewAccount(toAcc.Address.Key, amount)
	channelState := ledger.NewChannelState(channelID, 1, fromAcc, toAcc)
	// need permission from both account, get signature from both
	fromSigState, err := ledger.Sign(*fromPrivKey, channelState)
	toSigState, err := ledger.Sign(*toPrivKey, channelState)
	if err != nil {
		log.Panic("error when signing the channel state")
	}
	signedChannelState := ledger.NewSignedChannelState(channelState, fromSigState, toSigState)
	// close channel
	err = ledger.CloseChannel(ctx, ledgerClient, signedChannelState)
	if err != nil {
		log.Panic("fail to close channel")
	}
}