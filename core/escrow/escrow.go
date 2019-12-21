package escrow

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
)

var log = logging.Logger("core/escrow")

func NewContract(configuration *config.Config, id string, n *core.IpfsNode, pid peer.ID,
	price int64) (*escrowpb.EscrowContract, error) {
	payerPubKey := n.PrivateKey.GetPublic()
	payerAddr, err := payerPubKey.Raw()
	if err != nil {
		return nil, err
	}
	hostPubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	hostAddr, err := hostPubKey.Raw()
	if err != nil {
		return nil, err
	}
	if len(configuration.Services.GuardPubKeys) == 0 {
		return nil, fmt.Errorf("No Services.GuardPubKeys are set in config")
	}
	authAddress, err := ConvertToAddress(configuration.Services.GuardPubKeys[0])
	if err != nil {
		return nil, err
	}
	return &escrowpb.EscrowContract{
		ContractId:       id,
		BuyerAddress:     payerAddr,
		SellerAddress:    hostAddr,
		AuthAddress:      authAddress,
		Amount:           price,
		CollateralAmount: 0,
		WithholdAmount:   0,
		TokenType:        escrowpb.TokenType_BTT,
		PayoutSchedule:   0,
		NumPayouts:       1,
	}, nil
}

func ConvertPubKeyFromString(pubKeyStr string) (ic.PubKey, error) {
	raw, err := base64.StdEncoding.DecodeString(pubKeyStr)
	if err != nil {
		return nil, err
	}
	return ic.UnmarshalPublicKey(raw)
}

func ConvertToAddress(pubKeyStr string) ([]byte, error) {
	pubKey, err := ConvertPubKeyFromString(pubKeyStr)
	if err != nil {
		return nil, err
	}
	return pubKey.Raw()
}

func NewSignedContract(contract *escrowpb.EscrowContract) *escrowpb.SignedEscrowContract {
	return &escrowpb.SignedEscrowContract{
		Contract: contract,
	}
}

func NewContractRequest(configuration *config.Config, signedContracts []*escrowpb.SignedEscrowContract, totalPrice int64) (*escrowpb.EscrowContractRequest, error) {
	// prepare channel commit
	buyerPrivKey, err := configuration.Identity.DecodePrivateKey("")
	if err != nil {
		return nil, err
	}
	buyerPubKey := buyerPrivKey.GetPublic()
	buyerAddr, err := buyerPubKey.Raw()
	if err != nil {
		return nil, err
	}
	if len(configuration.Services.EscrowPubKeys) == 0 {
		return nil, fmt.Errorf("No Services.EscrowPubKeys are set in config")
	}
	escrowAddress, err := ConvertToAddress(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return nil, err
	}
	chanCommit := &ledgerpb.ChannelCommit{
		Payer:     &ledgerpb.PublicKey{Key: buyerAddr},
		Recipient: &ledgerpb.PublicKey{Key: escrowAddress},
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
	request *escrowpb.EscrowContractRequest) (*escrowpb.SignedSubmitContractResult, error) {
	var (
		response *escrowpb.SignedSubmitContractResult
		err      error
	)
	err = grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
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
	return response, err
}

func verifyEscrowRes(configuration *config.Config, message proto.Message, sig []byte) error {
	escrowPubkey, err := ConvertPubKeyFromString(configuration.Services.EscrowPubKeys[0])
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
				log.Error(err)
				return err
			}
			err = verifyEscrowRes(configuration, res.Result, res.EscrowSignature)
			if err != nil {
				log.Error(err)
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
