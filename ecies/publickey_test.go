package ecies

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPublicKey_Equals(t *testing.T) {
	privkey, err := GenerateKey()
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, privkey.PublicKey.Equals(privkey.PublicKey))
}
