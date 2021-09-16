// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swap

import (
	"errors"
	"math/big"

	"github.com/ethersphere/bee/pkg/p2p"
)

const (
	exchangeRateFieldName = "exchange"
)

var (
	// ErrFieldLength denotes p2p.Header having malformed field length in bytes
	ErrFieldLength = errors.New("field length error")
	// ErrNoExchangeHeader denotes p2p.Header lacking specified field
	ErrNoExchangeHeader = errors.New("no exchange header")
)

func MakeSettlementHeaders(exchangeRate *big.Int) p2p.Headers {

	return p2p.Headers{
		exchangeRateFieldName: exchangeRate.Bytes(),
	}
}

func ParseSettlementResponseHeaders(receivedHeaders p2p.Headers) (exchange *big.Int, err error) {

	exchangeRate, err := ParseExchangeHeader(receivedHeaders)
	if err != nil {
		return nil, err
	}

	return exchangeRate, nil
}

func ParseExchangeHeader(receivedHeaders p2p.Headers) (*big.Int, error) {
	if receivedHeaders[exchangeRateFieldName] == nil {
		return nil, ErrNoExchangeHeader
	}

	exchange := new(big.Int).SetBytes(receivedHeaders[exchangeRateFieldName])
	return exchange, nil
}
