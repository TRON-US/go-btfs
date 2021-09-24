// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap_test

import (
	"context"
	"errors"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/TRON-US/go-btfs/settlement/swap"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	mockchequebook "github.com/TRON-US/go-btfs/settlement/swap/chequebook/mock"
	mockchequestore "github.com/TRON-US/go-btfs/settlement/swap/chequestore/mock"
	"github.com/TRON-US/go-btfs/settlement/swap/swapprotocol"
	mockstore "github.com/TRON-US/go-btfs/statestore/mock"
	"github.com/TRON-US/go-btfs/transaction/crypto"
	"github.com/TRON-US/go-btfs/transaction/logging"
	"github.com/ethereum/go-ethereum/common"

	peerInfo "github.com/libp2p/go-libp2p-core/peer"

	//"github.com/ethersphere/bee/pkg/swarm"
)

type swapProtocolMock struct {
	emitCheque func(context.Context, string, common.Address, *big.Int, swapprotocol.IssueFunc) (*big.Int, error)
}

func (m *swapProtocolMock) EmitCheque(ctx context.Context, peer string, beneficiary common.Address, value *big.Int, issueFunc swapprotocol.IssueFunc) (*big.Int, error) {
	if m.emitCheque != nil {
		return m.emitCheque(ctx, peer, beneficiary, value, issueFunc)
	}
	return nil, errors.New("not implemented")
}

type testObserver struct {
	receivedCalled chan notifyPaymentReceivedCall
	sentCalled     chan notifyPaymentSentCall
}

type notifyPaymentReceivedCall struct {
	peer   string
	amount *big.Int
}

type notifyPaymentSentCall struct {
	peer   string
	amount *big.Int
	err    error
}

func newTestObserver() *testObserver {
	return &testObserver{
		receivedCalled: make(chan notifyPaymentReceivedCall, 1),
		sentCalled:     make(chan notifyPaymentSentCall, 1),
	}
}

func (t *testObserver) PeerDebt(peer string) (*big.Int, error) {
	return nil, nil
}

func (t *testObserver) NotifyPaymentReceived(peer string, amount *big.Int) error {
	t.receivedCalled <- notifyPaymentReceivedCall{
		peer:   peer,
		amount: amount,
	}
	return nil
}

func (t *testObserver) NotifyRefreshmentReceived(peer string, amount *big.Int) error {
	return nil
}

func (t *testObserver) NotifyPaymentSent(peer string, amount *big.Int, err error) {
	t.sentCalled <- notifyPaymentSentCall{
		peer:   peer,
		amount: amount,
		err:    err,
	}
}

func (t *testObserver) Connect(peer string) {

}

func (t *testObserver) Disconnect(peer string) {

}

type addressbookMock struct {
	migratePeer     func(oldPeer, newPeer string) error
	beneficiary     func(peer string) (beneficiary common.Address, known bool, err error)
	chequebook      func(peer string) (chequebookAddress common.Address, known bool, err error)
	beneficiaryPeer func(beneficiary common.Address) (peer string, known bool, err error)
	chequebookPeer  func(chequebook common.Address) (peer string, known bool, err error)
	putBeneficiary  func(peer string, beneficiary common.Address) error
	putChequebook   func(peer string, chequebook common.Address) error
}

func (m *addressbookMock) MigratePeer(oldPeer, newPeer string) error {
	return m.migratePeer(oldPeer, newPeer)
}
func (m *addressbookMock) Beneficiary(peer string) (beneficiary common.Address, known bool, err error) {
	return m.beneficiary(peer)
}
func (m *addressbookMock) Chequebook(peer string) (chequebookAddress common.Address, known bool, err error) {
	return m.chequebook(peer)
}
func (m *addressbookMock) BeneficiaryPeer(beneficiary common.Address) (peer string, known bool, err error) {
	return m.beneficiaryPeer(beneficiary)
}
func (m *addressbookMock) ChequebookPeer(chequebook common.Address) (peer string, known bool, err error) {
	return m.chequebookPeer(chequebook)
}
func (m *addressbookMock) PutBeneficiary(peer string, beneficiary common.Address) error {
	return m.putBeneficiary(peer, beneficiary)
}
func (m *addressbookMock) PutChequebook(peer string, chequebook common.Address) error {
	return m.putChequebook(peer, chequebook)
}

type cashoutMock struct {
	cashCheque    func(ctx context.Context, chequebook common.Address, recipient common.Address) (common.Hash, error)
	cashoutStatus func(ctx context.Context, chequebookAddress common.Address) (*chequebook.CashoutStatus, error)
}

func (m *cashoutMock) CashCheque(ctx context.Context, chequebook, recipient common.Address) (common.Hash, error) {
	return m.cashCheque(ctx, chequebook, recipient)
}
func (m *cashoutMock) CashoutStatus(ctx context.Context, chequebookAddress common.Address) (*chequebook.CashoutStatus, error) {
	return m.cashoutStatus(ctx, chequebookAddress)
}

func TestReceiveCheque(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()
	chequebookService := mockchequebook.NewChequebook()
	amount := big.NewInt(50)
	exchangeRate := big.NewInt(10)
	chequebookAddress := common.HexToAddress("0xcd")

	peer := peerInfo.ID("abcd").String()
	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("0xab"),
			CumulativePayout: big.NewInt(10),
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	chequeStore := mockchequestore.NewChequeStore(
		mockchequestore.WithReceiveChequeFunc(func(ctx context.Context, c *chequebook.SignedCheque, e *big.Int) (*big.Int, error) {
			if !cheque.Equal(c) {
				t.Fatalf("passed wrong cheque to store. wanted %v, got %v", cheque, c)
			}
			if exchangeRate.Cmp(e) != 0 {
				t.Fatalf("passed wrong exchange rate to store. wanted %v, got %v", exchangeRate, e)
			}
			return amount, nil
		}),
	)

	networkID := uint64(1)
	addressbook := &addressbookMock{
		chequebook: func(p string) (common.Address, bool, error) {
			//if !peer.Equal(p) {
			if strings.Compare(peer, p) != 0 {
				t.Fatal("querying chequebook for wrong peer")
			}
			return chequebookAddress, true, nil
		},
		putChequebook: func(p string, chequebook common.Address) (err error) {
			//if !peer.Equal(p) {
			if strings.Compare(peer, p) != 0 {
				t.Fatal("storing chequebook for wrong peer")
			}
			if chequebook != chequebookAddress {
				t.Fatal("storing wrong chequebook")
			}
			return nil
		},
	}

	observer := newTestObserver()

	swap := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		chequebookService,
		chequeStore,
		addressbook,
		networkID,
		&cashoutMock{},
		observer,
	)

	err := swap.ReceiveCheque(context.Background(), peer, cheque, exchangeRate)
	if err != nil {
		t.Fatal(err)
	}

	expectedAmount := big.NewInt(4)

	select {
	case call := <-observer.receivedCalled:
		if call.amount.Cmp(expectedAmount) != 0 {
			t.Fatalf("observer called with wrong amount. got %d, want %d", call.amount, expectedAmount)
		}

		//if !call.peer.Equal(peer) {
		if strings.Compare(call.peer, peer) != 0 {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peer)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

}

func TestReceiveChequeReject(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()
	chequebookService := mockchequebook.NewChequebook()
	chequebookAddress := common.HexToAddress("0xcd")
	exchangeRate := big.NewInt(10)

	peer := peerInfo.ID("abcd").String()
	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("0xab"),
			CumulativePayout: big.NewInt(10),
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	var errReject = errors.New("reject")

	chequeStore := mockchequestore.NewChequeStore(
		mockchequestore.WithReceiveChequeFunc(func(ctx context.Context, c *chequebook.SignedCheque, e *big.Int) (*big.Int, error) {
			return nil, errReject
		}),
	)
	networkID := uint64(1)
	addressbook := &addressbookMock{
		chequebook: func(p string) (common.Address, bool, error) {
			return chequebookAddress, true, nil
		},
	}

	observer := newTestObserver()

	swap := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		chequebookService,
		chequeStore,
		addressbook,
		networkID,
		&cashoutMock{},
		observer,
	)

	err := swap.ReceiveCheque(context.Background(), peer, cheque, exchangeRate)
	if err == nil {
		t.Fatal("accepted invalid cheque")
	}
	if !errors.Is(err, errReject) {
		t.Fatalf("wrong error. wanted %v, got %v", errReject, err)
	}

	select {
	case <-observer.receivedCalled:
		t.Fatalf("observer called by error.")
	default:
	}

}

func TestReceiveChequeWrongChequebook(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()
	chequebookService := mockchequebook.NewChequebook()
	chequebookAddress := common.HexToAddress("0xcd")
	exchangeRate := big.NewInt(10)

	peer := peerInfo.ID("abcd").String()
	cheque := &chequebook.SignedCheque{
		Cheque: chequebook.Cheque{
			Beneficiary:      common.HexToAddress("0xab"),
			CumulativePayout: big.NewInt(10),
			Chequebook:       chequebookAddress,
		},
		Signature: []byte{},
	}

	chequeStore := mockchequestore.NewChequeStore()
	networkID := uint64(1)
	addressbook := &addressbookMock{
		chequebook: func(p string) (common.Address, bool, error) {
			return common.HexToAddress("0xcfff"), true, nil
		},
	}

	observer := newTestObserver()
	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		chequebookService,
		chequeStore,
		addressbook,
		networkID,
		&cashoutMock{},
		observer,
	)

	err := swapService.ReceiveCheque(context.Background(), peer, cheque, exchangeRate)
	if err == nil {
		t.Fatal("accepted invalid cheque")
	}
	if !errors.Is(err, swap.ErrWrongChequebook) {
		t.Fatalf("wrong error. wanted %v, got %v", swap.ErrWrongChequebook, err)
	}

	select {
	case <-observer.receivedCalled:
		t.Fatalf("observer called by error.")
	default:
	}

}

func TestPay(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	amount := big.NewInt(50)
	beneficiary := common.HexToAddress("0xcd")
	peer := peerInfo.ID("abcd").String()

	networkID := uint64(1)
	addressbook := &addressbookMock{
		beneficiary: func(p string) (common.Address, bool, error) {
			//if !peer.Equal(p) {
			if strings.Compare(peer, p) != 0 {
				t.Fatal("querying beneficiary for wrong peer")
			}
			return beneficiary, true, nil
		},
	}

	observer := newTestObserver()

	var emitCalled bool
	swap := swap.New(
		&swapProtocolMock{
			emitCheque: func(ctx context.Context, p string, b common.Address, a *big.Int, issueFunc swapprotocol.IssueFunc) (*big.Int, error) {
				//if !peer.Equal(p) {
				if strings.Compare(peer, p) != 0 {
					t.Fatal("sending to wrong peer")
				}
				if b != beneficiary {
					t.Fatal("issuing for wrong beneficiary")
				}
				if amount.Cmp(a) != 0 {
					t.Fatal("issuing with wrong amount")
				}
				emitCalled = true
				return amount, nil
			},
		},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		networkID,
		&cashoutMock{},
		observer,
	)

	swap.Pay(context.Background(), peer, amount)

	if !emitCalled {
		t.Fatal("swap protocol was not called")
	}
}

func TestPayIssueError(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	amount := big.NewInt(50)
	beneficiary := common.HexToAddress("0xcd")

	peer := peerInfo.ID("abcd").String()
	errReject := errors.New("reject")
	networkID := uint64(1)
	addressbook := &addressbookMock{
		beneficiary: func(p string) (common.Address, bool, error) {
			//if !peer.Equal(p) {
			if strings.Compare(peer, p) != 0 {
				t.Fatal("querying beneficiary for wrong peer")
			}
			return beneficiary, true, nil
		},
	}

	swap := swap.New(
		&swapProtocolMock{
			emitCheque: func(c context.Context, a1 string, a2 common.Address, i *big.Int, issueFunc swapprotocol.IssueFunc) (*big.Int, error) {
				return nil, errReject
			},
		},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		networkID,
		&cashoutMock{},
		nil,
	)

	observer := newTestObserver()
	swap.SetAccounting(observer)

	swap.Pay(context.Background(), peer, amount)
	select {
	case call := <-observer.sentCalled:

		//if !call.peer.Equal(peer) {
		if strings.Compare(call.peer, peer) != 0 {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peer)
		}
		if !errors.Is(call.err, errReject) {
			t.Fatalf("wrong error. wanted %v, got %v", errReject, call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}

}

func TestPayUnknownBeneficiary(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	amount := big.NewInt(50)
	peer := peerInfo.ID("abcd").String()
	networkID := uint64(1)
	addressbook := &addressbookMock{
		beneficiary: func(p string) (common.Address, bool, error) {
			//if !peer.Equal(p) {
			if strings.Compare(peer, p) != 0 {
				t.Fatal("querying beneficiary for wrong peer")
			}
			return common.Address{}, false, nil
		},
	}

	observer := newTestObserver()

	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		networkID,
		&cashoutMock{},
		observer,
	)

	swapService.Pay(context.Background(), peer, amount)

	select {
	case call := <-observer.sentCalled:
		//if !call.peer.Equal(peer) {
		if strings.Compare(call.peer, peer) != 0 {
			t.Fatalf("observer called with wrong peer. got %v, want %v", call.peer, peer)
		}
		if !errors.Is(call.err, swap.ErrUnknownBeneficary) {
			t.Fatalf("wrong error. wanted %v, got %v", swap.ErrUnknownBeneficary, call.err)
		}

	case <-time.After(time.Second):
		t.Fatal("expected observer to be called")
	}
}

func TestHandshake(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	beneficiary := common.HexToAddress("0xcd")
	networkID := uint64(1)
	txHash := common.HexToHash("0x1")

	peer := crypto.NewOverlayFromEthereumAddress(beneficiary[:], networkID, txHash.Bytes())

	var putCalled bool
	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		&addressbookMock{
			beneficiary: func(p string) (common.Address, bool, error) {
				return beneficiary, true, nil
			},
			beneficiaryPeer: func(common.Address) (peer string, known bool, err error) {
				return peer, true, nil
			},
			migratePeer: func(oldPeer, newPeer string) error {
				return nil
			},
			putBeneficiary: func(p string, b common.Address) error {
				putCalled = true
				return nil
			},
		},
		networkID,
		&cashoutMock{},
		nil,
	)

	err := swapService.Handshake(peer, beneficiary)
	if err != nil {
		t.Fatal(err)
	}

	if putCalled {
		t.Fatal("beneficiary was saved again")
	}
}

func TestHandshakeNewPeer(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	beneficiary := common.HexToAddress("0xcd")
	trx := common.HexToHash("0x1")
	networkID := uint64(1)
	peer := crypto.NewOverlayFromEthereumAddress(beneficiary[:], networkID, trx.Bytes())

	var putCalled bool
	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		&addressbookMock{
			beneficiary: func(p string) (common.Address, bool, error) {
				return beneficiary, false, nil
			},
			beneficiaryPeer: func(beneficiary common.Address) (string, bool, error) {
				return peer, true, nil
			},
			migratePeer: func(oldPeer, newPeer string) error {
				return nil
			},
			putBeneficiary: func(p string, b common.Address) error {
				putCalled = true
				return nil
			},
		},
		networkID,
		&cashoutMock{},
		nil,
	)

	err := swapService.Handshake(peer, beneficiary)
	if err != nil {
		t.Fatal(err)
	}

	if !putCalled {
		t.Fatal("beneficiary was not saved")
	}
}

func TestMigratePeer(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	beneficiary := common.HexToAddress("0xcd")
	trx := common.HexToHash("0x1")
	networkID := uint64(1)
	peer := crypto.NewOverlayFromEthereumAddress(beneficiary[:], networkID, trx.Bytes())

	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		&addressbookMock{
			beneficiaryPeer: func(beneficiary common.Address) (string, bool, error) {
				return peerInfo.ID("00112233").String(), true, nil
			},
			migratePeer: func(oldPeer, newPeer string) error {
				return nil
			},
		},
		networkID,
		&cashoutMock{},
		nil,
	)

	err := swapService.Handshake(peer, beneficiary)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCashout(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	theirChequebookAddress := common.HexToAddress("ffff")
	ourChequebookAddress := common.HexToAddress("fffa")
	//peer := swarm.MustParseHexAddress("abcd")
	peer := peerInfo.ID("abcd").String()

	txHash := common.HexToHash("eeee")
	addressbook := &addressbookMock{
		chequebook: func(p string) (common.Address, bool, error) {
			//if !peer.Equal(p) {
			if strings.Compare(peer, p) != 0 {
				t.Fatal("querying chequebook for wrong peer")
			}
			return theirChequebookAddress, true, nil
		},
	}

	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(
			mockchequebook.WithChequebookAddressFunc(func() common.Address {
				return ourChequebookAddress
			}),
		),
		mockchequestore.NewChequeStore(),
		addressbook,
		uint64(1),
		&cashoutMock{
			cashCheque: func(ctx context.Context, c common.Address, r common.Address) (common.Hash, error) {
				if c != theirChequebookAddress {
					t.Fatalf("not cashing with the right chequebook. wanted %v, got %v", theirChequebookAddress, c)
				}
				if r != ourChequebookAddress {
					t.Fatalf("not cashing with the right recipient. wanted %v, got %v", ourChequebookAddress, r)
				}
				return txHash, nil
			},
		},
		nil,
	)

	returnedHash, err := swapService.CashCheque(context.Background(), peer)
	if err != nil {
		t.Fatal(err)
	}

	if returnedHash != txHash {
		t.Fatalf("go wrong tx hash. wanted %v, got %v", txHash, returnedHash)
	}
}

func TestCashoutStatus(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	store := mockstore.NewStateStore()

	theirChequebookAddress := common.HexToAddress("ffff")
	//peer := swarm.MustParseHexAddress("abcd")
	peer := peerInfo.ID("abcd").String()
	addressbook := &addressbookMock{
		chequebook: func(p string) (common.Address, bool, error) {
			//if !peer.Equal(p) {
			if strings.Compare(peer, p) != 0 {
				t.Fatal("querying chequebook for wrong peer")
			}
			return theirChequebookAddress, true, nil
		},
	}

	expectedStatus := &chequebook.CashoutStatus{}

	swapService := swap.New(
		&swapProtocolMock{},
		logger,
		store,
		mockchequebook.NewChequebook(),
		mockchequestore.NewChequeStore(),
		addressbook,
		uint64(1),
		&cashoutMock{
			cashoutStatus: func(ctx context.Context, c common.Address) (*chequebook.CashoutStatus, error) {
				if c != theirChequebookAddress {
					t.Fatalf("getting status for wrong chequebook. wanted %v, got %v", theirChequebookAddress, c)
				}
				return expectedStatus, nil
			},
		},
		nil,
	)

	returnedStatus, err := swapService.CashoutStatus(context.Background(), peer)
	if err != nil {
		t.Fatal(err)
	}

	if expectedStatus != returnedStatus {
		t.Fatalf("go wrong status. wanted %v, got %v", expectedStatus, returnedStatus)
	}
}

func TestStateStoreKeys(t *testing.T) {
	address := common.HexToAddress("0xabcd")
	//swarmAddress := swarm.MustParseHexAddress("deff")
	swarmAddress := peerInfo.ID("deff").String()

	expected := "swap_chequebook_peer_deff"
	if swap.PeerKey(swarmAddress) != expected {
		t.Fatalf("wrong peer key. wanted %s, got %s", expected, swap.PeerKey(swarmAddress))
	}

	expected = "swap_peer_chequebook_000000000000000000000000000000000000abcd"
	if swap.ChequebookPeerKey(address) != expected {
		t.Fatalf("wrong peer key. wanted %s, got %s", expected, swap.ChequebookPeerKey(address))
	}

	expected = "swap_peer_beneficiary_deff"
	if swap.PeerBeneficiaryKey(swarmAddress) != expected {
		t.Fatalf("wrong peer beneficiary key. wanted %s, got %s", expected, swap.PeerBeneficiaryKey(swarmAddress))
	}

	expected = "swap_beneficiary_peer_000000000000000000000000000000000000abcd"
	if swap.BeneficiaryPeerKey(address) != expected {
		t.Fatalf("wrong beneficiary peer key. wanted %s, got %s", expected, swap.BeneficiaryPeerKey(address))
	}

	expected = "swap_deducted_by_peer_deff"
	if swap.PeerDeductedByKey(swarmAddress) != expected {
		t.Fatalf("wrong peer deducted by key. wanted %s, got %s", expected, swap.PeerDeductedByKey(swarmAddress))
	}

	expected = "swap_deducted_for_peer_deff"
	if swap.PeerDeductedForKey(swarmAddress) != expected {
		t.Fatalf("wrong peer deducted for key. wanted %s, got %s", expected, swap.PeerDeductedForKey(swarmAddress))
	}
}
