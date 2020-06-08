package wallet

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/TRON-US/go-btfs/core"

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

const (
	BTTName = "1002000"
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
	from, err = toHex(from)
	if err != nil {
		return nil, err
	}
	to, err = toHex(to)
	if err != nil {
		return nil, err
	}
	url := cfg.Services.FullnodeDomain
	oa, err := hex.DecodeString(from)
	if err != nil {
		return nil, err
	}
	ta, err := hex.DecodeString(to)
	if err != nil {
		return nil, err
	}
	var ret *tronPb.Return
	txId := ""
	err = grpc.WalletClient(url).WithContext(ctx, func(ctx context.Context, client tronPb.WalletClient) error {
		tx, err := client.TransferAsset2(context.Background(), &protocol_core.TransferAssetContract{
			AssetName:    []byte(BTTName),
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
		txId = hex.EncodeToString(tx.Txid)
		sig, err := sign(privKey, tx.Transaction.RawData)
		if err != nil {
			return err
		}
		tx.Transaction.Signature = [][]byte{sig}
		ret, err = client.BroadcastTransaction(ctx, tx.Transaction)
		if err != nil {
			return err
		}
		PersistTx(n.Repo.Datastore(), n.Identity.String(), txId, amount, BttWallet, to, StatusSuccess)
		return nil
	})
	if err != nil {
		PersistTx(n.Repo.Datastore(), n.Identity.String(), txId, amount, BttWallet, to, StatusFailed)
		return nil, err
	}
	return &TronRet{
		Message: string(ret.Message),
		Result:  ret.Result,
		Code:    ret.Code.String(),
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
