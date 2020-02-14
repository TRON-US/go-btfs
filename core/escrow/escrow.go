package escrow

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/TRON-US/go-btfs/core"
	"os"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("core/escrow")
const DEBUG = false
func Dump(data []byte) {
	stdoutDumper := hex.Dumper(os.Stdout)
	defer stdoutDumper.Close()
	stdoutDumper.Write(data)
}

func NewContract(configuration *config.Config, id string, n *core.IpfsNode, hostPid peer.ID,
	price int64, customizedSchedule bool, period int, offSignPid peer.ID) (*escrowpb.EscrowContract, error) {
	var payerPubKey	ic.PubKey
	var err error
	if offSignPid == "" {
		payerPubKey = n.PrivateKey.GetPublic()
	} else {
		payerPubKey, err = offSignPid.ExtractPublicKey()
		if err != nil {
			return nil, err
		}
	}
	hostPubKey, err := hostPid.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	if len(configuration.Services.GuardPubKeys) == 0 {
		return nil, fmt.Errorf("No Services.GuardPubKeys are set in config")
	}
	authPubKey, err := ConvertToPubKey(configuration.Services.GuardPubKeys[0])
	if err != nil {
		return nil, err
	}
	ps := escrowpb.Schedule_MONTHLY
	p := 0
	if customizedSchedule {
		ps = escrowpb.Schedule_CUSTOMIZED
		p = period
	}
	if DEBUG {
		fmt.Println("==============================")
		Dump([]byte(id))
		b, _ := payerPubKey.Bytes()
		Dump(b)
		b2, _ := hostPubKey.Bytes()
		Dump(b2)
		b3, _ := authPubKey.Bytes()
		Dump(b3)
		fmt.Println("============================== ===========================")
	}


	return ledger.NewEscrowContract(id, payerPubKey, hostPubKey, authPubKey, price, ps, int32(p))
}

func ConvertPubKeyFromString(pubKeyStr string) (ic.PubKey, error) {
	raw, err := base64.StdEncoding.DecodeString(pubKeyStr)
	if err != nil {
		return nil, err
	}
	return ic.UnmarshalPublicKey(raw)
}

func ConvertToPubKey(pubKeyStr string) (ic.PubKey, error) {
	pubKey, err := ConvertPubKeyFromString(pubKeyStr)
	if err != nil {
		return nil, err
	}
	return pubKey, nil
}

func NewSignedContract(contract *escrowpb.EscrowContract) *escrowpb.SignedEscrowContract {
	return &escrowpb.SignedEscrowContract{
		Contract: contract,
	}
}

func NewContractRequestHelper(configuration *config.Config) (ic.PubKey, error){
	// prepare channel commit
	if len(configuration.Services.EscrowPubKeys) == 0 {
		return nil, fmt.Errorf("No Services.EscrowPubKeys are set in config")
	}
	var escrowPubKey ic.PubKey
	escrowPubKey, err := ConvertToPubKey(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return nil, err
	}
	return escrowPubKey, nil
}
func NewContractRequest(configuration *config.Config, signedContracts []*escrowpb.SignedEscrowContract,
	totalPrice int64) (*escrowpb.EscrowContractRequest, error) {
	// prepare channel commit
	buyerPrivKey, err := configuration.Identity.DecodePrivateKey("")
	if err != nil {
		return nil, err
	}
	var escrowPubKey ic.PubKey
	escrowPubKey, err = NewContractRequestHelper(configuration)
	if err != nil {
		return nil, err
	}

	var chanCommit *ledgerpb.ChannelCommit
	var buyerChanSig []byte
	chanCommit, err = ledger.NewChannelCommit(buyerPrivKey.GetPublic(), escrowPubKey, totalPrice)
	if err != nil {
		return nil, err
	}
	buyerChanSig, err = crypto.Sign(buyerPrivKey, chanCommit)
	if err != nil {
		return nil, err
	}
	return &escrowpb.EscrowContractRequest{
		Contract:     signedContracts,
		BuyerChannel: ledger.NewSignedChannelCommit(chanCommit, buyerChanSig),
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
			err = VerifyEscrowRes(configuration, response.Result, response.EscrowSignature)
			if err != nil {
				return fmt.Errorf("verify escrow failed %v", err)
			}
			return nil
		})
	return response, err
}

func VerifyEscrowRes(configuration *config.Config, message proto.Message, sig []byte) error {
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
	payinReq, err := ledger.NewPayinRequest(result.Result.PayinId, payerPubKey, chanState)
	if err != nil {
		return nil, err
	}
	payinSig, err := crypto.Sign(payerPrivKey, payinReq)
	if err != nil {
		return nil, err
	}
	return ledger.NewSignedPayinRequest(payinReq, payinSig), nil
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
			err = VerifyEscrowRes(configuration, res.Result, res.EscrowSignature)
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

func SignContractAndMarshalOffSign(unsignedContract *escrowpb.EscrowContract, signedBytes []byte, signedContract *escrowpb.SignedEscrowContract,
	isPayer bool) ([]byte, error) {
	if DEBUG {
		b, _ := proto.Marshal(unsignedContract)
		fmt.Println("Unsigned Escrow Contract OFFLINE-12:")
		Dump(b)
	}

	if signedContract == nil {
		signedContract = NewSignedContract(unsignedContract)
	}
	if isPayer {
		signedContract.BuyerSignature = signedBytes
	} else {
		signedContract.SellerSignature = signedBytes
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

func SignContractID(id string, privKey ic.PrivKey) (*escrowpb.SignedContractID, error) {
	contractID, err := ledger.NewContractID(id, privKey.GetPublic())
	if err != nil {
		return nil, err
	}
	// sign contractID
	sig, err := crypto.Sign(privKey, contractID)
	if err != nil {
		return nil, err
	}
	return ledger.NewSingedContractID(contractID, sig), nil
}

func IsPaidin(ctx context.Context, configuration *config.Config, contractID *escrowpb.SignedContractID) (bool, error) {
	var signedPayinRes *escrowpb.SignedPayinStatus
	err := grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.IsPaid(ctx, contractID)
			if err != nil {
				return err
			}
			err = VerifyEscrowRes(configuration, res.Status, res.EscrowSignature)
			if err != nil {
				return err
			}
			signedPayinRes = res
			return nil
		})
	if err != nil {
		return false, err
	}
	return signedPayinRes.Status.Paid, nil
}

func Balance(ctx context.Context, configuration *config.Config) (int64, error) {
	privKey, err := configuration.Identity.DecodePrivateKey("")
	if err != nil {
		return 0, err
	}
	lgSignedPubKey, err := ledger.NewSignedPublicKey(privKey, privKey.GetPublic())
	if err != nil {
		return 0, err
	}
	var balance int64 = 0
	err = grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.BalanceOf(ctx, lgSignedPubKey)
			if err != nil {
				return err
			}
			err = VerifyEscrowRes(configuration, res.Result, res.EscrowSignature)
			if err != nil {
				return err
			}
			balance = res.Result.Balance
			log.Debug("balanceof account is ", balance)
			return nil
		})
	if err != nil {
		return 0, err
	}
	return balance, nil
}
