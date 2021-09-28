// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/upload"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	"github.com/TRON-US/go-btfs/settlement/swap/priceoracle"
	"github.com/TRON-US/go-btfs/transaction/logging"
	//"github.com/ethersphere/bee/pkg/swarm"

	"github.com/ethereum/go-ethereum/common"
	peerInfo "github.com/libp2p/go-libp2p-core/peer"
)

const (
	protocolName    = "swap"
	protocolVersion = "1.0.0"
	streamName      = "swap" // stream for cheques
)

var (
	ErrNegotiateRate      = errors.New("exchange rates mismatch")
)

type SendChequeFunc chequebook.SendChequeFunc

type IssueFunc func(ctx context.Context, beneficiary common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error)

// (context.Context, common.Address, *big.Int, chequebook.SendChequeFunc) (*big.Int, error)

// Interface is the main interface to send messages over swap protocol.
type Interface interface {
	// EmitCheque sends a signed cheque to a peer.
	EmitCheque(ctx context.Context, peer string, beneficiary common.Address, amount *big.Int, issue IssueFunc) (balance *big.Int, err error)
}

// Swap is the interface the settlement layer should implement to receive cheques.
type Swap interface {
	// ReceiveCheque is called by the swap protocol if a cheque is received.
	ReceiveCheque(ctx context.Context, peer string, cheque *chequebook.SignedCheque, exchangeRate *big.Int) error
}

// Service is the main implementation of the swap protocol.
type Service struct {
	logger      logging.Logger
	swap        Swap
	priceOracle priceoracle.Service
	beneficiary common.Address
}

// New creates a new swap protocol Service.
func New(logger logging.Logger, beneficiary common.Address, priceOracle priceoracle.Service) *Service {
	return &Service{
		logger:      logger,
		beneficiary: beneficiary,
		priceOracle: priceOracle,
	}
}

// SetSwap sets the swap to notify.
func (s *Service) SetSwap(swap Swap) {
	s.swap = swap
}

//func (s *Service) Protocol() p2p.ProtocolSpec {
//	return p2p.ProtocolSpec{
//		Name:    protocolName,
//		Version: protocolVersion,
//		StreamSpecs: []p2p.StreamSpec{
//			{
//				Name:    streamName,
//				Handler: s.handler,
//				Headler: s.headler,
//			},
//		},
//		ConnectOut: s.init,
//		ConnectIn:  s.init,
//	}
//}

func (s *Service) Handler(ctx context.Context, requestPid string, encodedCheque string, exchangeRate *big.Int) (err error) {
	var signedCheque *chequebook.SignedCheque
	err = json.Unmarshal([]byte(encodedCheque), &signedCheque)
	if err != nil {
		return err
	}

	// signature validation
	return s.swap.ReceiveCheque(ctx, requestPid, signedCheque, exchangeRate)
}

// InitiateCheque attempts to send a cheque to a peer.
func (s *Service) EmitCheque(ctx context.Context, peer string, beneficiary common.Address, amount *big.Int, issue IssueFunc) (balance *big.Int, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// get current global exchangeRate rate
	checkExchangeRate, err := s.priceOracle.CurrentRates()
	if err != nil {
		return nil, err
	}

	paymentAmount := new(big.Int).Mul(amount, checkExchangeRate)
	sentAmount := paymentAmount

	// issue cheque call with provided callback for sending cheque to finish transaction

	balance, err = issue(ctx, beneficiary, sentAmount, func(cheque *chequebook.SignedCheque) error {
		// for simplicity we use json marshaller. can be replaced by a binary encoding in the future.
		encodedCheque, err := json.Marshal(cheque)
		if err != nil {
			return err
		}

		exchangeRate, err := s.priceOracle.CurrentRates()
		if err != nil {
			return err
		}

		// sending cheque
		s.logger.Tracef("sending cheque message to peer %v (%v)", peer, cheque)
		{
			hostPid, err := peerInfo.IDB58Decode(peer)
			if err != nil {
				s.logger.Tracef("peer.IDB58Decode(peer:%s) error: %s", peer, err)
				return err
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				err = func() error {
					ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
					ctxParams, err := helper.ExtractContextParams(upload.Req, upload.Env)
					_, err = remote.P2PCall(ctx, ctxParams.N, ctxParams.Api, hostPid, "/storage/upload/cheque",
						encodedCheque,
						exchangeRate,
						)
					if err != nil {
						return err
					}
					return nil
				}()
				if err != nil {
					s.logger.Tracef("remote.P2PCall hostPid:%s, /storage/upload/cheque, error: %s", peer, err)
				}
				wg.Done()
			}()

			wg.Wait()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return balance, nil
}
