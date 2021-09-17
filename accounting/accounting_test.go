// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting_test

import (
	"context"
	"errors"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/TRON-US/go-btfs/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/statestore/mock"

	"github.com/ethersphere/bee/pkg/swarm"
)

type paymentCall struct {
	peer   swarm.Address
	amount *big.Int
}


//TestAccountingCallSettlementTooSoon
func TestAccountingCallSettlementTooSoon(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(logger, store, nil)
	if err != nil {
		t.Fatal(err)
	}

	paychan := make(chan paymentCall, 1)

	acc.SetPayFunc(func(ctx context.Context, peer swarm.Address, amount *big.Int) {
		paychan <- paymentCall{peer: peer, amount: amount}
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}


	requestPriceTmp := 1000

	select {
	case call := <-paychan:
		if call.amount.Cmp(big.NewInt(int64(requestPriceTmp))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPriceTmp)
		}
		if !call.peer.Equal(peer1Addr) {
			t.Fatalf("wrong peer address got %v wanted %v", call.peer, peer1Addr)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("payment not sent")
	}

	acc.NotifyPaymentSent(peer1Addr, big.NewInt(int64(requestPriceTmp)), errors.New("error"))
	return
}

// NotifyPaymentReceived
func TestAccountingNotifyPaymentReceived(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(logger, store, nil)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	var amoutTmp uint64 = 5000

	err = acc.NotifyPaymentReceived(peer1Addr, new(big.Int).SetUint64(amoutTmp))
	if err != nil {
		t.Fatal(err)
	}

	return
}
