package storage

import (
	"github.com/gogo/protobuf/proto"
	"github.com/tron-us/go-btfs-common/protos/escrow"
	"testing"
)

func TestGenContractsFromChunk(t *testing.T) {
	contract1 := &escrow.SignedEscrowContract{}
	contract1Bytes, err := proto.Marshal(contract1)
	if err != nil {
		t.Fatal(err)
	}
	contract2 := &escrow.SignedEscrowContract{}
	contract2Bytes, err := proto.Marshal(contract2)
	if err != nil {
		t.Fatal(err)
	}
	chunk := make(map[string]*Shard)
	chunk["1"] = &Shard{TotalPay: 1, SignedEscrowContract: contract1Bytes}
	chunk["2"] = &Shard{TotalPay: 10, SignedEscrowContract: contract2Bytes}

	_, totalPrice, err := PrepareContractFromShard(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if totalPrice != 11 {
		t.Fatal("price doesn't match")
	}
}
