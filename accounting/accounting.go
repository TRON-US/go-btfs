// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package accounting provides functionalities needed
// to do per-peer accounting.
package accounting

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pricing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)


// PayFunc is the function used for async monetary settlement
type PayFunc func(context.Context, swarm.Address, *big.Int)

// accountingPeer holds all in-memory accounting information for one peer.
type accountingPeer struct {
	lock                           sync.Mutex // lock to be held during any accounting action for this peer
	lastSettlementFailureTimestamp int64      // time of last unsuccessful attempt to issue a cheque
	connected                      bool
}


// Accounting is the main implementation of the accounting interface.
type Accounting struct {
	// Mutex for accessing the accountingPeers map.
	accountingPeersMu sync.Mutex
	accountingPeers   map[string]*accountingPeer
	logger            logging.Logger
	store             storage.StateStorer

	// function used for monetary settlement
	payFunction PayFunc

	// lower bound for the value of issued cheques
	minimumPayment *big.Int
	pricing        pricing.Interface
	wg             sync.WaitGroup
	p2p            p2p.Service
	timeNow        func() time.Time
}

// NewAccounting creates a new Accounting instance with the provided options.
func NewAccounting(
	Logger logging.Logger,
	Store storage.StateStorer,
	Pricing pricing.Interface,

) (*Accounting, error) {
	return &Accounting{
		accountingPeers:  make(map[string]*accountingPeer),
		logger:           Logger,
		store:            Store,
		pricing:          Pricing,
		timeNow:          time.Now,
	}, nil
}

func (a *Accounting) SetPayFunc(f PayFunc) {
	a.payFunction = f
}

// Close hangs up running websockets on shutdown.
func (a *Accounting) Close() error {
	a.wg.Wait()
	return nil
}

// Settle to a peer. The lock on the accountingPeer must be held when called.
func (a *Accounting) Settle(toPeer swarm.Address, paymentAmount *big.Int) error {
	if paymentAmount.Cmp(a.minimumPayment) >= 0 {
		a.wg.Add(1)
		go a.payFunction(context.Background(), toPeer, paymentAmount)
	}

	return nil
}

// getAccountingPeer returns the accountingPeer for a given swarm address.
// If not found in memory it will initialize it.
func (a *Accounting) getAccountingPeer(peer swarm.Address) *accountingPeer {
	a.accountingPeersMu.Lock()
	defer a.accountingPeersMu.Unlock()

	peerData, ok := a.accountingPeers[peer.String()]
	if !ok {
		peerData = &accountingPeer{
			connected:        false,
		}
		a.accountingPeers[peer.String()] = peerData
	}

	return peerData
}

// NotifyPaymentSent is triggered by async monetary settlement to update our balance and remove it's price from the shadow reserve
func (a *Accounting) NotifyPaymentSent(peer swarm.Address, amount *big.Int, receivedError error) {
	defer a.wg.Done()
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	if receivedError != nil {
		accountingPeer.lastSettlementFailureTimestamp = a.timeNow().Unix()
		a.logger.Warningf("accounting: payment failure %v", receivedError)
		return
	}
}


// NotifyPayment is called by Settlement when we receive a payment.
func (a *Accounting) NotifyPaymentReceived(peer swarm.Address, amount *big.Int) error {
	accountingPeer := a.getAccountingPeer(peer)

	accountingPeer.lock.Lock()
	defer accountingPeer.lock.Unlock()

	a.logger.Tracef("accounting: crediting peer %v with amount %d due to payment.", peer, amount)

	return nil
}
