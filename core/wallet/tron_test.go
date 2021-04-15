package wallet

import (
	"github.com/tron-us/go-btfs-common/crypto"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHexAddress(t *testing.T) {
	keys, err := crypto.FromPrivateKey("CAISILOZbORDZlczUlp5jdonb5y5SMZgaZy6OWp58SkS8jS8")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "41bc8e7f3e2bb11310b75d6b0b6e8537d069cdb72e", keys.HexAddress)
}

var body = `{
  "ret": [
    {
      "contractRet": "SUCCESS"
    }
  ]
}`

func TestTxResult(t *testing.T) {
	result, err := parseTxResult(body)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "SUCCESS", result)
}
