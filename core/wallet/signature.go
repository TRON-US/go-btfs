package wallet

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"time"

	gbc_crypto "github.com/tron-us/go-btfs-common/crypto"
	exPb "github.com/tron-us/go-btfs-common/protos/exchange"
	ledgerPb "github.com/tron-us/go-btfs-common/protos/ledger"
	corePb "github.com/tron-us/go-btfs-common/protos/protocol/core"

	"github.com/golang/protobuf/proto"
)

type EcdsaSignature struct {
	R, S *big.Int
}

var (
	ErrTransactionParam   = errors.New("transaction is nil")
	ErrChannelStateParam  = errors.New("channelState is nil")
	ErrChannelCommitParam = errors.New("channelCommit is nil")
	ErrTypeParam          = errors.New("wrong type")
)

//Sign a Transaction, ChannelState, ChannelCommit in exchange proto or tron proto or ledger proto.
//parameter 'in' can be Transaction, ChannelState, ChannelCommit, return signature.
func Sign(in interface{}, key *ecdsa.PrivateKey) ([]byte, error) {
	switch in.(type) {
	case *exPb.TronTransaction:
		transaction := in.(*exPb.TronTransaction)
		if transaction == nil {
			return nil, ErrTransactionParam
		}

		if transaction.GetRawData().Timestamp == 0 {
			transaction.GetRawData().Timestamp = time.Now().UnixNano() / 1000000
		}

		rawData, err := proto.Marshal(transaction.GetRawData())
		if err != nil {
			return nil, err
		}
		return gbc_crypto.EcdsaSign(key, rawData)

	case *corePb.Transaction:
		transaction := in.(*corePb.Transaction)
		if transaction == nil {
			return nil, ErrTransactionParam
		}

		if transaction.GetRawData().Timestamp == 0 {
			transaction.GetRawData().Timestamp = time.Now().UnixNano() / 1000000
		}

		rawData, err := proto.Marshal(transaction.GetRawData())
		if err != nil {
			return nil, err
		}
		return gbc_crypto.EcdsaSign(key, rawData)

	case *ledgerPb.ChannelState:
		channelState := in.(*ledgerPb.ChannelState)
		if channelState == nil {
			return nil, ErrChannelStateParam
		}

		raw, err := proto.Marshal(channelState)
		if err != nil {
			return nil, err
		}
		return gbc_crypto.EcdsaSign(key, raw)

	case *ledgerPb.ChannelCommit:
		channelCommit := in.(*ledgerPb.ChannelCommit)
		if channelCommit == nil {
			return nil, ErrChannelCommitParam
		}

		raw, err := proto.Marshal(channelCommit)
		if err != nil {
			return nil, err
		}
		return gbc_crypto.EcdsaSign(key, raw)

	default:
		return nil, ErrTypeParam
	}
}
