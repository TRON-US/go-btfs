package escrow

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	"github.com/tron-us/go-btfs-common/protos/ledger"
	"google.golang.org/grpc"
)

func NewContract(configuration *config.Config, id string, payerID string, recverID string, price int64) *escrowpb.EscrowContract {
	payerAddr := []byte(payerID)
	recverAddr := []byte(recverID)
	// TODO: Make escrow GuardAddress configurable
	//authAddress := configuration.Services.GuardPubKeys[0]
	authAddress := "guardAddress"
	return &escrowpb.EscrowContract{
		ContractId:       id,
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
		Contract: contract,
	}
}

func NewContractRequest(configuration *config.Config, chunkInfo map[string]*storage.Chunk) (*escrowpb.EscrowContractRequest, error) {
	var signedContracts []*escrowpb.SignedEscrowContract
	var totalPrice int64
	for _, chunk := range chunkInfo {
		sc, err := UnmarshalEscrowContract(chunk.SignedContract)
		if err != nil {
			return nil, err
		}
		signedContracts = append(signedContracts, sc)
		totalPrice += chunk.Price
	}
	// prepare channel commit
	pid := configuration.Identity.PeerID
	buyerPrivKey, err := configuration.Identity.DecodePrivateKey("")
	if err != nil {
		return nil, err
	}
	// TODO: Make escrow Address configurable
	//escrowAddress := configuration.Services.EscrowPubKeys[0]
	escrowAddress := "escrowAddress"
	chanCommit := &ledger.ChannelCommit{
		Payer:     &ledger.PublicKey{Key: []byte(pid)},
		Recipient: &ledger.PublicKey{Key: []byte(escrowAddress)},
		Amount:    totalPrice, // total amount in the contract request
		PayerId:   time.Now().UnixNano(),
	}
	buyerChanSig, err := crypto.Sign(buyerPrivKey, chanCommit)
	if err != nil {
		return nil, err
	}

	signedChanCommit := &ledger.SignedChannelCommit{
		Channel:   chanCommit,
		Signature: buyerChanSig,
	}
	return &escrowpb.EscrowContractRequest{
		Contract:     signedContracts,
		BuyerChannel: signedChanCommit,
	}, nil
}

func SubmitContractToEscrow(configuration *config.Config, request *escrowpb.EscrowContractRequest) error {
	var conn *grpc.ClientConn
	// TODO: Make escrow IP hidden in config too, now for testing purpose leave it here
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

// TODO: move this to go-btfs-common also, and delete it here
func convertPubKey(pubStr string) (ic.PubKey, error) {
	raw, err := base64.StdEncoding.DecodeString(pubStr)
	if err != nil {
		return nil, err
	}
	return crypto.ToPubKey(raw)
}

func SignContractAndMarshal(contract *escrowpb.EscrowContract, signedContract *escrowpb.SignedEscrowContract,
	privKey ic.PrivKey, isPayer bool) ([]byte, error) {
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
