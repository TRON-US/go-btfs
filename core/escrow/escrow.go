package escrow

import (
	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	//"github.com/libp2p/go-libp2p-core/peer"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
)

func NewContract(id string, payerID string, recverID string, price int64) *escrowpb.EscrowContract {
	payerAddr := []byte(payerID)
	recverAddr := []byte(recverID)
	return &escrowpb.EscrowContract{
		ContractId: []byte(id),
		BuyerAddress: payerAddr,
		SellerAddress: recverAddr,
		Amount: price,
	}
}


