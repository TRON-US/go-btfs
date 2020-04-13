package upload

import (
	"encoding/base64"

	"github.com/TRON-US/go-btfs/core/escrow"

	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

func convertPubKeyFromString(pubKeyStr string) (ic.PubKey, error) {
	raw, err := base64.StdEncoding.DecodeString(pubKeyStr)
	if err != nil {
		return nil, err
	}
	return ic.UnmarshalPublicKey(raw)
}

func convertToPubKey(pubKeyStr string) (ic.PubKey, error) {
	pubKey, err := convertPubKeyFromString(pubKeyStr)
	if err != nil {
		return nil, err
	}
	return pubKey, nil
}

func pidFromString(key string) (peer.ID, error) {
	pubKey, err := escrow.ConvertPubKeyFromString(key)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pubKey)
}
