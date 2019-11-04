package ecies

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
)

type EciesMetadata struct {
	Iv             string `json:"iv"`
	EphemPublicKey string `json:"ephemPublicKey"`
	Mac            string `json:"mac"`
	Mode           string `json:"mode"`
}

const (
	B        = iota
	KB int64 = 1 << (10 * iota)
	MB
	GB
	TB
)

const (
	ENCRYPTION_MODE_1 = "AES256"
)

func Encrypt(pubkeyHex string, plainbytes []byte) (string, *EciesMetadata, error) {

	// make sure size of input bytes lt 2g
	if plainbytes == nil || int64(len(plainbytes)) >= 2*GB {
		return "", nil, errors.New("file exceeds the maximum file-size(lt 2G)")
	}

	// derive Share Secret
	priv, err := GenerateKey()
	if err != nil {
		return "", nil, err
	}
	ephemPublicKey := priv.PublicKey.Bytes(false)
	pk, err := NewPublicKeyFromHex(pubkeyHex)
	if err != nil {
		return "", nil, err
	}
	shareSecret, err := priv.ECDH(pk)

	// generate iv
	iv, err := randBytes(16)
	if err != nil {
		return "", nil, err
	}

	sha512hash := sha512.Sum512(shareSecret[1:])
	encryptionKey := sha512hash[0:32]
	macKey := sha512hash[32:]
	ciphertext, err := aesCBCEnc(encryptionKey, plainbytes, iv)
	if err != nil {
		return "", nil, err
	}

	dataToMac := make([]byte, 0)
	dataToMac = append(dataToMac, iv...)
	dataToMac = append(dataToMac, ephemPublicKey...)
	dataToMac = append(dataToMac, ciphertext...)
	mac := getHmacCode(macKey, dataToMac)
	metadata := &EciesMetadata{
		Iv:             hex.EncodeToString(iv),
		EphemPublicKey: hex.EncodeToString(ephemPublicKey),
		Mac:            mac,
		Mode:           ENCRYPTION_MODE_1,
	}
	return hex.EncodeToString(ciphertext), metadata, err
}

func randBytes(bits int) ([]byte, error) {
	key := make([]byte, bits)

	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func Decrypt(privateKeyHex string, cipherText string, t *EciesMetadata) (string, error) {
	switch t.Mode {
	case ENCRYPTION_MODE_1:
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

		cipherBytes, err := hex.DecodeString(cipherText)
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
	default:
		return "", errors.New("encryption mode not support")
	}
}

func getHmacCode(key []byte, data []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	s := hex.EncodeToString(h.Sum(nil))
	return s
}
