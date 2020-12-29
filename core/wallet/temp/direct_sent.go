package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/TRON-US/go-btfs/core/wallet"
	walletpb "github.com/TRON-US/go-btfs/protos/wallet"
	"github.com/tron-us/go-btfs-common/crypto"
	tronPb "github.com/tron-us/go-btfs-common/protos/protocol/api"
	corePb "github.com/tron-us/go-btfs-common/protos/protocol/core"
	protocol_core "github.com/tron-us/go-btfs-common/protos/protocol/core"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"

	ds "github.com/ipfs/go-datastore"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/mr-tron/base58"
	"github.com/status-im/keycard-go/hexutils"
)

const (
	FullNodeDomain               = "grpc.trongrid.io:50051"
	walletTransactionV1KeyPrefix = "/btfs/%v/wallet/v1/transactions/"
	walletTransactionV1Key       = walletTransactionV1KeyPrefix + "%v/"
	solidityService              = "grpc.trongrid.io:50052"
	//TokenId                      = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t" //usdt
	//TokenId = "TN3W4H6rK2ce4vX9YnFQHwKENnHjoxb3m9" //btc
)

var TokenId string

func main() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("please input private key of account: ")
	text, _ := reader.ReadString('\n')
	//read the payer id
	privateKeyStr := strings.Replace(text, "\n", "", -1)
	privKey, err := crypto.ToPrivKey(privateKeyStr)
	if err != nil {
		fmt.Println("wrong private key, please check")
		return
	}

	keys, err := crypto.FromIcPrivateKey(privKey)
	if err != nil {
		fmt.Printf("error to parse privkey")
		return
	}

	from := keys.HexAddress
	fromBytes, err := hex.DecodeString(from)
	if err != nil {
		fmt.Printf("error to parse privkey")
		return
	}
	fmt.Print("please input target wallet address: ")
	text, _ = reader.ReadString('\n')
	to := strings.Replace(text, "\n", "", -1)

	//amount := int64(100000)

	for {
		balance, err := GetTokenBalance(context.Background(), fromBytes, TokenId)
		if err != nil {
			fmt.Println("error to get balance")
			return
		}
		doneWork := false
		if balance != nil {
			doneWork = true
			for tokenId, amount := range balance {
				TokenId = tokenId
				if amount == 0 {
					continue
				}
				txId, err := TransferUSDTWithMemo(context.Background(), privKey, to, amount, "failsafe retry")
				if err == nil {
					fmt.Println("done the transaction txId: ", txId)
					//return
				} else {
					fmt.Println("error", err.Error())
				}
			}
		}
		//for balance != nil {
		//	doneWork = true
		//	for
		//
		//	txId, err := TransferUSDTWithMemo(context.Background(), privKey, to, amount, "failsafe retry")
		//	if err == nil {
		//		fmt.Println("done the transaction txId: ", txId)
		//		//return
		//	} else {
		//		fmt.Println("error", err.Error())
		//	}
		//}
		if doneWork {
			return
		}

	}

}

func TransferUSDTWithMemo(ctx context.Context, privKey ic.PrivKey,
	to string, amount int64, memo string) (*string, error) {
	var err error
	//if privKey == nil {
	//	privKey, err = crypto.ToPrivKey(cfg.Identity.PrivKey)
	//	if err != nil {
	//		return nil, err
	//	}
	//}

	keys, err := crypto.FromIcPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	from := keys.HexAddress

	tx, err := PrepareTx(ctx, from, to, amount, memo)
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
	err = wallet.SendRawTransaction(ctx, FullNodeDomain, rawBytes, sig)
	if err != nil {
		return nil, err
	}
	//err = wallet.PersistTx(n.Repo.Datastore(), n.Identity.String(), txId, amount,
	//	BttWallet, to, StatusPending, walletpb.TransactionV1_ON_CHAIN)
	//if err != nil {
	//	return nil, err
	//}
	//go func() {
	//	// confirmed after 19 * 3 second/block
	//	time.Sleep(1 * time.Minute)
	//	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	//	defer cancel()
	//	status, err := GetStatus(ctx, cfg.Services.SolidityDomain, txId)
	//	if err != nil {
	//		log.Error(err)
	//		return
	//	}
	//	err = PersistTx(n.Repo.Datastore(), n.Identity.String(), txId, amount,
	//		BttWallet, to, status, walletpb.TransactionV1_ON_CHAIN)
	//	if err != nil {
	//		log.Error(err)
	//		return
	//	}
	//}()
	return &txId, nil
}

func PrepareTx(ctx context.Context, from string, to string, amount int64, memo string) (*tronPb.TransactionExtention, error) {
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
	//if strings.Contains(cfg.Services.EscrowDomain, "dev") ||
	//	strings.Contains(cfg.Services.EscrowDomain, "staging") {
	//	tokenId = TokenIdDev
	//}
	err = grpc.WalletClient(FullNodeDomain).WithContext(ctx, func(ctx context.Context, client tronPb.WalletClient) error {
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

func PersistTx(d ds.Datastore, peerId string, txId string, amount int64,
	from string, to string, status string, txType walletpb.TransactionV1_Type) error {
	return PersistTxWithTime(d, peerId, txId, amount, from, to, status, txType, time.Now())
}

func PersistTxWithTime(d ds.Datastore, peerId string, txId string, amount int64,
	from string, to string, status string, txType walletpb.TransactionV1_Type, timeCreate time.Time) error {
	return sessions.Save(d, fmt.Sprintf(walletTransactionV1Key, peerId, txId),
		&walletpb.TransactionV1{
			Id:         txId,
			TimeCreate: timeCreate,
			Amount:     amount,
			From:       from,
			To:         to,
			Status:     status,
			Type:       txType,
		})
}

func GetTokenBalance(ctx context.Context, addr []byte, tokenId string) (map[string]int64, error) {

	var result map[string]int64
	err := grpc.SolidityClient(solidityService).WithContext(ctx,
		func(ctx context.Context, client tronPb.WalletSolidityClient) error {
			account := &corePb.Account{Address: addr}

			myAccount, err := client.GetAccount(ctx, account)
			if err != nil {
				return err
			}
			tokenMap := myAccount.GetAssetV2()
			if tokenMap == nil || len(tokenMap) == 0 {
				return nil
			}
			result = tokenMap
			return nil
		})
	if err != nil {
		return nil, err
	}
	return result, nil
}

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
