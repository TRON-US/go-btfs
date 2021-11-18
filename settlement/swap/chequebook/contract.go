// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"context"
	"math/big"

	"github.com/TRON-US/go-btfs/transaction"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type chequebookContract struct {
	address            common.Address
	transactionService transaction.Service
}

func newChequebookContract(address common.Address, transactionService transaction.Service) *chequebookContract {
	return &chequebookContract{
		address:            address,
		transactionService: transactionService,
	}
}

func (c *chequebookContract) Issuer(ctx context.Context) (common.Address, error) {
	callData, err := chequebookABI.Pack("issuer")
	if err != nil {
		return common.Address{}, err
	}

	output, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.address,
		Data: callData,
	})
	if err != nil {
		return common.Address{}, err
	}

	results, err := chequebookABI.Unpack("issuer", output)
	if err != nil {
		return common.Address{}, err
	}

	return *abi.ConvertType(results[0], new(common.Address)).(*common.Address), nil
}

// TotalBalance returns the token balance of the chequebook.
func (c *chequebookContract) TotalBalance(ctx context.Context) (*big.Int, error) {
	callData, err := chequebookABI.Pack("totalbalance")
	if err != nil {
		return nil, err
	}

	output, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.address,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}

	results, err := chequebookABI.Unpack("totalbalance", output)
	if err != nil {
		return nil, err
	}

	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

// LiquidBalance returns the token balance of the chequebook sub stake amount
func (c *chequebookContract) LiquidBalance(ctx context.Context) (*big.Int, error) {
	callData, err := chequebookABI.Pack("liquidBalance")
	if err != nil {
		return nil, err
	}

	output, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.address,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}

	results, err := chequebookABI.Unpack("liquidBalance", output)
	if err != nil {
		return nil, err
	}

	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func (c *chequebookContract) PaidOut(ctx context.Context, address common.Address) (*big.Int, error) {
	callData, err := chequebookABI.Pack("paidOut", address)
	if err != nil {
		return nil, err
	}

	output, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.address,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}

	results, err := chequebookABI.Unpack("paidOut", output)
	if err != nil {
		return nil, err
	}

	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func (c *chequebookContract) TotalPaidOut(ctx context.Context) (*big.Int, error) {
	callData, err := chequebookABI.Pack("totalPaidOut")
	if err != nil {
		return nil, err
	}

	output, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.address,
		Data: callData,
	})
	if err != nil {
		return nil, err
	}

	results, err := chequebookABI.Unpack("totalPaidOut", output)
	if err != nil {
		return nil, err
	}

	return abi.ConvertType(results[0], new(big.Int)).(*big.Int), nil
}

func (c *chequebookContract) Receiver(ctx context.Context) (common.Address, error) {
	callData, err := chequebookABI.Pack("receiver")
	if err != nil {
		return common.Address{}, err
	}

	output, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &c.address,
		Data: callData,
	})
	if err != nil {
		return common.Address{}, err
	}

	results, err := chequebookABI.Unpack("receiver", output)
	if err != nil {
		return common.Address{}, err
	}

	return *abi.ConvertType(results[0], new(common.Address)).(*common.Address), nil
}

func (c *chequebookContract) SetReceiver(ctx context.Context, newReceiver common.Address) (common.Hash, error) {
	callData, err := chequebookABI.Pack("setReciever", newReceiver)
	if err != nil {
		return common.Hash{}, err
	}

	hash, err := c.transactionService.Send(ctx, &transaction.TxRequest{
		To:   &c.address,
		Data: callData,
	})
	if err != nil {
		return hash, err
	}

	return hash, nil
}
