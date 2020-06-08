package wallet

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	walletpb "github.com/TRON-US/go-btfs/protos/wallet"

	escrowPb "github.com/tron-us/go-btfs-common/protos/escrow"
	exPb "github.com/tron-us/go-btfs-common/protos/exchange"
	ledgerPb "github.com/tron-us/go-btfs-common/protos/ledger"
	tronPb "github.com/tron-us/go-btfs-common/protos/protocol/api"
	corePb "github.com/tron-us/go-btfs-common/protos/protocol/core"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"
)

var (
	ErrInsufficientExchangeBalanceOnTron   = errors.New("exchange balance on Tron network is not sufficient")
	ErrInsufficientUserBalanceOnTron       = errors.New(fmt.Sprint("User balance on tron network is not sufficient."))
	ErrInsufficientUserBalanceOnLedger     = errors.New("rpc error: code = ResourceExhausted desc = NSF")
	ErrInsufficientExchangeBalanceOnLedger = errors.New("exchange balance on Private Ledger is not sufficient")
)

// Do the deposit action, integrate exchange's PrepareDeposit and Deposit API.
func Deposit(ctx context.Context, n *core.IpfsNode, ledgerAddr []byte, amount int64,
	privateKey *ecdsa.PrivateKey, runDaemon bool, async bool) (*exPb.PrepareDepositResponse, error) {
	log.Debug("Deposit begin!")
	//PrepareDeposit
	prepareResponse, err := PrepareDeposit(ctx, ledgerAddr, amount)
	if err != nil {
		log.Error(fmt.Sprintf("PrepareDeposit error, reasons: [%v]", err))
		return nil, err
	}
	if prepareResponse.Response.Code != exPb.Response_SUCCESS {
		log.Error(fmt.Sprintf("PrepareDeposit failed, reasons: [%s]", string(prepareResponse.Response.ReturnMessage)))
		return prepareResponse, errors.New(string(prepareResponse.Response.ReturnMessage))
	}
	log.Debug(fmt.Sprintf("PrepareDeposit success, id: [%d]", prepareResponse.GetId()))

	//Do the DepositRequest.
	depositResponse, err := DepositRequest(ctx, prepareResponse, privateKey)
	if err != nil {
		log.Error(fmt.Sprintf("Deposit error, reasons: [%v]", err))
		return prepareResponse, err
	}
	if depositResponse.Response.Code != exPb.Response_SUCCESS {
		log.Error(fmt.Sprintf("Deposit failed, reasons: [%s]", string(depositResponse.Response.ReturnMessage)))
		return prepareResponse, errors.New(string(depositResponse.Response.ReturnMessage))
	}
	log.Debug(fmt.Sprintf("Call Deposit API success, id: [%d]", prepareResponse.GetId()))

	err = PersistTx(n.Repo.Datastore(), n.Identity.Pretty(), strconv.FormatInt(prepareResponse.GetId(), 10),
		amount, BttWallet, InAppWallet, StatusPending)
	if err != nil {
		log.Error(fmt.Sprintf("Record deposit tx failed, reasons: [%v]", err))
		return nil, err
	}

	if runDaemon && async {
		go ConfirmDepositProcess(context.Background(), n, prepareResponse, privateKey)
	} else {
		err := ConfirmDepositProcess(ctx, n, prepareResponse, privateKey)
		if err != nil {
			return nil, err
		}
	}
	// Doing confirm deposit.

	log.Debug("Deposit end!")
	return prepareResponse, nil
}

// Call exchange's PrepareDeposit API.
func PrepareDeposit(ctx context.Context, ledgerAddr []byte, amount int64) (*exPb.PrepareDepositResponse, error) {
	// Prepare to Deposit.
	var err error
	var prepareResponse *exPb.PrepareDepositResponse
	err = grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			prepareDepositRequest := &exPb.PrepareDepositRequest{Amount: amount, OutTxId: time.Now().UnixNano(),
				UserAddress: ledgerAddr}
			prepareResponse, err = client.PrepareDeposit(ctx, prepareDepositRequest)
			if err != nil {
				log.Error(fmt.Sprintf("PrepareDeposit error, reasons: [%v]", err))
				return err
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return prepareResponse, nil
}

// Call exchange's Deposit API
func DepositRequest(ctx context.Context, prepareResponse *exPb.PrepareDepositResponse, privateKey *ecdsa.PrivateKey) (*exPb.DepositResponse, error) {
	// Sign Tron Transaction.
	tronTransaction := prepareResponse.GetTronTransaction()
	for range tronTransaction.GetRawData().GetContract() {
		signature, err := Sign(tronTransaction, privateKey)
		if err != nil {
			return nil, err
		}
		tronTransaction.Signature = append(tronTransaction.GetSignature(), signature)
	}

	var err error
	var depositResponse *exPb.DepositResponse
	err = grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			depositRequest := &exPb.DepositRequest{Id: prepareResponse.GetId(), SignedTronTransaction: tronTransaction}

			depositResponse, err = client.Deposit(ctx, depositRequest)
			if err != nil {
				log.Error(fmt.Sprintf("Deposit error, reasons: [%v]", err))
				return err
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return depositResponse, nil
}

// Continuous call ConfirmDeposit until it responses a FAILED or SUCCESS.
func ConfirmDepositProcess(ctx context.Context, n *core.IpfsNode, prepareResponse *exPb.PrepareDepositResponse,
	privateKey *ecdsa.PrivateKey) error {
	log.Debug(fmt.Sprintf("[Id:%d] ConfirmDepositProcess begin.", prepareResponse.GetId()))

	// Continuous call ConfirmDeposit until it responses a FAILED or SUCCESS.
	for time.Now().UnixNano()/1e6 < (prepareResponse.GetTronTransaction().GetRawData().GetExpiration() + 240*1000) {
		time.Sleep(10 * time.Second)

		// ConfirmDeposit after 1min, because watcher need wait for 1min to confirm tron transaction.
		if time.Now().UnixNano()/1e6 < (prepareResponse.GetTronTransaction().GetRawData().GetTimestamp() + 60*1000) {
			continue
		}
		log.Debug(fmt.Sprintf("[Id:%d] ConfirmDeposit begin.", prepareResponse.GetId()))

		confirmDepositResponse, err := ConfirmDeposit(ctx, prepareResponse.GetId())
		if err != nil {
			log.Error(fmt.Sprintf("[Id:%d]ConfirmDeposit error: %v", prepareResponse.GetId(), err))
			continue
		}
		if confirmDepositResponse.GetResponse().GetCode() == exPb.Response_TRANSACTION_PENDING {
			log.Debug(fmt.Sprintf("[Id:%d]TronTransaction is PENDING.", prepareResponse.GetId()))

			err := UpdateStatus(n.Repo.Datastore(), n.Identity.Pretty(), prepareResponse.GetId(), StatusPending)
			if err != nil {
				log.Error(fmt.Sprintf("[Id:%d]Update tx status error: %v", prepareResponse.GetId(), err))
			}
			continue
		}
		if confirmDepositResponse.GetResponse().GetCode() == exPb.Response_TRANSACTION_FAILED {
			log.Info(fmt.Sprintf("[Id:%d]TronTransaction is FAILED, deposit failed.", prepareResponse.GetId()))
			return UpdateStatus(n.Repo.Datastore(), n.Identity.Pretty(), prepareResponse.GetId(), StatusFailed)
		}
		if confirmDepositResponse.GetResponse().GetCode() == exPb.Response_SUCCESS {
			signSuccessChannelState := confirmDepositResponse.GetSuccessChannelState()
			if signSuccessChannelState != nil {
				toSignature, err := Sign(signSuccessChannelState.GetChannel(), privateKey)
				if err != nil {
					log.Error(fmt.Sprintf("[Id:%d] Sign SuccessChannelState error: %v", prepareResponse.GetId(), err))
					return err
				}
				signSuccessChannelState.ToSignature = toSignature

				err = UpdateStatus(n.Repo.Datastore(), n.Identity.Pretty(), prepareResponse.GetId(), StatusSuccess)
				if err != nil {
					log.Error(fmt.Sprintf("[Id:%d]Update tx status error: %v", prepareResponse.GetId(), err))
					return err
				}
			} else {
				return fmt.Errorf("[Id:%d] SignSuccessChannelState is nil", prepareResponse.GetId())
			}

			err = grpc.EscrowClient(escrowService).WithContext(ctx,
				func(ctx context.Context, client escrowPb.EscrowServiceClient) error {
					_, err = client.CloseChannel(ctx, signSuccessChannelState)
					if err != nil {
						log.Error(fmt.Sprintf("[Id:%d] Close SuccessChannelState error: %v", prepareResponse.GetId(), err))
						return err
					}
					return nil
				})
			if err != nil {
				return err
			}

			log.Info(fmt.Sprintf("[Id:%d] Close SuccessChannelState succeed.", prepareResponse.GetId()))
			return nil
		}
	}
	if time.Now().UnixNano()/1e6 >= (prepareResponse.GetTronTransaction().GetRawData().GetExpiration() + 240*1000) {
		err := fmt.Errorf("[Id:%d] Didn't get the tron transaction results until the expiration time.",
			prepareResponse.GetId())
		log.Error(err)
		return err
	}
	return nil
}

// Call exchange's ConfirmDeposit API.
func ConfirmDeposit(ctx context.Context, logId int64) (*exPb.ConfirmDepositResponse, error) {
	var err error
	var confirmDepositResponse *exPb.ConfirmDepositResponse
	err = grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			confirmDepositRequest := &exPb.ConfirmDepositRequest{Id: logId}
			confirmDepositResponse, err = client.ConfirmDeposit(ctx, confirmDepositRequest)
			if err != nil {
				log.Error(fmt.Sprintf("ConfirmDeposit failed, reasons: [%v]", err))
				return err
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return confirmDepositResponse, nil
}

// Do the withdraw action, integrate exchange's PrepareWithdraw and Withdraw API, return channel id and error.
// If Withdraw succeed, with return channel id and logInfo id, error is nil; otherwise will return error, channel id
// and logInfo id is 0.
func Withdraw(ctx context.Context, n *core.IpfsNode, ledgerAddr, externalAddr []byte, amount int64,
	privateKey *ecdsa.PrivateKey) (int64, int64, error) {
	log.Debug("Withdraw begin!")
	outTxId := time.Now().UnixNano()
	//PrepareWithdraw
	prepareResponse, err := PrepareWithdraw(ctx, ledgerAddr, externalAddr, amount, outTxId)
	if err != nil {
		log.Error(fmt.Sprintf("Prepare withdraw error, reasons: [%v]", err))
		return 0, 0, err
	}
	if prepareResponse.Response.Code != exPb.Response_SUCCESS {
		log.Error(fmt.Sprintf("Prepare withdraw failed, reasons: [%s]", string(prepareResponse.Response.ReturnMessage)))
		return 0, 0, errors.New(string(prepareResponse.Response.ReturnMessage))
	}
	log.Debug(fmt.Sprintf("Prepare withdraw success, id: [%d]", prepareResponse.GetId()))

	channelCommit := &ledgerPb.ChannelCommit{
		Payer:     &ledgerPb.PublicKey{Key: ledgerAddr},
		Recipient: &ledgerPb.PublicKey{Key: prepareResponse.GetLedgerExchangeAddress()},
		Amount:    amount,
		PayerId:   time.Now().UnixNano() + prepareResponse.GetId(),
	}
	//Sign channel commit.
	signature, err := Sign(channelCommit, privateKey)
	if err != nil {
		log.Error(fmt.Sprintf("Sign error, reasons: [%v]", err))
		return 0, 0, err
	}

	var channelId *ledgerPb.ChannelID
	err = grpc.EscrowClient(escrowService).WithContext(ctx,
		func(ctx context.Context, client escrowPb.EscrowServiceClient) error {
			channelId, err = client.CreateChannel(ctx,
				&ledgerPb.SignedChannelCommit{Channel: channelCommit, Signature: signature})
			if err != nil {
				if err.Error() == ErrInsufficientUserBalanceOnLedger.Error() {
					log.Error(fmt.Sprintf("[Addr:%s] balance on ledger is insufficient.", hex.EncodeToString(ledgerAddr)))
					return ErrInsufficientUserBalanceOnLedger
				}
				log.Error(fmt.Sprintf("Create channel error, reasons: [%v]", err))
				return err
			}
			return nil
		})
	if err != nil {
		return 0, 0, err
	}
	log.Debug(fmt.Sprintf("CreateChannel success, channelId: [%d]", channelId.GetId()))

	//Do the WithdrawRequest.
	withdrawResponse, err := WithdrawRequest(ctx, channelId, ledgerAddr, amount, prepareResponse, privateKey)
	if err != nil {
		log.Error(fmt.Sprintf("Withdraw error, reasons: [%v]", err))
		return 0, 0, err
	}

	err = PersistTx(n.Repo.Datastore(), n.Identity.Pretty(), strconv.FormatInt(prepareResponse.GetId(), 10), amount,
		InAppWallet, BttWallet, StatusPending)
	if err != nil {
		log.Error(fmt.Sprintf("Persist tx error, reasons: [%v]", err))
		return 0, 0, err
	}

	if withdrawResponse.Response.Code != exPb.Response_SUCCESS {
		log.Error(fmt.Sprintf("Withdraw failed, reasons: [%s]", string(withdrawResponse.Response.ReturnMessage)))

		err := UpdateStatus(n.Repo.Datastore(), n.Identity.Pretty(), prepareResponse.GetId(), StatusFailed)
		if err != nil {
			log.Error(fmt.Sprintf("Update tx status error, reasons: [%v]", err))
			return 0, 0, err
		}
		return 0, 0, errors.New(string(withdrawResponse.Response.ReturnMessage))
	}
	log.Debug("Withdraw end!")
	err = UpdateStatus(n.Repo.Datastore(), n.Identity.Pretty(), prepareResponse.GetId(), StatusSuccess)
	if err != nil {
		log.Error(fmt.Sprintf("Update tx status error, reasons: [%v]", err))
		return 0, 0, err
	}
	return channelId.Id, prepareResponse.GetId(), nil
}

// Call exchange's Withdraw API
func PrepareWithdraw(ctx context.Context, ledgerAddr, externalAddr []byte, amount, outTxId int64) (
	*exPb.PrepareWithdrawResponse, error) {
	var err error
	var prepareResponse *exPb.PrepareWithdrawResponse
	err = grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			prepareWithdrawRequest := &exPb.PrepareWithdrawRequest{
				Amount: amount, OutTxId: outTxId, UserAddress: ledgerAddr, UserExternalAddress: externalAddr}
			prepareResponse, err = client.PrepareWithdraw(ctx, prepareWithdrawRequest)
			if err != nil {
				log.Error(fmt.Sprintf("PrepareWithdraw error, reasons: [%v]", err))
				return err
			}
			log.Debug(prepareResponse)
			return nil
		})
	if err != nil {
		log.Error("wallet PrepareWithdraw error: ", err)
		return nil, err
	}

	return prepareResponse, nil
}

// Call exchange's PrepareWithdraw API
func WithdrawRequest(ctx context.Context, channelId *ledgerPb.ChannelID, ledgerAddr []byte, amount int64,
	prepareResponse *exPb.PrepareWithdrawResponse, privateKey *ecdsa.PrivateKey) (*exPb.WithdrawResponse, error) {
	//make signed success channel state.
	successChannelState := &ledgerPb.ChannelState{
		Id:       channelId,
		Sequence: 1,
		From: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: ledgerAddr,
			},
			Balance: 0,
		},
		To: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: prepareResponse.GetLedgerExchangeAddress(),
			},
			Balance: amount,
		},
	}
	successSignature, err := Sign(successChannelState, privateKey)
	if err != nil {
		return nil, err
	}
	successChannelStateSigned := &ledgerPb.SignedChannelState{Channel: successChannelState, FromSignature: successSignature}

	//make signed fail channel state.
	failChannelState := &ledgerPb.ChannelState{
		Id:       channelId,
		Sequence: 1,
		From: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: ledgerAddr,
			},
			Balance: amount,
		},
		To: &ledgerPb.Account{
			Address: &ledgerPb.PublicKey{
				Key: prepareResponse.GetLedgerExchangeAddress(),
			},
			Balance: 0,
		},
	}
	failSignature, err := Sign(failChannelState, privateKey)
	if err != nil {
		return nil, err
	}

	var withdrawResponse *exPb.WithdrawResponse
	err = grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			failChannelStateSigned := &ledgerPb.SignedChannelState{Channel: failChannelState, FromSignature: failSignature}
			//Post the withdraw request.
			withdrawRequest := &exPb.WithdrawRequest{
				Id:                  prepareResponse.GetId(),
				SuccessChannelState: successChannelStateSigned,
				FailureChannelState: failChannelStateSigned,
			}
			withdrawResponse, err = client.Withdraw(ctx, withdrawRequest)
			if err != nil {
				log.Error(fmt.Sprintf("Withdraw error, reasons: [%v]", err))
				return err
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return withdrawResponse, nil
}

//Get the token balance on tron blockchain
func GetTokenBalance(ctx context.Context, addr []byte, tokenId string) (int64, error) {
	var tokenBalance int64 = 0
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
			tokenBalance = tokenMap[tokenId]
			return nil
		})
	if err != nil {
		log.Error("wallet GetTokenBalance error: ", err)
		return 0, err
	}
	return tokenBalance, nil
}

var (
	walletTransactionKeyPrefix = "/btfs/%v/wallet/transactions/"
	walletTransactionKey       = walletTransactionKeyPrefix + "%v/"

	walletTransactionV1KeyPrefix = "/btfs/%v/wallet/v1/transactions/"
	walletTransactionV1Key       = walletTransactionV1KeyPrefix + "%v/"
)

func PersistTx(d ds.Datastore, peerId string, txId string, amount int64,
	from string, to string, status string) error {
	return sessions.Save(d, fmt.Sprintf(walletTransactionV1Key, peerId, txId),
		&walletpb.TransactionV1{
			Id:         txId,
			TimeCreate: time.Now(),
			Amount:     amount,
			From:       from,
			To:         to,
			Status:     status,
		})
}

func UpdateStatus(d ds.Datastore, peerId string, txId int64, status string) error {
	key := fmt.Sprintf(walletTransactionV1Key, peerId, txId)
	s := &walletpb.TransactionV1{}
	err := sessions.Get(d, key, s)
	if err != nil {
		return err
	}
	if s.Status != status {
		s.Status = status
		return sessions.Save(d, key, s)
	}
	return nil
}

func GetTransactions(d ds.Datastore, peerId string) ([]*walletpb.TransactionV1, error) {
	txs := make(TxSlice, 0)
	v0Txs, err := loadV0Txs(d, peerId)
	if err != nil || v0Txs == nil {
		//ignore, NOP
		err = nil
	}
	txs = append(txs, v0Txs...)
	list, err := sessions.List(d, fmt.Sprintf(walletTransactionV1KeyPrefix, peerId))
	if err != nil {
		return nil, err
	}
	for _, bytes := range list {
		tx := new(walletpb.TransactionV1)
		err := proto.Unmarshal(bytes, tx)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	sort.Sort(txs)
	return txs, nil
}

type TxSlice []*walletpb.TransactionV1

func (p TxSlice) Len() int {
	return len(p)
}

func (p TxSlice) Less(i, j int) bool {
	return p[i].TimeCreate.After(p[j].TimeCreate)
}

func (p TxSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func loadV0Txs(d ds.Datastore, peerId string) ([]*walletpb.TransactionV1, error) {
	list, err := sessions.List(d, fmt.Sprintf(walletTransactionKeyPrefix, peerId))
	if err != nil {
		return nil, err
	}
	txs := make([]*walletpb.TransactionV1, 0)
	for _, bytes := range list {
		tx := new(walletpb.Transaction)
		err := proto.Unmarshal(bytes, tx)
		if err != nil {
			return nil, err
		}
		txs = append(txs, &walletpb.TransactionV1{
			Id:         strconv.FormatInt(tx.Id, 10),
			TimeCreate: tx.TimeCreate,
			Amount:     tx.Amount,
			From:       tx.From,
			To:         tx.To,
			Status:     tx.Status,
		})
	}
	return txs, nil
}
