package wallet

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/btcsuite/btcutil/base58"
	eth "github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrDecodeLength = errors.New("base58 decode length error")
	ErrDecodeCheck  = errors.New("base58 check failed")
	ErrEncodeLength = errors.New("base58 encode length error")
)

const (
	AddressLength = 21
	AddressPrefix = "41"
)

type Address [AddressLength]byte

func PublicKeyToAddress(p ecdsa.PublicKey) (Address, error) {
	addr := eth.PubkeyToAddress(p)

	addressTron := make([]byte, AddressLength)

	addressPrefix, err := FromHex(AddressPrefix)
	if err != nil {
		return Address{}, err
	}

	addressTron = append(addressTron, addressPrefix...)
	addressTron = append(addressTron, addr.Bytes()...)

	return BytesToAddress(addressTron), nil
}

// Decode hex string as bytes
func FromHex(input string) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("empty hex string")
	}

	return hex.DecodeString(input[:])
}

func (a *Address) Bytes() []byte {
	return a[:]
}

// Convert byte to address.
func (a *Address) SetBytes(b []byte) {
	if len(b) > len(a) {
		b = b[len(b)-AddressLength:]
	}
	copy(a[AddressLength-len(b):], b)
}

func BytesToAddress(b []byte) Address {
	var a Address
	a.SetBytes(b)
	return a
}

// Decode by base58 and check.
func Decode58Check(input string) ([]byte, error) {
	decodeCheck := base58.Decode(input)
	if len(decodeCheck) <= 4 {
		return nil, ErrDecodeLength
	}
	decodeData := decodeCheck[:len(decodeCheck)-4]
	hash0, err := Hash(decodeData)
	if err != nil {
		return nil, err
	}
	hash1, err := Hash(hash0)
	if hash1 == nil {
		return nil, err
	}
	if hash1[0] == decodeCheck[len(decodeData)] && hash1[1] == decodeCheck[len(decodeData)+1] &&
		hash1[2] == decodeCheck[len(decodeData)+2] && hash1[3] == decodeCheck[len(decodeData)+3] {
		return decodeData, nil
	}
	return nil, ErrDecodeCheck
}

// Encode by base58 and check.
func Encode58Check(input []byte) (string, error) {
	h0, err := Hash(input)
	if err != nil {
		return "", err
	}
	h1, err := Hash(h0)
	if err != nil {
		return "", err
	}
	if len(h1) < 4 {
		return "", ErrEncodeLength
	}
	inputCheck := append(input, h1[:4]...)

	return base58.Encode(inputCheck), nil
}

//Package goLang sha256 hash algorithm.
func Hash(s []byte) ([]byte, error) {
	h := sha256.New()
	_, err := h.Write(s)
	if err != nil {
		return nil, err
	}
	bs := h.Sum(nil)
	return bs, nil
}
