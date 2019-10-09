package ecies

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

const msg = "EC69F678-F6A9-4A4D-93B8-07D0DE19C521"
//const msg = "hello,world\n"

const pkHex = "048903aca62f342426d0595597bcd4b03519723c7292f231a5d40c02" +
	"d43c0f69309de7166e91d2fe7a479bcad8a4f611175b8a593178683518fcab05b8bf74dc09"

//const privHex = "0abfa58854e585d9bb04a1ffad0f5ac507ac042e7aa69abbcf18f3103a936f6f"
const privHex = "080212201b69f9edb6f20ab64814cffdb7d76e7531f8e41664d943b849edabb78e3e5041"

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
