// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package settlement

import (
	"errors"
	"math/big"

	//"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrPeerNoSettlements = errors.New("no settlements for peer")
)

// Interface is the interface used by Accounting to trigger settlement
type Interface interface {
	// TotalSent returns the total amount sent to a peer
	TotalSent(peer string) (totalSent *big.Int, err error)
	// TotalReceived returns the total amount received from a peer
	TotalReceived(peer string) (totalSent *big.Int, err error)
	// SettlementsSent returns sent settlements for each individual known peer
	SettlementsSent() (map[string]*big.Int, error)
	// SettlementsReceived returns received settlements for each individual known peer
	SettlementsReceived() (map[string]*big.Int, error)
}

type Accounting interface {
	PeerDebt(peer string) (*big.Int, error)
	NotifyPaymentReceived(peer string, amount *big.Int) error
	NotifyPaymentSent(peer string, amount *big.Int, receivedError error)
	NotifyRefreshmentReceived(peer string, amount *big.Int) error
	Connect(peer string)
	Disconnect(peer string)
}
