// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	conabi "github.com/TRON-US/go-btfs/chain/abi"
	"github.com/TRON-US/go-btfs/transaction"
	"github.com/TRON-US/go-btfs/transaction/sctx"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/net/context"
)

var (
	ErrInvalidFactory       = errors.New("not a valid factory contract")
	ErrNotDeployedByFactory = errors.New("chequebook not deployed by factory")
	errDecodeABI            = errors.New("could not decode abi data")

	factoryABI                  = transaction.ParseABIUnchecked(conabi.SimpleSwapFactoryABI)
	simpleSwapDeployedEventType = factoryABI.Events["SimpleSwapDeployed"]
)

// Factory is the main interface for interacting with the chequebook factory.
type Factory interface {
	// ERC20Address returns the token for which this factory deploys chequebooks.
	ERC20Address(ctx context.Context) (common.Address, error)
	// Deploy deploys a new chequebook and returns once the transaction has been submitted.
	Deploy(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int, nonce common.Hash) (common.Hash, error)
	// WaitDeployed waits for the deployment transaction to confirm and returns the chequebook address
	WaitDeployed(ctx context.Context, txHash common.Hash) (common.Address, error)
	// VerifyBytecode checks that the factory is valid.
	VerifyBytecode(ctx context.Context) error
	// VerifyChequebook checks that the supplied chequebook has been deployed by this factory.
	VerifyChequebook(ctx context.Context, chequebook common.Address) error
}

type factory struct {
	backend            transaction.Backend
	transactionService transaction.Service
	address            common.Address // address of the factory to use for deployments
}

type simpleSwapDeployedEvent struct {
	ContractAddress common.Address
}

// the bytecode of factories which can be used for deployment
var currentDeployVersion []byte = common.FromHex(conabi.FactoryDeployedBin)

// NewFactory creates a new factory service for the provided factory contract.
func NewFactory(backend transaction.Backend, transactionService transaction.Service, address common.Address) Factory {
	return &factory{
		backend:            backend,
		transactionService: transactionService,
		address:            address,
	}
}

// Deploy deploys a new chequebook and returns once the transaction has been submitted.
func (c *factory) Deploy(ctx context.Context, issuer common.Address, defaultHardDepositTimeoutDuration *big.Int, nonce common.Hash) (common.Hash, error) {
	callData, err := factoryABI.Pack("deploySimpleSwap", issuer, nonce)
	if err != nil {
		return common.Hash{}, err
	}

	request := &transaction.TxRequest{
		To:          &c.address,
		Data:        callData,
		GasPrice:    sctx.GetGasPrice(ctx),
		GasLimit:    175000,
		Value:       big.NewInt(0),
		Description: "chequebook deployment",
	}

	txHash, err := c.transactionService.Send(ctx, request)
	if err != nil {
		return common.Hash{}, err
	}

	return txHash, nil
}

// WaitDeployed waits for the deployment transaction to confirm and returns the chequebook address
func (c *factory) WaitDeployed(ctx context.Context, txHash common.Hash) (common.Address, error) {
	receipt, err := c.transactionService.WaitForReceipt(ctx, txHash)
	if err != nil {
		return common.Address{}, err
	}

	var event simpleSwapDeployedEvent
	err = transaction.FindSingleEvent(&factoryABI, receipt, c.address, simpleSwapDeployedEventType, &event)
	if err != nil {
		return common.Address{}, fmt.Errorf("contract deployment failed: %w", err)
	}

	return event.ContractAddress, nil
}

// VerifyBytecode checks that the factory is valid.
func (c *factory) VerifyBytecode(ctx context.Context) (err error) {
	code, err := c.backend.CodeAt(ctx, c.address, nil)
	if err != nil {
		return err
	}

	if !bytes.Equal(code, currentDeployVersion) {
		return ErrInvalidFactory
	}

	return nil
}

func (c *factory) verifyChequebookAgainstFactory(ctx context.Context, factory, chequebook common.Address) (bool, error) {
	callData, err := factoryABI.Pack("deployedContracts", chequebook)
	if err != nil {
		return false, err
	}

	output, err := c.transactionService.Call(ctx, &transaction.TxRequest{
		To:   &factory,
		Data: callData,
	})
	if err != nil {
		return false, err
	}

	results, err := factoryABI.Unpack("deployedContracts", output)
	if err != nil {
		return false, err
	}

	if len(results) != 1 {
		return false, errDecodeABI
	}

	deployed, ok := abi.ConvertType(results[0], new(bool)).(*bool)
	if !ok || deployed == nil {
		return false, errDecodeABI
	}
	if !*deployed {
		return false, nil
	}
	return true, nil
}

// VerifyChequebook checks that the supplied chequebook has been deployed by a supported factory.
func (c *factory) VerifyChequebook(ctx context.Context, chequebook common.Address) error {
	deployed, err := c.verifyChequebookAgainstFactory(ctx, c.address, chequebook)
	if err != nil {
		return err
	}
	if deployed {
		return nil
	}

	return ErrNotDeployedByFactory
}

// ERC20Address returns the token for which this factory deploys chequebooks.
func (c *factory) ERC20Address(ctx context.Context) (common.Address, error) {
	callData, err := factoryABI.Pack("TokenAddress")
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

	results, err := factoryABI.Unpack("TokenAddress", output)
	if err != nil {
		return common.Address{}, err
	}

	if len(results) != 1 {
		return common.Address{}, errDecodeABI
	}

	erc20Address, ok := abi.ConvertType(results[0], new(common.Address)).(*common.Address)
	if !ok || erc20Address == nil {
		return common.Address{}, errDecodeABI
	}
	return *erc20Address, nil
}
