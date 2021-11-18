// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/TRON-US/go-btfs/settlement"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	"github.com/TRON-US/go-btfs/settlement/swap/swapprotocol"
	"github.com/TRON-US/go-btfs/transaction/storage"
	"github.com/ethereum/go-ethereum/common"
	logging "github.com/ipfs/go-log"
	//"github.com/ethersphere/bee/pkg/swarm"
)

var log = logging.Logger("swap")

var (
	// ErrWrongChequebook is the error if a peer uses a different chequebook from before.
	ErrWrongChequebook = errors.New("wrong chequebook")
	// ErrWrongReceiver is the error if a peer uses a different receiver.
	ErrWrongReceiver = errors.New("wrong receiver")
	// ErrUnknownBeneficary is the error if a peer has never announced a beneficiary.
	ErrUnknownBeneficary = errors.New("unknown beneficiary for peer")
	// ErrChequeValueTooLow is the error a peer issued a cheque not covering 1 accounting credit
	ErrChequeValueTooLow = errors.New("cheque value too low")
)

type Interface interface {
	settlement.Interface

	// LastReceivedCheque returns the last received cheque for the peer
	LastReceivedCheque(peer string) (*chequebook.SignedCheque, error)
	// LastReceivedCheques returns the list of last received cheques for all peers
	LastReceivedCheques() (map[string]*chequebook.SignedCheque, error)
	// ReceivedChequeRecordsByPeer gets the records of the cheque for the peer's chequebook
	ReceivedChequeRecordsByPeer(peer string) ([]chequebook.ChequeRecord, error)
	// ReceivedChequeRecordsAll gets the records of the cheque for the peer's chequebook
	ReceivedChequeRecordsAll() ([]chequebook.ChequeRecord, error)

	// LastReceivedCheque returns the last received cheque for the peer
	LastSendCheque(peer string) (*chequebook.SignedCheque, error)
	// LastSendCheques returns the list of last send cheques for this peer
	LastSendCheques() (map[string]*chequebook.SignedCheque, error)

	// CashCheque sends a cashing transaction for the last cheque of the peer
	CashCheque(ctx context.Context, peer string) (common.Hash, error)
	// CashoutStatus gets the status of the latest cashout transaction for the peers chequebook
	CashoutStatus(ctx context.Context, peer string) (*chequebook.CashoutStatus, error)
	HasCashoutAction(ctx context.Context, peer string) (bool, error)
}

// Service is the implementation of the swap settlement layer.
type Service struct {
	proto       swapprotocol.Interface
	store       storage.StateStorer
	accounting  settlement.Accounting
	metrics     metrics
	chequebook  chequebook.Service
	chequeStore chequebook.ChequeStore
	cashout     chequebook.CashoutService
	addressbook Addressbook
	chainID     int64
}

// New creates a new swap Service.
func New(proto swapprotocol.Interface, store storage.StateStorer, chequebook chequebook.Service, chequeStore chequebook.ChequeStore, addressbook Addressbook, chainID int64, cashout chequebook.CashoutService, accounting settlement.Accounting) *Service {
	return &Service{
		proto:       proto,
		store:       store,
		metrics:     newMetrics(),
		chequebook:  chequebook,
		chequeStore: chequeStore,
		addressbook: addressbook,
		chainID:     chainID,
		cashout:     cashout,
		accounting:  accounting,
	}
}

func (s *Service) GetProtocols() swapprotocol.Interface {
	return s.proto
}

// ReceiveCheque is called by the swap protocol if a cheque is received.
func (s *Service) ReceiveCheque(ctx context.Context, peer string, cheque *chequebook.SignedCheque, exchangeRate *big.Int) (err error) {
	// check this is the same chequebook for this peer as previously
	expectedChequebook, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return err
	}

	if known && expectedChequebook != cheque.Chequebook {
		return ErrWrongChequebook
	}

	receiver, err := s.chequebook.Receiver()
	if err != nil {
		return err
	}
	if receiver != cheque.Receiver {
		return ErrWrongReceiver
	}

	receivedAmount, err := s.chequeStore.ReceiveCheque(ctx, cheque, exchangeRate)
	if err != nil {
		s.metrics.ChequesRejected.Inc()
		return fmt.Errorf("rejecting cheque: %w", err)
	}

	decreasedAmount := receivedAmount
	amount := new(big.Int).Div(decreasedAmount, exchangeRate)

	if !known {
		err = s.addressbook.PutChequebook(peer, cheque.Chequebook)
		if err != nil {
			return err
		}
	}

	tot, _ := big.NewFloat(0).SetInt(receivedAmount).Float64()
	s.metrics.TotalReceived.Add(tot)
	s.metrics.ChequesReceived.Inc()

	return s.accounting.NotifyPaymentReceived(peer, amount)
}

// ReceiveCheque is called by the swap protocol if a cheque is received.
func (s *Service) PutChequebookWhenSendCheque(peer string, chequebook common.Address) (err error) {
	// check this is the same chequebook for this peer as previously
	_, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return err
	}

	if !known {
		err = s.addressbook.PutChequebook(peer, chequebook)
		if err != nil {
			return err
		}
	}

	return nil
}

// Pay initiates a payment to the given peer
func (s *Service) Pay(ctx context.Context, peer string, amount *big.Int, contractId string) {
	var err error
	defer func() {
		if err != nil {
			s.accounting.NotifyPaymentSent(peer, amount, err)
		}
	}()

	/*
		beneficiary, known, err := s.addressbook.Beneficiary(peer)
		if err != nil {
			return
		}
		if !known {
			err = ErrUnknownBeneficary
			return
		}
	*/
	balance, err := s.proto.EmitCheque(ctx, peer, amount, contractId, s.chequebook.Issue)

	if err != nil {
		return
	}

	bal, _ := big.NewFloat(0).SetInt(balance).Float64()
	s.metrics.AvailableBalance.Set(bal)
	s.accounting.NotifyPaymentSent(peer, amount, nil)
	amountFloat, _ := big.NewFloat(0).SetInt(amount).Float64()
	s.metrics.TotalSent.Add(amountFloat)
	s.metrics.ChequesSent.Inc()
}

func (s *Service) SetAccounting(accounting settlement.Accounting) {
	s.accounting = accounting
}

// TotalSent returns the total amount sent to a peer
func (s *Service) TotalSent(peer string) (totalSent *big.Int, err error) {
	beneficiary, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, settlement.ErrPeerNoSettlements
	}
	cheque, err := s.chequebook.LastCheque(beneficiary)
	if err != nil {
		if err == chequebook.ErrNoCheque {
			return nil, settlement.ErrPeerNoSettlements
		}
		return nil, err
	}
	return cheque.CumulativePayout, nil
}

// TotalReceived returns the total amount received from a peer
func (s *Service) TotalReceived(peer string) (totalReceived *big.Int, err error) {
	chequebookAddress, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, settlement.ErrPeerNoSettlements
	}

	cheque, err := s.chequeStore.LastReceivedCheque(chequebookAddress)
	if err != nil {
		if err == chequebook.ErrNoCheque {
			return nil, settlement.ErrPeerNoSettlements
		}
		return nil, err
	}
	return cheque.CumulativePayout, nil
}

// SettlementsSent returns sent settlements for each individual known peer
func (s *Service) SettlementsSent() (map[string]*big.Int, error) {
	result := make(map[string]*big.Int)
	cheques, err := s.chequebook.LastCheques()
	if err != nil {
		return nil, err
	}

	for beneficiary, cheque := range cheques {
		peer, known, err := s.addressbook.BeneficiaryPeer(beneficiary)
		if err != nil {
			return nil, err
		}
		if !known {
			continue
		}
		result[peer] = cheque.CumulativePayout
	}

	return result, nil
}

// SettlementsReceived returns received settlements for each individual known peer.
func (s *Service) SettlementsReceived() (map[string]*big.Int, error) {
	result := make(map[string]*big.Int)
	cheques, err := s.chequeStore.LastReceivedCheques()
	if err != nil {
		return nil, err
	}

	for chequebook, cheque := range cheques {
		peer, known, err := s.addressbook.ChequebookPeer(chequebook)
		if err != nil {
			return nil, err
		}
		if !known {
			continue
		}
		result[peer] = cheque.CumulativePayout
	}
	return result, err
}

// Handshake is called by the swap protocol when a handshake is received.
func (s *Service) Handshake(peer string, beneficiary common.Address) error {
	oldPeer, known, err := s.addressbook.BeneficiaryPeer(beneficiary)
	if err != nil {
		return err
	}
	if known && strings.Compare(peer, oldPeer) != 0 {
		log.Debugf("migrating swap addresses from peer %s to %s", oldPeer, peer)
		return s.addressbook.MigratePeer(oldPeer, peer)
	}

	_, known, err = s.addressbook.Beneficiary(peer)
	if err != nil {
		return err
	}
	if !known {
		log.Infof("initial swap handshake peer: %v beneficiary: %x", peer, beneficiary)
		return s.addressbook.PutBeneficiary(peer, beneficiary)
	}

	return nil
}

// LastReceivedCheque returns the last received cheque for the peer
func (s *Service) LastReceivedCheque(peer string) (*chequebook.SignedCheque, error) {

	common, known, err := s.addressbook.Chequebook(peer)

	if err != nil {
		return nil, err
	}

	if !known {
		return nil, chequebook.ErrNoCheque
	}

	return s.chequeStore.LastReceivedCheque(common)
}

// LastReceivedCheques returns the list of last received cheques for all peers
func (s *Service) LastReceivedCheques() (map[string]*chequebook.SignedCheque, error) {
	lastcheques, err := s.chequeStore.LastReceivedCheques()
	if err != nil {
		return nil, err
	}

	resultmap := make(map[string]*chequebook.SignedCheque, len(lastcheques))

	for i, j := range lastcheques {
		addr, known, err := s.addressbook.ChequebookPeer(i)
		if err == nil && known {
			resultmap[addr] = j
		}
	}

	return resultmap, nil
}

// LastReceivedCheque returns the last received cheque for the peer
func (s *Service) LastSendCheque(peer string) (*chequebook.SignedCheque, error) {
	comm, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return nil, err
	}

	if !known {
		return nil, chequebook.ErrNoCheque
	}

	return s.chequebook.LastCheque(comm)
}

// LastReceivedCheques returns the list of last received cheques for all peers
func (s *Service) LastSendCheques() (map[string]*chequebook.SignedCheque, error) {
	lastcheques, err := s.chequebook.LastCheques()
	if err != nil {
		return nil, err
	}

	resultmap := make(map[string]*chequebook.SignedCheque, len(lastcheques))

	for i, j := range lastcheques {
		addr, known, err := s.addressbook.ChequebookPeer(i)
		if err == nil && known {
			resultmap[addr] = j
		}
	}

	return resultmap, nil
}

// CashCheque sends a cashing transaction for the last cheque of the peer
func (s *Service) CashCheque(ctx context.Context, peer string) (common.Hash, error) {
	chequebookAddress, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return common.Hash{}, err
	}
	if !known {
		return common.Hash{}, chequebook.ErrNoCheque
	}
	return s.cashout.CashCheque(ctx, chequebookAddress)
}

// CashoutStatus gets the status of the latest cashout transaction for the peers chequebook
func (s *Service) CashoutStatus(ctx context.Context, peer string) (*chequebook.CashoutStatus, error) {
	chequebookAddress, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return nil, err
	}
	if !known {
		return nil, chequebook.ErrNoCheque
	}
	return s.cashout.CashoutStatus(ctx, chequebookAddress)
}

func (s *Service) GetChainid() int64 {
	return s.chainID
}

func (s *Service) Settle(toPeer string, paymentAmount *big.Int, contractId string) error {
	return s.accounting.Settle(toPeer, paymentAmount, contractId)
}

func (s *Service) PutBeneficiary(peer string, beneficiary common.Address) (common.Address, error) {
	last_address, known, err := s.addressbook.Beneficiary(peer)
	if err != nil {
		return beneficiary, err
	}
	if !known {
		return beneficiary, s.addressbook.PutBeneficiary(peer, beneficiary)
	}

	//compare last_address and beneficiary
	if !bytes.Equal(last_address.Bytes(), beneficiary.Bytes()) {
		s.addressbook.MigratePeer(last_address.String(), beneficiary.String())
	}

	return beneficiary, nil
}

// LastReceivedCheque returns the last received cheque for the peer
func (s *Service) ReceivedChequeRecordsByPeer(peer string) ([]chequebook.ChequeRecord, error) {
	common, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return nil, err
	}

	if !known {
		return nil, chequebook.ErrNoCheque
	}

	return s.chequeStore.ReceivedChequeRecordsByPeer(common)
}

// ReceivedChequeRecordsAll returns the last received cheque for the peer
func (s *Service) ReceivedChequeRecordsAll() ([]chequebook.ChequeRecord, error) {
	mp, err := s.chequeStore.ReceivedChequeRecordsAll()
	if err != nil {
		return nil, err
	}

	records := make([]chequebook.ChequeRecord, 0)
	for comm, _ := range mp {
		l, err := s.chequeStore.ReceivedChequeRecordsByPeer(comm)
		if err != nil {
			return nil, err
		}

		for _, a := range l {
			records = append(records, a)
		}
	}

	return records, nil
}

func (s *Service) Beneficiary(peer string) (beneficiary common.Address, known bool, err error) {
	return s.addressbook.Beneficiary(peer)
}

func (s *Service) HasCashoutAction(ctx context.Context, peer string) (bool, error) {
	chequebook, known, err := s.addressbook.Chequebook(peer)
	if err != nil {
		return false, err
	}
	if !known {
		return false, fmt.Errorf("unkonw peer")
	}
	return s.cashout.HasCashoutAction(ctx, chequebook)
}
