package wallet

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"errors"
	"math/big"
	"time"

	exPb "github.com/tron-us/go-btfs-common/protos/exchange"
	ledgerPb "github.com/tron-us/go-btfs-common/protos/ledger"
	corePb "github.com/tron-us/go-btfs-common/protos/protocol/core"

	eth "github.com/ethereum/go-ethereum/crypto"
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
		return SignTron(rawData, key)

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
		return SignTron(rawData, key)

	case *ledgerPb.ChannelState:
		channelState := in.(*ledgerPb.ChannelState)
		if channelState == nil {
			return nil, ErrChannelStateParam
		}

		raw, err := proto.Marshal(channelState)
		if err != nil {
			return nil, err
		}
		return SignChannel(raw, key)

	case *ledgerPb.ChannelCommit:
		channelCommit := in.(*ledgerPb.ChannelCommit)
		if channelCommit == nil {
			return nil, ErrChannelCommitParam
		}

		raw, err := proto.Marshal(channelCommit)
		if err != nil {
			return nil, err
		}
		return SignChannel(raw, key)

	default:
		return nil, ErrTypeParam
	}
}

//Tron' Sign function, return signature and error.
func SignTron(rawData []byte, key *ecdsa.PrivateKey) ([]byte, error) {
	hash, err := Hash(rawData)
	if err != nil {
		return nil, err
	}

	signature, err := eth.Sign(hash, key)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

//Channel' sign function, return signature and error.
func SignChannel(raw []byte, key *ecdsa.PrivateKey) ([]byte, error) {
	hash, err := Hash(raw)
	if err != nil {
		return nil, err
	}

	signature, err := key.Sign(rand.Reader, hash, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

// Verify signature.
func Verify(publicKey, data, signature []byte) (bool, error) {
	pubKey, err := eth.UnmarshalPubkey(publicKey)
	if err != nil {
		return false, err
	}
	a := EcdsaSignature{}
	_, err = asn1.Unmarshal(signature, &a)
	if err != nil {
		return false, nil
	}
	return ecdsa.Verify(pubKey, data, a.R, a.S), nil
}
