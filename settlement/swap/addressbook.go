// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	"errors"
	"fmt"

	"github.com/TRON-US/go-btfs/transaction/storage"
	"github.com/ethereum/go-ethereum/common"
	//"github.com/ethersphere/bee/pkg/swarm"
)

var (
	peerPrefix            = "swap_chequebook_peer_"
	peerChequebookPrefix  = "swap_peer_chequebook_"
	beneficiaryPeerPrefix = "swap_beneficiary_peer_"
	peerBeneficiaryPrefix = "swap_peer_beneficiary_"
	deductedForPeerPrefix = "swap_deducted_for_peer_"
	deductedByPeerPrefix  = "swap_deducted_by_peer_"
)

// Addressbook maps peers to beneficaries, chequebooks and in reverse.
type Addressbook interface {
	// Beneficiary returns the beneficiary for the given peer.
	Beneficiary(peer string) (beneficiary common.Address, known bool, err error)
	// Chequebook returns the chequebook for the given peer.
	Chequebook(peer string) (chequebookAddress common.Address, known bool, err error)
	// BeneficiaryPeer returns the peer for a beneficiary.
	BeneficiaryPeer(beneficiary common.Address) (peer string, known bool, err error)
	// ChequebookPeer returns the peer for a beneficiary.
	ChequebookPeer(chequebook common.Address) (peer string, known bool, err error)
	// PutBeneficiary stores the beneficiary for the given peer.
	PutBeneficiary(peer string, beneficiary common.Address) error
	// PutChequebook stores the chequebook for the given peer.
	PutChequebook(peer string, chequebook common.Address) error
	// MigratePeer returns whether a peer have already received a cheque that has been deducted
	MigratePeer(oldPeer, newPeer string) error
}

type addressbook struct {
	store storage.StateStorer
}

// NewAddressbook creates a new addressbook using the store.
func NewAddressbook(store storage.StateStorer) Addressbook {
	return &addressbook{
		store: store,
	}
}

func (a *addressbook) MigratePeer(oldPeer, newPeer string) error {
	ba, known, err := a.Beneficiary(oldPeer)
	if err != nil {
		return err
	}
	if !known {
		return errors.New("old beneficiary not known")
	}

	cb, known, err := a.Chequebook(oldPeer)
	if err != nil {
		return err
	}

	if err := a.PutBeneficiary(newPeer, ba); err != nil {
		return err
	}

	if err := a.store.Delete(peerBeneficiaryKey(oldPeer)); err != nil {
		return err
	}

	if known {
		if err := a.PutChequebook(newPeer, cb); err != nil {
			return err
		}
		if err := a.store.Delete(peerKey(oldPeer)); err != nil {
			return err
		}
	}

	return nil
}

// Beneficiary returns the beneficiary for the given peer.
func (a *addressbook) Beneficiary(peer string) (beneficiary common.Address, known bool, err error) {
	err = a.store.Get(peerBeneficiaryKey(peer), &beneficiary)
	if err != nil {
		if err != storage.ErrNotFound {
			return common.Address{}, false, err
		}
		return common.Address{}, false, nil
	}
	return beneficiary, true, nil
}

// BeneficiaryPeer returns the peer for a beneficiary.
func (a *addressbook) BeneficiaryPeer(beneficiary common.Address) (peer string, known bool, err error) {
	err = a.store.Get(beneficiaryPeerKey(beneficiary), &peer)
	if err != nil {
		if err != storage.ErrNotFound {
			return "", false, err
		}
		return "", false, nil
	}
	return peer, true, nil
}

// Chequebook returns the chequebook for the given peer.
func (a *addressbook) Chequebook(peer string) (chequebookAddress common.Address, known bool, err error) {
	err = a.store.Get(peerKey(peer), &chequebookAddress)
	if err != nil {
		if err != storage.ErrNotFound {
			return common.Address{}, false, err
		}
		return common.Address{}, false, nil
	}
	return chequebookAddress, true, nil
}

// ChequebookPeer returns the peer for a beneficiary.
func (a *addressbook) ChequebookPeer(chequebook common.Address) (peer string, known bool, err error) {
	err = a.store.Get(chequebookPeerKey(chequebook), &peer)
	if err != nil {
		if err != storage.ErrNotFound {
			return "", false, err
		}
		return "", false, nil
	}
	return peer, true, nil
}

// PutBeneficiary stores the beneficiary for the given peer.
func (a *addressbook) PutBeneficiary(peer string, beneficiary common.Address) error {
	err := a.store.Put(peerBeneficiaryKey(peer), beneficiary)
	if err != nil {
		return err
	}
	return a.store.Put(beneficiaryPeerKey(beneficiary), peer)
}

// PutChequebook stores the chequebook for the given peer.
func (a *addressbook) PutChequebook(peer string, chequebook common.Address) error {
	err := a.store.Put(peerKey(peer), chequebook)
	if err != nil {
		return err
	}
	return a.store.Put(chequebookPeerKey(chequebook), peer)
}

// peerKey computes the key where to store the chequebook from a peer.
func peerKey(peer string) string {
	return fmt.Sprintf("%s%s", peerPrefix, peer)
}

// chequebookPeerKey computes the key where to store the peer for a chequebook.
func chequebookPeerKey(chequebook common.Address) string {
	return fmt.Sprintf("%s%x", peerChequebookPrefix, chequebook)
}

// peerBeneficiaryKey computes the key where to store the beneficiary for a peer.
func peerBeneficiaryKey(peer string) string {
	return fmt.Sprintf("%s%s", peerBeneficiaryPrefix, peer)
}

// beneficiaryPeerKey computes the key where to store the peer for a beneficiary.
func beneficiaryPeerKey(peer common.Address) string {
	return fmt.Sprintf("%s%x", beneficiaryPeerPrefix, peer)
}

func peerDeductedByKey(peer string) string {
	return fmt.Sprintf("%s%s", deductedByPeerPrefix, peer)
}

func peerDeductedForKey(peer string) string {
	return fmt.Sprintf("%s%s", deductedForPeerPrefix, peer)
}
