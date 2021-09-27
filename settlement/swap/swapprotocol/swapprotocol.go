// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"math/big"
	"time"

	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	swap "github.com/TRON-US/go-btfs/settlement/swap/headers"
	"github.com/TRON-US/go-btfs/settlement/swap/priceoracle"
	"github.com/TRON-US/go-btfs/settlement/swap/swapprotocol/pb"
	"github.com/TRON-US/go-btfs/transaction/logging"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	//"github.com/ethersphere/bee/pkg/swarm"
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

func (s *Service) headler(receivedHeaders p2p.Headers, peerAddress string) (returnHeaders p2p.Headers) {

	exchangeRate, err := s.priceOracle.CurrentRates()
	if err != nil {
		return p2p.Headers{}
	}

	returnHeaders = swap.MakeSettlementHeaders(exchangeRate)
	return
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

		// sending cheque
		s.logger.Tracef("sending cheque message to peer %v (%v)", peer, cheque)

		//w := protobuf.NewWriter(stream)
		//return w.WriteMsgWithContext(ctx, &pb.EmitCheque{
		//	Cheque: encodedCheque,
		//})

		{
			hostPid, err := peer.IDB58Decode(peer)
			if err != nil {
				log.Errorf("shard %s decodes host_pid error: %s", h, err.Error())
				return err
			}

			cb := make(chan error)

			go func() {
				ctx, _ := context.WithTimeout(rss.Ctx, 10*time.Second)
				ctxParams, err := helper.ExtractContextParams(req, env)
				_, err = remote.P2PCall(ctx, ctxParams.N, ctxParams.Api, hostPid, "/storage/upload/swap",
					encodedCheque,
				)
				if err != nil {
					cb <- err
				}
			}()

			// host needs to send recv in 30 seconds, or the contract will be invalid.
			tick := time.Tick(30 * time.Second)
			select {
			case err = <-cb:
				return err
			case <-tick:
				return errors.New("call /storage/upload/swap timeout")
			}

			//return msg, do issue ok.

		}
	})
	if err != nil {
		return nil, err
	}

	return balance, nil
}
