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
	ErrNegotiateRate = errors.New("exchange rates mismatch")
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
	GetChainid() int64
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

func (s *Service) getChainID() int64 {
	return s.swap.GetChainid()
}

// SetSwap sets the swap to notify.
func (s *Service) SetSwap(swap Swap) {
	s.swap = swap
}

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
func (s *Service) EmitCheque(ctx context.Context, peer string, beneficiarydel common.Address, amount *big.Int, issue IssueFunc) (balance *big.Int, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	fmt.Println("enter EmitCheque---peer is ", peer)

	// get current global exchangeRate rate
	checkExchangeRate, err := s.priceOracle.CurrentRates()
	if err != nil {
		return nil, err
	}

	fmt.Println("enter EmitCheque---checkExchangeRate is ", checkExchangeRate)

	paymentAmount := new(big.Int).Mul(amount, checkExchangeRate)
	sentAmount := paymentAmount

	fmt.Println("enter EmitCheque---paymentAmount is ", paymentAmount)
	fmt.Println("enter EmitCheque---sentAmount is ", sentAmount)

	//get beneficiary
	beneficiary := &pb.Handshake{}
	/*
		responseChannel := make(chan string, 1)

		var wgResponse sync.WaitGroup
		wgResponse.Add(1)
		// 启动读取结果的控制器
		go func() {
			for response := range responseChannel {
				// 处理结果
				fmt.Println("enter EmitCheque---get channel ")
				beneficiary.Beneficiary = []byte(response)
			}
			// 当 responseChannel被关闭时且channel中所有的值都已经被处理完毕后, 将执行到这一行
			wgResponse.Done()
		}()
	*/

	log.Infof("get beneficiary from peer %v (%v)", peer)
	hostPid, err := peerInfo.IDB58Decode(peer)
	if err != nil {
		log.Infof("peer.IDB58Decode(peer:%s) error: %s", peer, err)
		return nil, err
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
			//get beneficiary
			output, err := remote.P2PCall(ctx, ctxParams.N, ctxParams.Api, hostPid, "/p2p/handshake",
				s.getChainID(),
				hostPid,
			)

			if err != nil {
				fmt.Println("err1 is ", err)
				return err
			}

			err = json.Unmarshal(output, beneficiary)
			if err != nil {
				fmt.Println("err2 is ", err)
				return err
			}
			fmt.Println("beneficiary is ", beneficiary.Beneficiary)

			return nil
		}()
		if err != nil {
			log.Infof("remote.P2PCall hostPid:%s, /p2p/handshake, error: %s", peer, err)
		}

		wg.Done()
	}()

	wg.Wait()

	fmt.Println("beneficiary2 is ", beneficiary.Beneficiary)
	/*
		// issue cheque call with provided callback for sending cheque to finish transaction

		balance, err = issue(ctx, common.BytesToAddress(beneficiary.Beneficiary), sentAmount, func(cheque *chequebook.SignedCheque) error {
			// for simplicity we use json marshaller. can be replaced by a binary encoding in the future.
			encodedCheque, err := json.Marshal(cheque)
			if err != nil {
				return err
			}

			exchangeRate, err := s.priceOracle.CurrentRates()
			if err != nil {
				return err
			}

			fmt.Println("sending cheque message to peer ", peer)

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

						//send cheque
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
						fmt.Println("sending cheque message to peer err ", err)
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
	*/

	fmt.Println("balance is ", balance)
	return balance, nil
}
