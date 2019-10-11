package ecies

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type text struct {
	Iv             string `json:"iv"`
	EphemPublicKey string `json:"ephemPublicKey"`
	Ciphertext     string `json:"ciphertext"`
	Mac            string `json:"mac"`
}

const (
	B        = iota
	KB int64 = 1 << (10 * iota)
	MB
	GB
	TB
)

func Encrypt(pubkeyHex string, plainbytes []byte) (string, error) {

	// make sure size of input bytes lt 2g
	if plainbytes == nil || int64(len(plainbytes)) >= 2*GB {
		return "", errors.New("file exceeds the maximum file-size(lt 2G)")
	}

	// derive Share Secret
	priv, err := GenerateKey()
	if err != nil {
		return "", err
	}
	ephemPublicKey := priv.PublicKey.Bytes(false)
	pk, err := NewPublicKeyFromHex(pubkeyHex)
	if err != nil {
		return "", err
	}
	shareSecret, err := priv.ECDH(pk)

	// generate iv
	iv, err := randBytes(16)
	if err != nil {
		return "", err
	}

	sha512hash := sha512.Sum512(shareSecret[1:])
	encryptionKey := sha512hash[0:32]
	macKey := sha512hash[32:]
	ciphertext, err := aesCBCEnc(encryptionKey, plainbytes, iv)
	if err != nil {
		return "", err
	}

	dataToMac := make([]byte, 0)
	dataToMac = append(dataToMac, iv...)
	dataToMac = append(dataToMac, ephemPublicKey...)
	dataToMac = append(dataToMac, ciphertext...)
	mac := getHmacCode(macKey, dataToMac)
	b, err := json.Marshal(&text{
		Iv:             hex.EncodeToString(iv),
		EphemPublicKey: hex.EncodeToString(ephemPublicKey),
		Ciphertext:     hex.EncodeToString(ciphertext),
		Mac:            mac,
	})
	return string(b), err
}

func randBytes(bits int) ([]byte, error) {
	key := make([]byte, bits)

	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func Decrypt(privateKeyHex string, cipherbytes []byte) (string, error) {
	t := &text{}
	err := json.Unmarshal([]byte(cipherbytes), t)
	if err != nil {
		return "", err
	}

	privKey, err := NewPrivateKeyFromHex(privateKeyHex)
	if err != nil {
		return "", err
	}

	epkHex := t.EphemPublicKey
	epk, err := NewPublicKeyFromHex(epkHex)
	if err != nil {
		return "", err
	}

	// derive Share Secret
	ecdh, err := privKey.ECDH(epk)
	if err != nil {
		return "", err
	}

	cipherBytes, err := hex.DecodeString(t.Ciphertext)
	if err != nil {
		return "", err
	}

	ivBytes, err := hex.DecodeString(t.Iv)
	if err != nil {
		return "", err
	}

	ecdhHash := sha512.Sum512(ecdh[1:])
	encryptionKey := ecdhHash[:32]
	plaintext, e := aesCBCDec(encryptionKey, cipherBytes, ivBytes)
	if e != nil {
		return "", err
	}

	return string(plaintext), nil
}

func getHmacCode(key []byte, data []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	s := hex.EncodeToString(h.Sum(nil))
	return s
}
