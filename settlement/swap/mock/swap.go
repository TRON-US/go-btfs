// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/TRON-US/go-btfs/settlement/swap"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	"github.com/TRON-US/go-btfs/settlement/swap/swapprotocol"
	//"github.com/ethersphere/bee/pkg/swarm"
)

type Service struct {
	settlementsSent map[string]*big.Int
	settlementsRecv map[string]*big.Int

	settlementSentFunc func(string) (*big.Int, error)
	settlementRecvFunc func(string) (*big.Int, error)

	settlementsSentFunc func() (map[string]*big.Int, error)
	settlementsRecvFunc func() (map[string]*big.Int, error)

	receiveChequeFunc   func(context.Context, string, *chequebook.SignedCheque, *big.Int) error
	payFunc             func(context.Context, string, *big.Int)
	handshakeFunc       func(string, common.Address) error
	lastSentChequeFunc  func(string) (*chequebook.SignedCheque, error)
	lastSentChequesFunc func() (map[string]*chequebook.SignedCheque, error)

	lastReceivedChequeFunc  func(string) (*chequebook.SignedCheque, error)
	lastReceivedChequesFunc func() (map[string]*chequebook.SignedCheque, error)

	cashChequeFunc    func(ctx context.Context, peer string) (common.Hash, error)
	cashoutStatusFunc func(ctx context.Context, peer string) (*chequebook.CashoutStatus, error)
}

// WithSettlementSentFunc sets the mock settlement function
func WithSettlementSentFunc(f func(string) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementSentFunc = f
	})
}

func WithSettlementRecvFunc(f func(string) (*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementRecvFunc = f
	})
}

// WithSettlementsSentFunc sets the mock settlements function
func WithSettlementsSentFunc(f func() (map[string]*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementsSentFunc = f
	})
}

func WithSettlementsRecvFunc(f func() (map[string]*big.Int, error)) Option {
	return optionFunc(func(s *Service) {
		s.settlementsRecvFunc = f
	})
}

func WithReceiveChequeFunc(f func(context.Context, string, *chequebook.SignedCheque, *big.Int) error) Option {
	return optionFunc(func(s *Service) {
		s.receiveChequeFunc = f
	})
}

func WithPayFunc(f func(context.Context, string, *big.Int)) Option {
	return optionFunc(func(s *Service) {
		s.payFunc = f
	})
}

func WithHandshakeFunc(f func(string, common.Address) error) Option {
	return optionFunc(func(s *Service) {
		s.handshakeFunc = f
	})
}

func WithLastSentChequeFunc(f func(string) (*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastSentChequeFunc = f
	})
}

func WithLastSentChequesFunc(f func() (map[string]*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastSentChequesFunc = f
	})
}

func WithLastReceivedChequeFunc(f func(string) (*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastReceivedChequeFunc = f
	})
}

func WithLastReceivedChequesFunc(f func() (map[string]*chequebook.SignedCheque, error)) Option {
	return optionFunc(func(s *Service) {
		s.lastReceivedChequesFunc = f
	})
}

func WithCashChequeFunc(f func(ctx context.Context, peer string) (common.Hash, error)) Option {
	return optionFunc(func(s *Service) {
		s.cashChequeFunc = f
	})
}

func WithCashoutStatusFunc(f func(ctx context.Context, peer string) (*chequebook.CashoutStatus, error)) Option {
	return optionFunc(func(s *Service) {
		s.cashoutStatusFunc = f
	})
}

// New creates the mock swap implementation
func New(opts ...Option) swap.Interface {
	mock := new(Service)
	mock.settlementsSent = make(map[string]*big.Int)
	mock.settlementsRecv = make(map[string]*big.Int)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

func NewSwap(opts ...Option) swapprotocol.Swap {
	mock := new(Service)
	mock.settlementsSent = make(map[string]*big.Int)
	mock.settlementsRecv = make(map[string]*big.Int)

	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// Pay is the mock Pay function of swap.
func (s *Service) Pay(ctx context.Context, peer string, amount *big.Int) {
	if s.payFunc != nil {
		s.payFunc(ctx, peer, amount)
		return
	}
	if settlement, ok := s.settlementsSent[peer]; ok {
		s.settlementsSent[peer] = big.NewInt(0).Add(settlement, amount)
	} else {
		s.settlementsSent[peer] = amount
	}
}

// TotalSent is the mock TotalSent function of swap.
func (s *Service) TotalSent(peer string) (totalSent *big.Int, err error) {
	if s.settlementSentFunc != nil {
		return s.settlementSentFunc(peer)
	}
	if v, ok := s.settlementsSent[peer]; ok {
		return v, nil
	}
	return big.NewInt(0), nil
}

// TotalReceived is the mock TotalReceived function of swap.
func (s *Service) TotalReceived(peer string) (totalReceived *big.Int, err error) {
	if s.settlementRecvFunc != nil {
		return s.settlementRecvFunc(peer)
	}
	if v, ok := s.settlementsRecv[peer]; ok {
		return v, nil
	}
	return big.NewInt(0), nil
}

// SettlementsSent is the mock SettlementsSent function of swap.
func (s *Service) SettlementsSent() (map[string]*big.Int, error) {
	if s.settlementsSentFunc != nil {
		return s.settlementsSentFunc()
	}
	return s.settlementsSent, nil
}

// SettlementsReceived is the mock SettlementsReceived function of swap.
func (s *Service) SettlementsReceived() (map[string]*big.Int, error) {
	if s.settlementsRecvFunc != nil {
		return s.settlementsRecvFunc()
	}
	return s.settlementsRecv, nil
}

// Handshake is called by the swap protocol when a handshake is received.
func (s *Service) Handshake(peer string, beneficiary common.Address) error {
	if s.handshakeFunc != nil {
		return s.handshakeFunc(peer, beneficiary)
	}
	return nil
}

func (s *Service) LastSentCheque(address string) (*chequebook.SignedCheque, error) {
	if s.lastSentChequeFunc != nil {
		return s.lastSentChequeFunc(address)
	}
	return nil, nil
}

func (s *Service) LastSentCheques() (map[string]*chequebook.SignedCheque, error) {
	if s.lastSentChequesFunc != nil {
		return s.lastSentChequesFunc()
	}
	return nil, nil
}

func (s *Service) LastReceivedCheque(address string) (*chequebook.SignedCheque, error) {
	if s.lastReceivedChequeFunc != nil {
		return s.lastReceivedChequeFunc(address)
	}
	return nil, nil
}

func (s *Service) LastReceivedCheques() (map[string]*chequebook.SignedCheque, error) {
	if s.lastReceivedChequesFunc != nil {
		return s.lastReceivedChequesFunc()
	}
	return nil, nil
}

func (s *Service) CashCheque(ctx context.Context, peer string) (common.Hash, error) {
	if s.cashChequeFunc != nil {
		return s.cashChequeFunc(ctx, peer)
	}
	return common.Hash{}, nil
}

func (s *Service) CashoutStatus(ctx context.Context, peer string) (*chequebook.CashoutStatus, error) {
	if s.cashoutStatusFunc != nil {
		return s.cashoutStatusFunc(ctx, peer)
	}
	return nil, nil
}

func (s *Service) ReceiveCheque(ctx context.Context, peer string, cheque *chequebook.SignedCheque, exchangeRate *big.Int) (err error) {
	if s.receiveChequeFunc != nil {
		return s.receiveChequeFunc(ctx, peer, cheque, exchangeRate)
	}

	return nil
}

// Option is the option passed to the mock settlement service
type Option interface {
	apply(*Service)
}

type optionFunc func(*Service)

func (f optionFunc) apply(r *Service) { f(r) }
