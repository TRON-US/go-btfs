// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap_test

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/TRON-US/go-btfs/settlement/swap/headers"
	"github.com/ethersphere/bee/pkg/p2p"
)

func TestParseSettlementResponseHeaders(t *testing.T) {
	headers := p2p.Headers{
		swap.ExchangeRateFieldName: []byte{10},
		//swap.DeductionFieldName:    []byte{20},
	}

	exchange, err := swap.ParseSettlementResponseHeaders(headers)
	if err != nil {
		t.Fatal(err)
	}

	if exchange.Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("Exchange rate mismatch, got %v, want %v", exchange, 10)
	}
}

func TestMakeSettlementHeaders(t *testing.T) {

	makeHeaders := swap.MakeSettlementHeaders(big.NewInt(906000))

	expectedHeaders := p2p.Headers{
		swap.ExchangeRateFieldName: []byte{13, 211, 16},
		//swap.DeductionFieldName:    []byte{20, 228},
	}

	if !reflect.DeepEqual(makeHeaders, expectedHeaders) {
		t.Fatalf("Made headers not as expected, got %+v, want %+v", makeHeaders, expectedHeaders)
	}
}

func TestParseExchangeHeader(t *testing.T) {
	toReadHeaders := p2p.Headers{
		swap.ExchangeRateFieldName: []byte{13, 211, 16},
	}

	parsedExchange, err := swap.ParseExchangeHeader(toReadHeaders)
	if err != nil {
		t.Fatal(err)
	}

	if parsedExchange.Cmp(big.NewInt(906000)) != 0 {
		t.Fatalf("Allowance mismatch, got %v, want %v", parsedExchange, big.NewInt(906000))
	}

}

