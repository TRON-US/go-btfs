package escrow

import (
	"context"
	"fmt"
	"time"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
)

func NewContract(configuration *config.Config, id string, payerID string, recverID string, price int64) *escrowpb.EscrowContract {
	payerAddr := []byte(payerID)
	recverAddr := []byte(recverID)
	authAddress := configuration.Services.GuardPubKeys[0]
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

func NewContractRequest(configuration *config.Config, signedContracts []*escrowpb.SignedEscrowContract, totalPrice int64) (*escrowpb.EscrowContractRequest, error) {
	// prepare channel commit
	pid := configuration.Identity.PeerID
	buyerPrivKey, err := configuration.Identity.DecodePrivateKey("")
	if err != nil {
		return nil, err
	}
	escrowAddress := configuration.Services.EscrowPubKeys[0]
	chanCommit := &ledgerpb.ChannelCommit{
		Payer:     &ledgerpb.PublicKey{Key: []byte(pid)},
		Recipient: &ledgerpb.PublicKey{Key: []byte(escrowAddress)},
		Amount:    totalPrice, // total amount in the contract request
		PayerId:   time.Now().UnixNano(),
	}
	buyerChanSig, err := crypto.Sign(buyerPrivKey, chanCommit)
	if err != nil {
		return nil, err
	}

	signedChanCommit := &ledgerpb.SignedChannelCommit{
		Channel:   chanCommit,
		Signature: buyerChanSig,
	}
	return &escrowpb.EscrowContractRequest{
		Contract:     signedContracts,
		BuyerChannel: signedChanCommit,
	}, nil
}

func SubmitContractToEscrow(ctx context.Context, configuration *config.Config,
	request *escrowpb.EscrowContractRequest) (
	response *escrowpb.SignedSubmitContractResult, err error) {
	grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			response, err = client.SubmitContracts(ctx, request)
			if err != nil {
				return err
			}
			if response == nil {
				return fmt.Errorf("escrow reponse is nil")
			}
			// verify
			err = verifyEscrowRes(configuration, response.Result, response.EscrowSignature)
			if err != nil {
				return fmt.Errorf("verify escrow failed %v", err)
			}
			return nil
		})
	return
}

func verifyEscrowRes(configuration *config.Config, message proto.Message, sig []byte) error {
	escrowPubkey, err := crypto.ToPubKey(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return err
	}
	ok, err := crypto.Verify(escrowPubkey, message, sig)
	if err != nil || !ok {
		return fmt.Errorf("verify escrow failed %v", err)
	}
	return nil
}

func NewPayinRequest(result *escrowpb.SignedSubmitContractResult, payerPubKey ic.PubKey, payerPrivKey ic.PrivKey) (*escrowpb.SignedPayinRequest, error) {
	chanState := result.Result.BuyerChannelState
	sig, err := crypto.Sign(payerPrivKey, chanState.Channel)
	if err != nil {
		return nil, err
	}
	chanState.FromSignature = sig
	payerAddr, err := payerPubKey.Raw()
	if err != nil {
		return nil, err
	}
	payinReq := &escrowpb.PayinRequest{
		PayinId:           result.Result.PayinId,
		BuyerAddress:      payerAddr,
		BuyerChannelState: chanState,
	}
	payinSig, err := crypto.Sign(payerPrivKey, payinReq)
	if err != nil {
		return nil, err
	}
	return &escrowpb.SignedPayinRequest{
		Request:        payinReq,
		BuyerSignature: payinSig,
	}, nil
}

func PayInToEscrow(ctx context.Context, configuration *config.Config, signedPayinReq *escrowpb.SignedPayinRequest) (*escrowpb.SignedPayinResult, error) {
	var signedPayinRes *escrowpb.SignedPayinResult
	err := grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.PayIn(ctx, signedPayinReq)
			if err != nil {
				return err
			}
			err = verifyEscrowRes(configuration, res.Result, res.EscrowSignature)
			if err != nil {
				return err
			}
			signedPayinRes = res
			return nil
		})
	if err != nil {
		return nil, err
	}
	return signedPayinRes, nil
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
