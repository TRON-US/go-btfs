package ecies

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

func aesCBCEnc(key []byte, plaintext []byte, iv []byte) (ciphertext []byte, err error) {
	block, err := aes.NewCipher(key) //选择加密算法
	if err != nil {
		return nil, err
	}

	plaintext = PKCS5Padding(plaintext, block.BlockSize())

	ciphertext = make([]byte, aes.BlockSize+len(plaintext))

	blockModel := cipher.NewCBCEncrypter(block, iv)

	blockModel.CryptBlocks(ciphertext[aes.BlockSize:], plaintext)
	return ciphertext[aes.BlockSize:], nil
}

func aesCBCDec(key []byte, ciphertext []byte, iv []byte) (plaintext []byte, err error) {
	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// CBC mode always works in whole blocks.
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
	}

	blockModel := cipher.NewCBCDecrypter(block, iv)
	plaintext = ciphertext
	blockModel.CryptBlocks(plaintext, ciphertext)

	plaintext = PKCS5UnPadding(plaintext)
	return plaintext, nil
}

func PKCS5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	if padding < 0 {
		padding = 0
	}
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS5UnPadding(plaintext []byte) []byte {
	length := len(plaintext)
	if length <= 0 {
		return nil
	}

	unpadding := int(plaintext[length-1])
	if length-unpadding < 0 {
		return nil
	}

	return plaintext[:(length - unpadding)]
}
