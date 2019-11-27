package storage

import (
	"github.com/TRON-US/go-btfs/core/escrow"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
)
func PrepareContractFromChunk(chunkInfo map[string]*Chunk) ([]*escrowpb.SignedEscrowContract, int64, error) {
	var signedContracts []*escrowpb.SignedEscrowContract
	var totalPrice int64
	for _, chunk := range chunkInfo {
		sc, err := escrow.UnmarshalEscrowContract(chunk.SignedContract)
		if err != nil {
			return nil, 0, err
		}
		signedContracts = append(signedContracts, sc)
		totalPrice += chunk.Price
	}
	return signedContracts, totalPrice, nil
}
