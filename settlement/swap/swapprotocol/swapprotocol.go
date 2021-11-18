// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swapprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	"github.com/TRON-US/go-btfs/settlement/swap/priceoracle"
	"github.com/TRON-US/go-btfs/settlement/swap/swapprotocol/pb"

	//"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethereum/go-ethereum/common"
	logging "github.com/ipfs/go-log"
	peerInfo "github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("swapprotocol")
var SwapProtocol *Service

var (
	Req *cmds.Request
	Env cmds.Environment
)

const (
	protocolName    = "swap"
	protocolVersion = "1.0.0"
	streamName      = "swap" // stream for cheques
)

var (
	ErrNegotiateRate  = errors.New("exchange rates mismatch")
	ErrGetBeneficiary = errors.New("get beneficiary err")
)

type SendChequeFunc chequebook.SendChequeFunc

type IssueFunc func(ctx context.Context, beneficiary common.Address, receiver common.Address, amount *big.Int, sendChequeFunc chequebook.SendChequeFunc) (*big.Int, error)

// (context.Context, common.Address, *big.Int, chequebook.SendChequeFunc) (*big.Int, error)

// Interface is the main interface to send messages over swap protocol.
type Interface interface {
	// EmitCheque sends a signed cheque to a peer.
	EmitCheque(ctx context.Context, peer string, amount *big.Int, contractId string, issue IssueFunc) (balance *big.Int, err error)
}

// Swap is the interface the settlement layer should implement to receive cheques.
type Swap interface {
	// ReceiveCheque is called by the swap protocol if a cheque is received.
	ReceiveCheque(ctx context.Context, peer string, cheque *chequebook.SignedCheque, exchangeRate *big.Int) error
	GetChainid() int64
	PutBeneficiary(peer string, beneficiary common.Address) (common.Address, error)
	Beneficiary(peer string) (beneficiary common.Address, known bool, err error)
}

// Service is the main implementation of the swap protocol.
type Service struct {
	swap        Swap
	priceOracle priceoracle.Service
	beneficiary common.Address
}

// New creates a new swap protocol Service.
func New(beneficiary common.Address, priceOracle priceoracle.Service) *Service {
	return &Service{
		beneficiary: beneficiary,
		priceOracle: priceOracle,
	}
}

func (s *Service) GetChainID() int64 {
	return s.swap.GetChainid()
}

// SetSwap sets the swap to notify.
func (s *Service) SetSwap(swap Swap) {
	s.swap = swap
}

func (s *Service) Handler(ctx context.Context, requestPid string, encodedCheque string, exchangeRate *big.Int) (err error) {
	fmt.Printf("Handler requestPid:%s exchangeRate:%+v \n", requestPid, exchangeRate)

	var signedCheque *chequebook.SignedCheque
	err = json.Unmarshal([]byte(encodedCheque), &signedCheque)
	if err != nil {
		return err
	}

	// signature validation
	return s.swap.ReceiveCheque(ctx, requestPid, signedCheque, exchangeRate)
}

// InitiateCheque attempts to send a cheque to a peer.
func (s *Service) EmitCheque(ctx context.Context, peer string, amount *big.Int, contractId string, issue IssueFunc) (balance *big.Int, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// get current global exchangeRate rate
	checkExchangeRate, err := s.priceOracle.CurrentRates()
	if err != nil {
		return nil, err
	}

	paymentAmount := new(big.Int).Mul(amount, checkExchangeRate)
	sentAmount := paymentAmount

	peerhostPid, err := peerInfo.IDB58Decode(peer)
	if err != nil {
		log.Infof("peer.IDB58Decode(peer:%s) error: %s", peer, err)
		return nil, err
	}

	// call P2PCall to get beneficiary address
	handshakeInfo := &pb.Handshake{}
	log.Infof("get handshakeInfo from peer %v (%v)", peerhostPid)
	var wg sync.WaitGroup
	times := 0
	wg.Add(1)
	go func() {
	FETCH_BENEFICIARY:
		err = func() error {
			if times >= 5 {
				log.Warnf("get handshakeInfo from peer %v (%v) error", peerhostPid)
				return ErrGetBeneficiary
			}
			ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
			ctxParams, err := helper.ExtractContextParams(Req, Env)
			if err != nil {
				return err
			}

			//get handshakeInfo
			output, err := remote.P2PCall(ctx, ctxParams.N, ctxParams.Api, peerhostPid, "/p2p/handshake",
				s.GetChainID(),
				ctxParams.N.Identity,
			)

			if err != nil {
				return err
			}

			err = json.Unmarshal(output, handshakeInfo)
			if err != nil {
				return err
			}

			//store beneficiary to db
			_, err = s.swap.PutBeneficiary(peer, common.BytesToAddress(handshakeInfo.Beneficiary))
			if err != nil {
				log.Warnf("put beneficiary (%s) error: %s", handshakeInfo.Beneficiary, err)
				return err
			}

			return nil
		}()
		if err != nil {
			if err != ErrGetBeneficiary {
				times += 1
				goto FETCH_BENEFICIARY
			} else {
				log.Infof("remote.P2PCall hostPid:%s, /p2p/handshake, error: %s", peer, err)
			}
		}
		wg.Done()
	}()

	wg.Wait()

	if times > 5 {
		fmt.Println("get handshakeInfo from peer error", peerhostPid)
		return nil, ErrGetBeneficiary
	}

	fmt.Printf("send cheque: /p2p/handshake ok,beneficiary:%v,receiver:%v ", common.BytesToAddress(handshakeInfo.Beneficiary),
		common.BytesToAddress(handshakeInfo.Receiver))

	// issue cheque call with provided callback for sending cheque to finish transaction
	balance, err = issue(ctx, common.BytesToAddress(handshakeInfo.Beneficiary), common.BytesToAddress(handshakeInfo.Receiver), sentAmount, func(cheque *chequebook.SignedCheque) error {
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
		log.Infof("sending cheque message to peer %v (%v)", peer, cheque)
		{
			hostPid, err := peerInfo.IDB58Decode(peer)
			if err != nil {
				log.Infof("peer.IDB58Decode(peer:%s) error: %s", peer, err)
				return err
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				err = func() error {
					ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
					ctxParams, err := helper.ExtractContextParams(Req, Env)
					if err != nil {
						return err
					}

					fmt.Println("begin send cheque: /storage/upload/cheque, hostPid, contractId = ", hostPid, contractId)

					//send cheque
					_, err = remote.P2PCall(ctx, ctxParams.N, ctxParams.Api, hostPid, "/storage/upload/cheque",
						encodedCheque,
						exchangeRate,
						contractId,
					)
					if err != nil {
						fmt.Println("end send cheque: /storage/upload/cheque, hostPid, contractId, err = ", hostPid, contractId, err)
						return err
					}
					return nil
				}()
				if err != nil {
					log.Infof("remote.P2PCall hostPid:%s, /storage/upload/cheque, error: %s", peer, err)
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

	fmt.Println("send cheque: /storage/upload/cheque ok")

	return balance, nil
}
