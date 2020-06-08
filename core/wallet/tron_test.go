package wallet

import (
	"context"
	"fmt"
	"testing"

	coremock "github.com/TRON-US/go-btfs/core/mock"

	"github.com/tron-us/go-btfs-common/crypto"

	"github.com/stretchr/testify/assert"
)

func TestGetHexAddress(t *testing.T) {
	privKey, err := crypto.ToPrivKey("CAISIBMlMbrGwtWKz894jM5LGyEMaT5O/FkklTGoF7mQViJj")
	if err != nil {
		t.Error(err)
	}
	hexAddr, err := getHexAddress(privKey)
	if err != nil {
		t.Error(err)
	}
	fmt.Println("hexAddr", hexAddr)
}

func TestTransferBTT(t *testing.T) {
	//ignore this test
	t.SkipNow()
	node, err := coremock.NewMockNode()
	if err != nil {
		t.Error(err)
	}
	cfg, err := node.Repo.Config()
	if err != nil {
		t.Error(err)
	}
	cfg.Services.SolidityDomain = "grpc.trongrid.io:50051"
	privKey, err := crypto.ToPrivKey("CAISIBMlMbrGwtWKz894jM5LGyEMaT5O/FkklTGoF7mQViJj")
	if err != nil {
		t.Error(err)
	}
	ret, err := TransferBTT(context.Background(), node, cfg, privKey, "416E2FFC26BDF48B1983CCC9EC2521867F98667760",
		"4129B733EB5F3F81228EB9D97EFEF7541851514AEE", 1)
	if err != nil {
		t.Error(err)
	}
	assert.True(t, ret.Result)
	assert.Equal(t, "SUCCESS", ret.Code)

	ret2, err := TransferBTT(context.Background(), node, cfg, privKey, "TL1ppDNyESQ5msZ1BBZQEFg6ksdbcyWDwj",
		"TDmnB5WC1yDD7yDMUXEo6SS9HE9zdnhodU", 1)
	if err != nil {
		t.Error(err)
	}
	assert.True(t, ret.Result)
	assert.Equal(t, "SUCCESS", ret2.Code)
}
