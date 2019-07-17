package main

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/core/ledger"
)

func main()  {
	ctx := context.Background()
	// build connection with ledger
	clientConn, err := ledger.LedgerConnection()
	if err != nil {
		fmt.Println("fail to connect:", err)
	}
	defer ledger.CloseConnection(clientConn)
	// new ledger client
	ledgerClient := ledger.NewClient(clientConn)
	// create Account
	fromPrivKey, fromAcc, err := ledger.CreateAccount(ctx, ledgerClient)
	toPrivKey, toAcc, err := ledger.CreateAccount(ctx, ledgerClient)
	if err != nil {
		fmt.Println("fail to create account: ", err)
	}
	// prepare channel commit
	amount := fromAcc.GetBalance() / 10
	channelCommit := ledger.NewChannelCommit(fromAcc.Address.Key, toAcc.Address.Key, amount)
	// sign for the channel commit
	fromSig, err := ledger.Sign(fromPrivKey, channelCommit)
	if err != nil {
		fmt.Println("fail to sign channel commit", err)
	}
	// create channel: payer start the channel
	channelID, err := ledger.CreateChannel(ctx, ledgerClient, channelCommit, fromSig)
	if err != nil {
		fmt.Println("fail to create channel ", err)
	}
	// channel state: transfer money from -> to
	fromAcc = ledger.NewAccount(fromAcc.Address.Key, 0)
	toAcc = ledger.NewAccount(toAcc.Address.Key, amount)
	channelState := ledger.NewChannelState(channelID, 1, fromAcc, toAcc)
	// need permission from both account, get signature from both
	fromSigState, err := ledger.Sign(fromPrivKey, channelState)
	toSigState, err := ledger.Sign(toPrivKey, channelState)
	if err != nil {
		fmt.Println("error when signing the channel state: ", err)
	}
	signedChannelState := ledger.NewSignedChannelState(channelState, fromSigState, toSigState)
	// close channel
	err = ledger.CloseChannel(ctx, ledgerClient, signedChannelState)
	if err != nil {
		fmt.Println("fail to close channel")
	}
}