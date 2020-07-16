package upload

import (
	"errors"
	"testing"

	coremock "github.com/TRON-US/go-btfs/core/mock"

	config "github.com/TRON-US/go-btfs-config"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
)

func TestAcceptContract(t *testing.T) {
	node, err := coremock.NewMockNode()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := node.Repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.UI = config.UI{
		Host: config.HostUI{
			ContractManager: &config.ContractManager{
				LowWater:  100,
				HighWater: 300,
				Threshold: 1000,
			},
		},
	}
	hm := NewHostManster(cfg)
	Count = func(ds datastore.Datastore, peerId string, status guardpb.Contract_ContractState) (int, error) {
		switch peerId {
		case "id1":
			return 50, nil
		case "id2":
			return 200, nil
		case "id3":
			return 400, nil
		}
		return -1, errors.New("np")
	}
	tests := []struct {
		peerId    string
		shardSize int64
		want      bool
	}{
		{peerId: "id1", shardSize: 500, want: true},   // <low, always accept new contract
		{peerId: "id1", shardSize: 2000, want: true},  // <low, always accept new contract
		{peerId: "id2", shardSize: 500, want: true},   // >low && <high, accept new contract witch < threshold
		{peerId: "id2", shardSize: 2000, want: false}, // >low && <high, accept new contract witch < threshold
		{peerId: "id3", shardSize: 500, want: false},  // >high, doesn't accept any new contract
		{peerId: "idx", shardSize: 500, want: true},   // accept new contract when func_count return error
	}
	for _, tc := range tests {
		accept, err := hm.AcceptContract(node.Repo.Datastore(), tc.peerId, tc.shardSize)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, tc.want, accept)
	}
}
