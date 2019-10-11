package ecies

import (
	"crypto/subtle"
	"github.com/stretchr/testify/assert"
	"testing"
)

const privkeyBase = "f0e5b2c3ba4df3fdb3ecea30d0e60c4e4a31d1ba928f51783ae18bbd3cada572"

var privkeys = []string{
	"73d808dc0da21075f252937986a78c84f1c2abbd4649d597f9473e14209efc90",
	"19848cb43883723309ace215ba1cbe80907f6e3faeaab32c68034d5f521a57de",
}

func TestPrivateKey_Hex(t *testing.T) {
	privkey, err := GenerateKey()
	if !assert.NoError(t, err) {
		return
	}

	privkey.Hex()
}

func TestPrivateKey_Equals(t *testing.T) {
	privkey, err := GenerateKey()
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, privkey.Equals(privkey))
}

func TestPrivateKey_UnsafeECDH(t *testing.T) {
	privkey1, err := NewPrivateKeyFromHex(privkeyBase)
	if !assert.NoError(t, err) {
		return
	}
	for _, key := range privkeys {
		privkey2, err := NewPrivateKeyFromHex(key)
		if !assert.NoError(t, err) {
			return
		}
		ss1, err := privkey1.ECDH(privkey2.PublicKey)
		if !assert.NoError(t, err) {
			return
		}
		ss2, err := privkey2.ECDH(privkey1.PublicKey)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, subtle.ConstantTimeCompare(ss1, ss2), 1)
	}
}
