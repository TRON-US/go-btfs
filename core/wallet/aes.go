package wallet

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
)

var iv = []byte{0x02, 0x00, 0x01, 0x06, 0x00, 0x08, 0x01, 0x04, 0x02, 0x00, 0x01, 0x06, 0x00, 0x08, 0x01, 0x04}

func EncryptWithAES(key, message string) (string, error) {
	hash := md5.New()
	hash.Write([]byte(key))
	keyData := hash.Sum(nil)

	block, err := aes.NewCipher(keyData)
	if err != nil {
		return "", err
	}

	enc := cipher.NewCBCEncrypter(block, iv)
	content, err := PKCS5Padding([]byte(message), block.BlockSize())
	if err != nil {
		return "", err
	}
	crypted := make([]byte, len(content))
	enc.CryptBlocks(crypted, content)
	return base64.StdEncoding.EncodeToString(crypted), nil
}

func DecryptWithAES(key, message string) (string, error) {
	hash := md5.New()
	hash.Write([]byte(key))
	keyData := hash.Sum(nil)

	block, err := aes.NewCipher(keyData)
	if err != nil {
		return "", err
	}

	messageData, _ := base64.StdEncoding.DecodeString(message)
	dec := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(messageData))
	dec.CryptBlocks(decrypted, messageData)
	unpadding, err := PKCS5Unpadding(decrypted)
	if err != nil {
		return "", err
	}
	return string(unpadding), nil
}

func PKCS5Padding(ciphertext []byte, blockSize int) ([]byte, error) {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...), nil
}

func PKCS5Unpadding(encrypt []byte) ([]byte, error) {
	padding := encrypt[len(encrypt)-1]
	return encrypt[:len(encrypt)-int(padding)], nil
}
