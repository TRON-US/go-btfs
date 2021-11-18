// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priceoracle_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/TRON-US/go-btfs/settlement/swap/priceoracle"
	"github.com/TRON-US/go-btfs/transaction"
	transactionmock "github.com/TRON-US/go-btfs/transaction/mock"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/go-price-oracle-abi/priceoracleabi"
)

var (
	priceOracleABI = transaction.ParseABIUnchecked(priceoracleabi.PriceOracleABIv0_1_0)
)

func TestExchangeGetPrice(t *testing.T) {
	priceOracleAddress := common.HexToAddress("0xabcd")

	expectedPrice := big.NewInt(100)
	expectedDeduce := big.NewInt(200)

	result := make([]byte, 64)
	expectedPrice.FillBytes(result[0:32])
	expectedDeduce.FillBytes(result[32:64])

	ex := priceoracle.New(
		priceOracleAddress,
		transactionmock.New(
			transactionmock.WithABICall(
				&priceOracleABI,
				priceOracleAddress,
				result,
				"getPrice",
			),
		),
		1,
	)

	price, err := ex.GetPrice(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if expectedPrice.Cmp(price) != 0 {
		t.Fatalf("got wrong price. wanted %d, got %d", expectedPrice, price)
	}
}
