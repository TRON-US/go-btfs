package escrow

import (
	"context"
	"encoding/base64"
	"fmt"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/storage"

	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
"google.golang.org/grpc"
)

func NewContract(configuration *config.Config, id string, payerID string, recverID string, price int64) *escrowpb.EscrowContract {
	payerAddr := []byte(payerID)
	recverAddr := []byte(recverID)
	authAddress := configuration.Services.GuardPubKeys[0]
	return &escrowpb.EscrowContract{
		ContractId:       []byte(id),
		BuyerAddress:     payerAddr,
		SellerAddress:    recverAddr,
		AuthAddress:      []byte(authAddress),
		Amount:           price,
		CollateralAmount: 0,
		WithholdAmount:   0,
		TokenType:        escrowpb.TokenType_BTT,
		PayoutSchedule:   0,
		NumPayouts:       1,
	}
}

func NewSignedContract(contract *escrowpb.EscrowContract) *escrowpb.SignedEscrowContract {
	return &escrowpb.SignedEscrowContract{
		Contract:        contract,
	}
}

func NewContractRequest(chunkInfo map[string]*storage.Chunk) *escrowpb.EscrowContractRequest {
	var signedContracts []*escrowpb.SignedEscrowContract
	for _, chunk := range chunkInfo {
		sc, err := UnmarshalEscrowContract(chunk.SignedContract)
		if err != nil {
			return nil
		}
		signedContracts = append(signedContracts, sc)
	}
	return &escrowpb.EscrowContractRequest{
		Contract: signedContracts,
	}
}

func SubmitContractToEscrow(configuration *config.Config, request *escrowpb.EscrowContractRequest) error {
	var conn *grpc.ClientConn
	conn, err := grpc.Dial("52.15.101.94:50051", grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()
	client := escrowpb.NewEscrowServiceClient(conn)
	response, err := client.SubmitContracts(context.Background(), request)
	if err != nil {
		return err
	}
	if response == nil {
		return fmt.Errorf("escrow reponse is nil")
	}
	escrowPubkey, err := convertPubKey(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return err
	}
	ok, err := crypto.Verify(escrowPubkey, response.Result, response.EscrowSignature)
	if err != nil || !ok {
		return fmt.Errorf("verify escrow failed %v", err)
	}
	return nil
}

func convertPubKey(pubStr string) (ic.PubKey, error) {
	raw, err := base64.StdEncoding.DecodeString(pubStr)
	if err != nil {
		return nil, err
	}
	return ic.UnmarshalSecp256k1PublicKey(raw)
}

func SignContractAndMarshal(contract *escrowpb.EscrowContract, signedContract *escrowpb.SignedEscrowContract,
	privKey ic.PrivKey, isPayer bool) ([]byte, error){
	sig, err := crypto.Sign(privKey, contract)
	if err != nil {
		return nil, err
	}
	if signedContract == nil {
		signedContract = NewSignedContract(contract)
	}
	if isPayer {
		signedContract.BuyerSignature = sig
	} else {
		signedContract.SellerSignature = sig
	}
	signedBytes, err := proto.Marshal(signedContract)
	if err != nil {
		return nil, err
	}
	return signedBytes, nil
}

func UnmarshalEscrowContract(marshaledBody []byte) (*escrowpb.SignedEscrowContract, error) {
	signedContract := &escrowpb.SignedEscrowContract{}
	err := proto.Unmarshal(marshaledBody, signedContract)
	if err != nil {
		return nil, err
	}
	return signedContract, nil
}
