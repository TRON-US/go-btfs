package wallet

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	walletpb "github.com/TRON-US/go-btfs/protos/wallet"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	tronPb "github.com/tron-us/go-btfs-common/protos/protocol/api"
	protocol_core "github.com/tron-us/go-btfs-common/protos/protocol/core"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	eth "github.com/ethereum/go-ethereum/crypto"
	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/mr-tron/base58/base58"
	"github.com/status-im/keycard-go/hexutils"
)

func TransferBTT(ctx context.Context, n *core.IpfsNode, cfg *config.Config, privKey ic.PrivKey,
	from string, to string, amount int64) (*TronRet, error) {
	var err error
	if privKey == nil {
		privKey, err = crypto.ToPrivKey(cfg.Identity.PrivKey)
		if err != nil {
			return nil, err
		}
	}
	if from == "" {
		from, err = getHexAddress(privKey)
		if err != nil {
			return nil, err
		}
	}
	txId := ""

	tx, err := PrepareTx(ctx, cfg, from, to, amount)
	if err != nil {
		return nil, err
	}
	sig, err := sign(privKey, tx.Transaction.RawData)
	if err != nil {
		return nil, err
	}
	rawBytes, err := proto.Marshal(tx.Transaction.RawData)
	if err != nil {
		return nil, err
	}
	err = SendRawTransaction(ctx, cfg.Services.FullnodeDomain, rawBytes, sig)
	if err != nil {
		return nil, err
	}
	err = PersistTx(n.Repo.Datastore(), n.Identity.String(), txId, amount,
		BttWallet, to, StatusPending, walletpb.TransactionV1_ON_CHAIN)
	if err != nil {
		return nil, err
	}
	go func() {
		// confirmed after 19 * 3 second/block
		time.Sleep(1 * time.Minute)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		status, err := GetStatus(ctx, cfg.Services.SolidityDomain, txId)
		if err != nil {
			log.Error(err)
			return
		}
		err = PersistTx(n.Repo.Datastore(), n.Identity.String(), txId, amount,
			BttWallet, to, status, walletpb.TransactionV1_ON_CHAIN)
		if err != nil {
			log.Error(err)
			return
		}
	}()
	if err != nil {
		e := PersistTx(n.Repo.Datastore(), n.Identity.String(), txId, amount,
			BttWallet, to, StatusFailed, walletpb.TransactionV1_ON_CHAIN)
		log.Debug(e)
		return nil, err
	}
	return &TronRet{
		Message: string(tx.Result.Message),
		Result:  tx.Result.Result,
		Code:    tx.Result.Code.String(),
	}, nil
}

// base58/hex -> hex
func toHex(address string) (string, error) {
	if strings.HasPrefix(address, "T") {
		bytes, err := base58.Decode(address)
		if err != nil {
			return "", err
		}
		address = hexutils.BytesToHex(bytes[:len(bytes)-4])
	}
	return address, nil
}

func sign(privKey ic.PrivKey, msg proto.Message) ([]byte, error) {
	raw, err := privKey.Raw()
	if err != nil {
		return nil, err
	}
	txBytes, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	ecdsa, err := eth.HexToECDSA(hex.EncodeToString(raw))
	sum := sha256.Sum256(txBytes)
	sig, err := eth.Sign(sum[:], ecdsa)
	return sig, err
}

func getHexAddress(privKey ic.PrivKey) (string, error) {
	bytes, err := privKey.Raw()
	if err != nil {
		return "", err
	}
	ecdsa, err := eth.ToECDSA(bytes)
	if err != nil {
		return "", err
	}
	address, err := publicKeyToAddress(ecdsa.PublicKey)
	return hex.EncodeToString(address.Bytes()), nil
}

func publicKeyToAddress(p ecdsa.PublicKey) (Address, error) {
	addr := eth.PubkeyToAddress(p)

	addressTron := make([]byte, AddressLength)

	addressPrefix, err := FromHex(AddressPrefix)
	if err != nil {
		return Address{}, err
	}

	addressTron = append(addressTron, addressPrefix...)
	addressTron = append(addressTron, addr.Bytes()...)

	return BytesToAddress(addressTron), nil
}

type TronRet struct {
	Message string
	Result  bool
	Code    string
}

func PrepareTx(ctx context.Context, cfg *config.Config, from string, to string, amount int64) (*tronPb.TransactionExtention, error) {
	var (
		tx  *tronPb.TransactionExtention
		err error
	)
	from, err = toHex(from)
	if err != nil {
		return nil, err
	}
	to, err = toHex(to)
	if err != nil {
		return nil, err
	}
	oa, err := hex.DecodeString(from)
	if err != nil {
		return nil, err
	}
	ta, err := hex.DecodeString(to)
	if err != nil {
		return nil, err
	}
	tokenId := TokenId
	if strings.Contains(cfg.Services.EscrowDomain, "dev") ||
		strings.Contains(cfg.Services.EscrowDomain, "staging") {
		tokenId = TokenIdDev
	}
	err = grpc.WalletClient(cfg.Services.FullnodeDomain).WithContext(ctx, func(ctx context.Context, client tronPb.WalletClient) error {
		tx, err = client.TransferAsset2(ctx, &protocol_core.TransferAssetContract{
			AssetName:    []byte(tokenId),
			OwnerAddress: oa,
			ToAddress:    ta,
			Amount:       amount,
		})
		if err != nil {
			return err
		}
		if !tx.Result.Result {
			return errors.New(string(tx.Result.Message))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func SendRawTransaction(ctx context.Context, url string, raw []byte, sig []byte) error {
	rawMsg := &protocol_core.TransactionRaw{}
	err := proto.Unmarshal(raw, rawMsg)
	if err != nil {
		return err
	}
	tx := &protocol_core.Transaction{
		RawData:   rawMsg,
		Signature: [][]byte{sig},
	}
	return grpc.WalletClient(url).WithContext(ctx, func(ctx context.Context, client tronPb.WalletClient) error {
		_, err = client.BroadcastTransaction(ctx, tx)
		return err
	})
}

func GetStatus(ctx context.Context, url string, txId string) (string, error) {
	txIdBytes, err := hex.DecodeString(txId)
	if err != nil {
		return "", err
	}
	var info *protocol_core.Transaction
	err = grpc.SolidityClient(url).WithContext(ctx, func(ctx context.Context, client tronPb.WalletSolidityClient) error {
		info, err = client.GetTransactionById(ctx, &tronPb.BytesMessage{Value: txIdBytes})
		return err
	})
	if err != nil {
		return StatusFailed, err
	}
	status := StatusPending
	if info != nil && info.Ret != nil && len(info.Ret) > 0 &&
		info.Ret[0].ContractRet == protocol_core.Transaction_Result_SUCCESS {
		status = StatusSuccess
	} else {
		status = StatusFailed
	}
	return status, nil
}
