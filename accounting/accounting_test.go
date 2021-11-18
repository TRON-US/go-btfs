// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting_test

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/TRON-US/go-btfs/accounting"
	"github.com/TRON-US/go-btfs/statestore/mock"

	peer "github.com/libp2p/go-libp2p-core/peer"
	//"github.com/ethersphere/bee/pkg/swarm"
)

type paymentCall struct {
	peer       string
	amount     *big.Int
	contractId string
}

//TestAccountingCallSettlementTooSoon
func TestAccountingCallSettlementTooSoon(t *testing.T) {
	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(store)
	if err != nil {
		t.Fatal(err)
	}

	paychan := make(chan paymentCall, 1)

	acc.SetPayFunc(func(ctx context.Context, peer string, amount *big.Int, contractId string) {
		paychan <- paymentCall{peer: peer, amount: amount, contractId: contractId}
	})

	peer1Addr := peer.ID("00112233").String()

	requestPriceTmp := 1000

	select {
	case call := <-paychan:
		if call.amount.Cmp(big.NewInt(int64(requestPriceTmp))) != 0 {
			t.Fatalf("paid wrong amount. got %d wanted %d", call.amount, requestPriceTmp)
		}
		if strings.Compare(call.peer, peer1Addr) != 0 {
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
	store := mock.NewStateStore()
	defer store.Close()

	acc, err := accounting.NewAccounting(store)
	if err != nil {
		t.Fatal(err)
	}

	peer1Addr := peer.ID("00112233").String()

	var amoutTmp uint64 = 5000

	err = acc.NotifyPaymentReceived(peer1Addr, new(big.Int).SetUint64(amoutTmp))
	if err != nil {
		t.Fatal(err)
	}

	return
}
