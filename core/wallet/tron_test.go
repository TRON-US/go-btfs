package wallet

import (
	"context"
	"testing"

	coremock "github.com/TRON-US/go-btfs/core/mock"

	"github.com/tron-us/go-btfs-common/crypto"

	"github.com/stretchr/testify/assert"
)

func TestGetHexAddress(t *testing.T) {
	privKey, err := crypto.ToPrivKey("CAISIBMlMbrGwtWKz894jM5LGyEMaT5O/FkklTGoF7mQViJj")
	if err != nil {
		t.Fatal(err)
	}
	hexAddr, err := getHexAddress(privKey)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "416e2ffc26bdf48b1983ccc9ec2521867f98667760", hexAddr)
}

func TestTransferBTT(t *testing.T) {
	node, err := coremock.NewMockNode()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := node.Repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Services.SolidityDomain = "grpc.trongrid.io:50052"
	cfg.Services.FullnodeDomain = "grpc.trongrid.io:50051"
	cfg.Services.EscrowDomain = "https://escrow-staging.btfs.io"
	// don't have dev token now, will delete this line later
	TokenIdDev = "1002000"
	privKey, err := crypto.ToPrivKey("CAISID6wbQHX3IHneklC2rOMyH250Wijf5W7phqPHZ1P5fFC")
	if err != nil {
		t.Fatal(err)
	}
	ret, err := TransferBTT(context.Background(), node, cfg, privKey, "41A2468828043EE28B2C2556E063DAD6A314372E2F",
		"416E2FFC26BDF48B1983CCC9EC2521867F98667760", 1)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, ret.Result)
	assert.Equal(t, "SUCCESS", ret.Code)

	ret2, err := TransferBTT(context.Background(), node, cfg, privKey, "TQmExmuk8KyUByiS36c5sAfke8PghojhDo",
		"TL1ppDNyESQ5msZ1BBZQEFg6ksdbcyWDwj", 1)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, ret.Result)
	assert.Equal(t, "SUCCESS", ret2.Code)
}
