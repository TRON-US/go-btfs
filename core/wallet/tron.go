package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core"
	"github.com/thedevsaddam/gojsonq/v2"
	"github.com/tron-us/go-btfs-common/crypto"
	tronPb "github.com/tron-us/go-btfs-common/protos/protocol/api"
	"io/ioutil"
	"net/http"
	"strings"

	protocol_core "github.com/tron-us/go-btfs-common/protos/protocol/core"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/mr-tron/base58/base58"
	"github.com/status-im/keycard-go/hexutils"
)

type TxData struct {
	amount    int64
	assetName string
	from      string
	to        string
	timestamp int64
}

func TransferBTT(ctx context.Context, n *core.IpfsNode, cfg *config.Config, privKey ic.PrivKey,
	from string, to string, amount int64) (*TronRet, error) {
	return TransferBTTWithMemo(ctx, n, cfg, privKey, from, to, amount, "")
}

func TransferBTTWithMemo(ctx context.Context, n *core.IpfsNode, cfg *config.Config, privKey ic.PrivKey,
	from string, to string, amount int64, memo string) (*TronRet, error) {
	var err error
	if privKey == nil {
		privKey, err = crypto.ToPrivKey(cfg.Identity.PrivKey)
		if err != nil {
			return nil, err
		}
	}
	if from == "" {
		keys, err := crypto.FromIcPrivateKey(privKey)
		if err != nil {
			return nil, err
		}
		from = keys.HexAddress
	}
	tx, err := PrepareTx(ctx, cfg, from, to, amount, memo)
	if err != nil {
		return nil, err
	}
	txId := ""
	if tx.Txid != nil {
		txId = hex.EncodeToString(tx.Txid)
	}
	raw, err := privKey.Raw()
	if err != nil {
		return nil, err
	}
	ecdsa, err := crypto.HexToECDSA(hex.EncodeToString(raw))
	if err != nil {
		return nil, err
	}
	bs, err := proto.Marshal(tx.Transaction.RawData)
	if err != nil {
		return nil, err
	}
	sig, err := crypto.EcdsaSign(ecdsa, bs)
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
	return &TronRet{
		Message: string(tx.Result.Message),
		Result:  tx.Result.Result,
		Code:    tx.Result.Code.String(),
		TxId:    txId,
	}, nil
}

// base58/hex -> hex
func toHex(address string) (string, error) {
	if strings.HasPrefix(address, "T") {
		bytes, err := base58.Decode(address)
		if err != nil {
			return "", err
		}
		if len(bytes) <= 4 {
			return "", errors.New("invalid address")
		}
		address = hexutils.BytesToHex(bytes[:len(bytes)-4])
	}
	return address, nil
}

type TronRet struct {
	Message string
	Result  bool
	Code    string
	TxId    string
}

func PrepareTx(ctx context.Context, cfg *config.Config, from string, to string, amount int64, memo string) (*tronPb.TransactionExtention, error) {
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
	tx.Transaction.RawData.Data = []byte(memo)
	bs, err := proto.Marshal(tx.Transaction.RawData)
	if err != nil {
		return nil, err
	}
	hashed := sha256.Sum256(bs)
	tx.Txid = hashed[:]
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

var client = http.DefaultClient

func GetStatus(ctx context.Context, url string, txId string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/wallet/gettransactionbyid?value="+txId, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	return parseTxResult(string(body))
}

func parseTxResult(body string) (string, error) {
	jq := gojsonq.New().FromString(body).From("ret").Select("contractRet")
	ret, ok := jq.Get().([]interface{})
	if !ok {
		return "", errors.New("get tx error")
	} else if len(ret) == 0 {
		return "", errors.New("get tx error")
	}
	return ret[0].(map[string]interface{})["contractRet"].(string), nil
}

func GetBalanceByWalletAddress(ctx context.Context, solidityUrl string, walletAddress string) (tokenMap map[string]int64, err error) {
	address, err := toHex(walletAddress)
	if err != nil {
		return nil, err
	}

	oa, err := hex.DecodeString(address)
	if err != nil {
		return nil, err
	}

	err = grpc.SolidityClient(solidityUrl).WithContext(ctx, func(ctx context.Context, client tronPb.WalletSolidityClient) error {
		account := &protocol_core.Account{Address: oa}
		myAccount, err := client.GetAccount(ctx, account)
		if err != nil {
			return err
		}
		tokenMap = myAccount.GetAssetV2()
		if tokenMap == nil {
			tokenMap = make(map[string]int64)
		}
		tokenMap["TRX"] = myAccount.Balance
		return nil
	})

	return tokenMap, err
}
