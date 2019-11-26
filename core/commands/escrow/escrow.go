package escrow

import (
	//"github.com/libp2p/go-libp2p-core/peer"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"

	"github.com/TRON-US/go-btfs/core"
	"github.com/gogo/protobuf/proto"
)

func NewContract(node *core.IpfsNode, id string, payerID string, recverID string, price int64) (*escrowpb.EscrowContract, error) {
	payerAddr := []byte(payerID)
	recverAddr := []byte(recverID)
	configuration, err := node.Repo.Config()
	if err != nil {
		return nil, err
	}
	authAddress := configuration.Services.GuardPubKeys[0]
	return &escrowpb.EscrowContract{
		ContractId:       []byte(id),
		BuyerAddress:     payerAddr,   //buyerAddress,
		SellerAddress:    recverAddr, //sellerAddress,
		AuthAddress:      []byte(authAddress),   //sellerAddress,
		Amount:           price,
		CollateralAmount: 0,
		WithholdAmount:   0,
		TokenType:        escrowpb.TokenType_BTT,
		PayoutSchedule:   0,
		NumPayouts:       1,
	}, nil
}

func SignContract(contract *escrowpb.EscrowContract, )  {

}

func UnmarshalEscrowContract(contractStr string) (*escrowpb.EscrowContract, error) {
	contract := &escrowpb.EscrowContract{}
	err := proto.Unmarshal([]byte(contractStr), contract)
	if err != nil {
		return nil, err
	}
	return contract, nil
}


