// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/transaction"
	"github.com/TRON-US/go-btfs/transaction/crypto"
	"github.com/TRON-US/go-btfs/transaction/storage"
	"github.com/ethereum/go-ethereum/common"
)

const (
	// prefix for the persistence key
	lastReceivedChequePrefix    = "swap_chequebook_last_received_cheque"
	receivedChequeHistoryPrefix = "swap_chequebook_history_received_cheque"
	//receivedChequeHistoryIndexPrefix = "swap_chequebook_history_received_cheque_index_"
	//180 days
	expireTime = 3600 * 24 * 180
)

var (
	// ErrNoCheque is the error returned if there is no prior cheque for a chequebook or beneficiary.
	ErrNoCheque = errors.New("no cheque")
	// ErrNoChequeRecords is the error returned if there is no prior cheque record for a chequebook or beneficiary.
	ErrNoChequeRecords = errors.New("no cheque records")
	// ErrChequeNotIncreasing is the error returned if the cheque amount is the same or lower.
	ErrChequeNotIncreasing = errors.New("cheque cumulativePayout is not increasing")
	// ErrChequeInvalid is the error returned if the cheque itself is invalid.
	ErrChequeInvalid = errors.New("invalid cheque")
	// ErrWrongBeneficiary is the error returned if the cheque has the wrong beneficiary.
	ErrWrongBeneficiary = errors.New("wrong beneficiary")
	// ErrBouncingCheque is the error returned if the chequebook is demonstrably illiquid.
	ErrBouncingCheque = errors.New("bouncing cheque")
	// ErrChequeValueTooLow is the error returned if the after deduction value of a cheque did not cover 1 accounting credit
	ErrChequeValueTooLow = errors.New("cheque value lower than acceptable")
)

// ChequeStore handles the verification and storage of received cheques
type ChequeStore interface {
	// ReceiveCheque verifies and stores a cheque. It returns the total amount earned.
	ReceiveCheque(ctx context.Context, cheque *SignedCheque, exchangeRate *big.Int) (*big.Int, error)
	// LastReceivedCheque returns the last cheque we received from a specific chequebook.
	LastReceivedCheque(chequebook common.Address) (*SignedCheque, error)
	// LastReceivedCheques returns the last received cheques from every known chequebook.
	LastReceivedCheques() (map[common.Address]*SignedCheque, error)
	// ReceivedChequeRecordsByPeer returns the records we received from a specific chequebook.
	ReceivedChequeRecordsByPeer(chequebook common.Address) ([]ChequeRecord, error)
	// ListReceivedChequeRecords returns the records we received from a specific chequebook.
	ReceivedChequeRecordsAll() (map[common.Address][]ChequeRecord, error)
}

type chequeStore struct {
	lock               sync.Mutex
	store              storage.StateStorer
	factory            Factory
	chaindID           int64
	transactionService transaction.Service
	beneficiary        common.Address // the beneficiary we expect in cheques sent to us
	recoverChequeFunc  RecoverChequeFunc
}

type RecoverChequeFunc func(cheque *SignedCheque, chainID int64) (common.Address, error)

// NewChequeStore creates new ChequeStore
func NewChequeStore(
	store storage.StateStorer,
	factory Factory,
	chainID int64,
	beneficiary common.Address,
	transactionService transaction.Service,
	recoverChequeFunc RecoverChequeFunc) ChequeStore {
	return &chequeStore{
		store:              store,
		factory:            factory,
		chaindID:           chainID,
		transactionService: transactionService,
		beneficiary:        beneficiary,
		recoverChequeFunc:  recoverChequeFunc,
	}
}

// lastReceivedChequeKey computes the key where to store the last cheque received from a chequebook.
func lastReceivedChequeKey(chequebook common.Address) string {
	return fmt.Sprintf("%s_%x", lastReceivedChequePrefix, chequebook)
}

func historyReceivedChequeIndexKey(chequebook common.Address) string {
	return fmt.Sprintf("%s_%x", receivedChequeHistoryPrefix, chequebook)
}

func historyReceivedChequeKey(chequebook common.Address, index uint64) string {
	chequebookStr := chequebook.String()
	return fmt.Sprintf("%s_%x", chequebookStr, index)
}

// LastCheque returns the last cheque we received from a specific chequebook.
func (s *chequeStore) LastReceivedCheque(chequebook common.Address) (*SignedCheque, error) {
	var cheque *SignedCheque
	err := s.store.Get(lastReceivedChequeKey(chequebook), &cheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return nil, ErrNoCheque
	}

	return cheque, nil
}

// ReceiveCheque verifies and stores a cheque. It returns the totam amount earned.
func (s *chequeStore) ReceiveCheque(ctx context.Context, cheque *SignedCheque, exchangeRate *big.Int) (*big.Int, error) {
	// verify we are the beneficiary
	if cheque.Beneficiary != s.beneficiary {
		return nil, ErrWrongBeneficiary
	}

	// don't allow concurrent processing of cheques
	// this would be sufficient on a per chequebook basis
	s.lock.Lock()
	defer s.lock.Unlock()

	// load the lastCumulativePayout for the cheques chequebook
	var lastCumulativePayout *big.Int
	var lastReceivedCheque *SignedCheque
	err := s.store.Get(lastReceivedChequeKey(cheque.Chequebook), &lastReceivedCheque)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}

		// if this is the first cheque from this chequebook, verify with the factory.
		err = s.factory.VerifyChequebook(ctx, cheque.Chequebook)
		if err != nil {
			return nil, err
		}

		lastCumulativePayout = big.NewInt(0)
	} else {
		lastCumulativePayout = lastReceivedCheque.CumulativePayout
	}

	// check this cheque is actually increasing in value
	amount := big.NewInt(0).Sub(cheque.CumulativePayout, lastCumulativePayout)

	if amount.Cmp(big.NewInt(0)) <= 0 {
		return nil, ErrChequeNotIncreasing
	}

	// blockchain calls below
	contract := newChequebookContract(cheque.Chequebook, s.transactionService)
	// this does not change for the same chequebook
	expectedIssuer, err := contract.Issuer(ctx)
	if err != nil {
		return nil, err
	}

	// verify the cheque signature
	issuer, err := s.recoverChequeFunc(cheque, s.chaindID)
	if err != nil {
		return nil, err
	}

	if issuer != expectedIssuer {
		return nil, ErrChequeInvalid
	}

	// basic balance check
	// could be omitted as it is not particularly useful
	balance, err := contract.TotalBalance(ctx)
	if err != nil {
		return nil, err
	}

	alreadyPaidOut, err := contract.PaidOut(ctx, s.beneficiary)
	if err != nil {
		return nil, err
	}

	if balance.Cmp(big.NewInt(0).Sub(cheque.CumulativePayout, alreadyPaidOut)) < 0 {
		return nil, ErrBouncingCheque
	}

	// store the accepted cheque
	err = s.store.Put(lastReceivedChequeKey(cheque.Chequebook), cheque)
	if err != nil {
		return nil, err
	}

	// store the history cheque
	err = s.storeChequeRecord(cheque.Chequebook, amount)
	if err != nil {
		return nil, err
	}
	return amount, nil
}

// ReceivedChequeRecords returns the records we received from a specific chequebook.
func (s *chequeStore) ReceivedChequeRecordsByPeer(chequebook common.Address) ([]ChequeRecord, error) {
	var records []ChequeRecord
	var record ChequeRecord
	var indexrange IndexRange
	err := s.store.Get(historyReceivedChequeIndexKey(chequebook), &indexrange)
	if err != nil {
		if err != storage.ErrNotFound {
			return nil, err
		}
		return nil, ErrNoChequeRecords
	}

	for index := indexrange.MinIndex; index < indexrange.MaxIndex; index++ {
		err = s.store.Get(historyReceivedChequeKey(chequebook, index), &record)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, nil
}

//store cheque record
//Beneficiary common.Address
func (s *chequeStore) storeChequeRecord(chequebook common.Address, amount *big.Int) error {
	var indexRange IndexRange
	err := s.store.Get(historyReceivedChequeIndexKey(chequebook), &indexRange)
	if err != nil {
		if err != storage.ErrNotFound {
			return err
		}
		//not found
		indexRange.MinIndex = 0
		indexRange.MaxIndex = 0
		/*
			err = s.store.Put(historyReceivedChequeIndexKey(chequebook), indexRange)
			if err != nil {
				fmt.Println("put historyReceivedChequeIndexKey err ", err)
				return err
			}
		*/
	}

	//stroe cheque record with the key: historyReceivedChequeKey(index)
	chequeRecord := ChequeRecord{
		chequebook,
		s.beneficiary,
		amount,
		time.Now().Unix(),
	}

	err = s.store.Put(historyReceivedChequeKey(chequebook, indexRange.MaxIndex), chequeRecord)
	if err != nil {
		return err
	}

	//update Max : add one record
	indexRange.MaxIndex += 1
	//delete records if these record are old (half year)
	minIndex, _ := s.deleteRecordsExpired(chequebook, indexRange)

	//uopdate Min: add delete count
	indexRange.MinIndex = minIndex

	//update index
	err = s.store.Put(historyReceivedChequeIndexKey(chequebook), indexRange)
	if err != nil {
		return err
	}

	return nil
}

func (s *chequeStore) deleteRecordsExpired(chequebook common.Address, indexRange IndexRange) (uint64, error) {
	//get the expire time
	expire := time.Now().Unix() - expireTime
	var chequeRecord ChequeRecord
	var endIndex uint64

	//find the last index expired to delete
	for index := indexRange.MinIndex; index < indexRange.MaxIndex; index++ {
		err := s.store.Get(historyReceivedChequeKey(chequebook, index), &chequeRecord)
		if err != nil {
			return indexRange.MinIndex, err
		}

		if chequeRecord.ReceiveTime >= expire {
			endIndex = index
			break
		}
	}

	//delete [min endIndex) records
	if endIndex <= indexRange.MinIndex {
		return indexRange.MinIndex, nil
	}

	//delete expired records
	for index := indexRange.MinIndex; index < endIndex; index++ {
		err := s.store.Delete(historyReceivedChequeKey(chequebook, index))
		if err != nil {
			return indexRange.MinIndex, err
		}
		//min++
		indexRange.MinIndex += 1
	}

	return indexRange.MinIndex, nil
}

// RecoverCheque recovers the issuer ethereum address from a signed cheque
func RecoverCheque(cheque *SignedCheque, chaindID int64) (common.Address, error) {
	eip712Data := eip712DataForCheque(&cheque.Cheque, chaindID)

	pubkey, err := crypto.RecoverEIP712(cheque.Signature, eip712Data)
	if err != nil {
		return common.Address{}, err
	}

	ethAddr, err := crypto.NewEthereumAddress(*pubkey)
	if err != nil {
		return common.Address{}, err
	}

	var issuer common.Address
	copy(issuer[:], ethAddr)
	return issuer, nil
}

// keyChequebook computes the chequebook a store entry is for.
func keyChequebook(key []byte, prefix string) (chequebook common.Address, err error) {
	k := string(key)

	split := strings.SplitAfter(k, prefix)
	if len(split) != 2 {
		return common.Address{}, errors.New("no peer in key")
	}
	return common.HexToAddress(split[1]), nil
}

// LastCheques returns the last received cheques from every known chequebook.
func (s *chequeStore) LastReceivedCheques() (map[common.Address]*SignedCheque, error) {
	result := make(map[common.Address]*SignedCheque)
	err := s.store.Iterate(lastReceivedChequePrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyChequebook(key, lastReceivedChequePrefix+"_")
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := result[addr]; !ok {
			lastCheque, err := s.LastReceivedCheque(addr)
			if err != nil && err != ErrNoCheque {
				return false, err
			} else if err == ErrNoCheque {
				return false, nil
			}

			result[addr] = lastCheque
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListReceivedChequeRecords returns the last received cheques from every known chequebook.
func (s *chequeStore) ReceivedChequeRecordsAll() (map[common.Address][]ChequeRecord, error) {
	result := make(map[common.Address][]ChequeRecord)
	err := s.store.Iterate(receivedChequeHistoryPrefix, func(key, val []byte) (stop bool, err error) {
		addr, err := keyChequebook(key, receivedChequeHistoryPrefix+"_")
		if err != nil {
			return false, fmt.Errorf("parse address from key: %s: %w", string(key), err)
		}

		if _, ok := result[addr]; !ok {
			records, err := s.ReceivedChequeRecordsByPeer(addr)
			if err != nil && err != ErrNoCheque && err != ErrNoChequeRecords {
				return false, err
			} else if err == ErrNoCheque || err == ErrNoChequeRecords {
				return false, nil
			}

			result[addr] = records
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
