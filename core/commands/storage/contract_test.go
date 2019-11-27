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
	chunk := make(map[string]*Chunk)
	chunk["1"] = &Chunk{Price: 1, SignedContract: contract1Bytes}
	chunk["2"] = &Chunk{Price: 10, SignedContract: contract2Bytes}

	_, totalPrice, err := PrepareContractFromChunk(chunk)
	if totalPrice != 11 {
		t.Fatal("price doesn't match")
	}
}
