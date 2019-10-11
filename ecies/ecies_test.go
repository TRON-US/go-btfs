package ecies

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

const msg = "hello,world\n"

const pkHex = "048903aca62f342426d0595597bcd4b03519723c7292f231a5d40c02" +
	"d43c0f69309de7166e91d2fe7a479bcad8a4f611175b8a593178683518fcab05b8bf74dc09"

const privHex = "0abfa58854e585d9bb04a1ffad0f5ac507ac042e7aa69abbcf18f3103a936f6f"

func TestEncrypt(t *testing.T) {

	twogBytes := make([]byte, 2*GB)
	twogBytes[2*GB-1] = 1
	_, err := Encrypt(pkHex, twogBytes)
	assert.Errorf(t, err, "")

	s, err := Encrypt(pkHex, []byte(msg))
	assert.NoError(t, err)

	decrypted, err := Decrypt(privHex, []byte(s))
	assert.NoError(t, err)
	assert.Equal(t, msg, decrypted)
}
