package escrow

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/TRON-US/go-btfs/core"
	shardpb "github.com/TRON-US/go-btfs/protos/shard"

	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("core/escrow")

const (
	payoutNotFoundErr = "rpc error: code = Unknown desc = not found"
)

func NewContractID(sessionId string) string {
	id := uuid.New().String()
	return sessionId + "," + id
}

// ExtractSessionIDFromContractID takes the first segment separated by ","
// and returns it as the session id. If in an old format, i.e. did not
// have a session id, return an error.
func ExtractSessionIDFromContractID(contractID string) (string, error) {
	ids := strings.Split(contractID, ",")
	if len(ids) != 2 {
		return "", fmt.Errorf("bad contract id: fewer than 2 segments")
	}
	if len(ids[0]) != 36 {
		return "", fmt.Errorf("invalid session id within contract id")
	}
	return ids[0], nil
}

func NewContract(configuration *config.Config, sessionId string, n *core.IpfsNode, hostPid peer.ID,
	price int64, customizedSchedule bool, period int, offSignPid peer.ID) (*escrowpb.EscrowContract, error) {
	var payerPubKey ic.PubKey
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
	return ledger.NewEscrowContract(NewContractID(sessionId),
		payerPubKey, hostPubKey, authPubKey, price, ps, int32(p))
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

func NewContractRequestHelper(configuration *config.Config) (ic.PubKey, error) {
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

	return BalanceHelper(ctx, configuration, false, nil, lgSignedPubKey)
}

func BalanceHelper(ctx context.Context, configuration *config.Config, offsign bool, signedBytes []byte, lgSignedPubKey *ledgerpb.SignedPublicKey) (int64, error) {
	if offsign {
		var ledgerSignedPubKey ledgerpb.SignedPublicKey
		err := proto.Unmarshal(signedBytes, &ledgerSignedPubKey)
		if err != nil {
			return 0, err
		}
		lgSignedPubKey = &ledgerSignedPubKey
	}

	var balance int64 = 0
	err := grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.BalanceOf(ctx, &ledgerpb.SignedCreateAccountRequest{
				Key:       lgSignedPubKey.Key,
				Signature: lgSignedPubKey.Signature,
			})
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

// SyncContractPayoutStatus looks at local contracts, refreshes from Guard, and then
// syncs payout status for all of them
func SyncContractPayoutStatus(ctx context.Context, n *core.IpfsNode,
	cs []*shardpb.SignedContracts) ([]*nodepb.Contracts_Contract, error) {
	cfg, err := n.Repo.Config()
	if err != nil {
		return nil, err
	}
	pk, err := n.Identity.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	pkBytes, err := ic.RawFull(pk)
	if err != nil {
		return nil, err
	}
	results := make([]*nodepb.Contracts_Contract, 0)
	err = grpc.EscrowClient(cfg.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context,
			client escrowpb.EscrowServiceClient) error {
			in := &escrowpb.SignedContractIDBatch{
				Data: &escrowpb.ContractIDBatch{
					Address: pkBytes,
				},
			}
			for _, c := range cs {
				in.Data.ContractId = append(in.Data.ContractId, c.GuardContract.ContractId)
			}
			sign, err := crypto.Sign(n.PrivateKey, in.Data)
			if err != nil {
				log.Error("sign contractID error:", err)
				return err
			}
			in.Signature = sign
			sb, err := client.GetPayOutStatusBatch(ctx, in)
			if err != nil {
				log.Error("get payout status batch error:", err)
				return err
			}
			if len(sb.Status) != len(cs) {
				return fmt.Errorf("payout status batch returned wrong length of contracts, need %d, got %d",
					len(cs), len(sb.Status))
			}
			for i, c := range cs {
				s := sb.Status[i]
				// Set dummy payout status on invalid id or error msg
				// Reset to dummy status so we have a copy locally with invalid params
				if s.ContractId != c.GuardContract.ContractId {
					s = &escrowpb.PayoutStatus{
						ContractId: c.GuardContract.ContractId,
						ErrorMsg:   "mistmatched contract id",
					}
				} else if s.ErrorMsg != "" {
					s = &escrowpb.PayoutStatus{
						ContractId: c.GuardContract.ContractId,
						ErrorMsg:   s.ErrorMsg,
					}
				}
				if s.ErrorMsg != "" {
					log.Debug("got payout status error message:", s.ErrorMsg)
				}

				results = append(results, &nodepb.Contracts_Contract{
					ContractId:              c.GuardContract.ContractId,
					HostId:                  c.GuardContract.HostPid,
					RenterId:                c.GuardContract.RenterPid,
					Status:                  c.GuardContract.State,
					StartTime:               c.GuardContract.RentStart,
					EndTime:                 c.GuardContract.RentEnd,
					NextEscrowTime:          s.NextPayoutTime,
					CompensationPaid:        s.PaidAmount,
					CompensationOutstanding: s.Amount - s.PaidAmount,
					UnitPrice:               c.GuardContract.Price,
					ShardSize:               c.GuardContract.ShardFileSize,
					ShardHash:               c.GuardContract.ShardHash,
					FileHash:                c.GuardContract.FileHash,
				})
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return results, nil
}
