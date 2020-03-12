package wallet

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	escrowPb "github.com/tron-us/go-btfs-common/protos/escrow"
	exPb "github.com/tron-us/go-btfs-common/protos/exchange"
	ledgerPb "github.com/tron-us/go-btfs-common/protos/ledger"
	tronPb "github.com/tron-us/go-btfs-common/protos/protocol/api"
	corePb "github.com/tron-us/go-btfs-common/protos/protocol/core"
	"github.com/tron-us/go-btfs-common/utils/grpc"
)

var (
	ErrInsufficientExchangeBalanceOnTron   = errors.New("exchange balance on Tron network is not sufficient")
	ErrInsufficientUserBalanceOnTron       = errors.New(fmt.Sprint("User balance on tron network is not sufficient."))
	ErrInsufficientUserBalanceOnLedger     = errors.New("rpc error: code = ResourceExhausted desc = NSF")
	ErrInsufficientExchangeBalanceOnLedger = errors.New("exchange balance on Private Ledger is not sufficient")
)

// Do the deposit action, integrate exchange's PrepareDeposit and Deposit API.
func Deposit(ledgerAddr []byte, amount int64, privateKey *ecdsa.PrivateKey, runDaemon bool) (*exPb.PrepareDepositResponse, error) {
	log.Debug("Deposit begin!")
	//PrepareDeposit
	prepareResponse, err := PrepareDeposit(ledgerAddr, amount)
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
	depositResponse, err := DepositRequest(prepareResponse, privateKey)
	if err != nil {
		log.Error(fmt.Sprintf("Deposit error, reasons: [%v]", err))
		return prepareResponse, err
	}
	if depositResponse.Response.Code != exPb.Response_SUCCESS {
		log.Error(fmt.Sprintf("Deposit failed, reasons: [%s]", string(depositResponse.Response.ReturnMessage)))
		return prepareResponse, errors.New(string(depositResponse.Response.ReturnMessage))
	}
	log.Debug(fmt.Sprintf("Call Deposit API success, id: [%d]", prepareResponse.GetId()))

	if runDaemon {
		go func() {
			ConfirmDepositProcess(prepareResponse, privateKey)
		}()
	} else {
		ConfirmDepositProcess(prepareResponse, privateKey)
	}
	// Doing confirm deposit.

	log.Debug("Deposit end!")
	return prepareResponse, nil
}

// Call exchange's PrepareDeposit API.
func PrepareDeposit(ledgerAddr []byte, amount int64) (*exPb.PrepareDepositResponse, error) {
	// Prepare to Deposit.
	ctx := context.Background()
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
func DepositRequest(prepareResponse *exPb.PrepareDepositResponse, privateKey *ecdsa.PrivateKey) (*exPb.DepositResponse, error) {
	// Sign Tron Transaction.
	tronTransaction := prepareResponse.GetTronTransaction()
	for range tronTransaction.GetRawData().GetContract() {
		signature, err := Sign(tronTransaction, privateKey)
		if err != nil {
			return nil, err
		}
		tronTransaction.Signature = append(tronTransaction.GetSignature(), signature)
	}

	ctx := context.Background()
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
func ConfirmDepositProcess(prepareResponse *exPb.PrepareDepositResponse, privateKey *ecdsa.PrivateKey) {
	ctx := context.Background()
	log.Debug(fmt.Sprintf("[Id:%d] ConfirmDepositProcess begin.", prepareResponse.GetId()))

	// Continuous call ConfirmDeposit until it responses a FAILED or SUCCESS.
	for time.Now().UnixNano()/1e6 < (prepareResponse.GetTronTransaction().GetRawData().GetExpiration() + 240*1000) {
		time.Sleep(10 * time.Second)

		// ConfirmDeposit after 1min, because watcher need wait for 1min to confirm tron transaction.
		if time.Now().UnixNano()/1e6 < (prepareResponse.GetTronTransaction().GetRawData().GetTimestamp() + 60*1000) {
			continue
		}
		log.Debug(fmt.Sprintf("[Id:%d] ConfirmDeposit begin.", prepareResponse.GetId()))

		confirmDepositResponse, err := ConfirmDeposit(prepareResponse.GetId())
		if err != nil {
			log.Error(fmt.Sprintf("[Id:%d]ConfirmDeposit error: %v", prepareResponse.GetId(), err))
			continue
		}
		if confirmDepositResponse.GetResponse().GetCode() == exPb.Response_TRANSACTION_PENDING {
			log.Debug(fmt.Sprintf("[Id:%d]TronTransaction is PENDING.", prepareResponse.GetId()))
			continue
		}
		if confirmDepositResponse.GetResponse().GetCode() == exPb.Response_TRANSACTION_FAILED {
			log.Info(fmt.Sprintf("[Id:%d]TronTransaction is FAILED, deposit failed.", prepareResponse.GetId()))
			return
		}
		if confirmDepositResponse.GetResponse().GetCode() == exPb.Response_SUCCESS {
			signSuccessChannelState := confirmDepositResponse.GetSuccessChannelState()
			if signSuccessChannelState != nil {
				toSignature, err := Sign(signSuccessChannelState.GetChannel(), privateKey)
				if err != nil {
					log.Error(fmt.Sprintf("[Id:%d] Sign SuccessChannelState error: %v", prepareResponse.GetId(), err))
					return
				}
				signSuccessChannelState.ToSignature = toSignature
			} else {
				log.Error(fmt.Sprintf("[Id:%d] SignSuccessChannelState is nil", prepareResponse.GetId()))
				return
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
				return
			}

			log.Info(fmt.Sprintf("[Id:%d] Close SuccessChannelState succeed.", prepareResponse.GetId()))
			return
		}
	}
	if time.Now().UnixNano()/1e6 >= (prepareResponse.GetTronTransaction().GetRawData().GetExpiration() + 240*1000) {
		log.Error(fmt.Sprintf("[Id:%d] Didn't get the tron transaction results until the expiration time.",
			prepareResponse.GetId()))
	}
}

// Call exchange's ConfirmDeposit API.
func ConfirmDeposit(logId int64) (*exPb.ConfirmDepositResponse, error) {
	ctx := context.Background()
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
func Withdraw(ledgerAddr, externalAddr []byte, amount int64, privateKey *ecdsa.PrivateKey) (int64, int64, error) {
	log.Debug("Withdraw begin!")
	outTxId := time.Now().UnixNano()
	//PrepareWithdraw
	prepareResponse, err := PrepareWithdraw(ledgerAddr, externalAddr, amount, outTxId)
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

	ctx := context.Background()
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
	withdrawResponse, err := WithdrawRequest(channelId, ledgerAddr, amount, prepareResponse, privateKey)
	if err != nil {
		log.Error(fmt.Sprintf("Withdraw error, reasons: [%v]", err))
		return 0, 0, err
	}
	if withdrawResponse.Response.Code != exPb.Response_SUCCESS {
		log.Error(fmt.Sprintf("Withdraw failed, reasons: [%s]", string(withdrawResponse.Response.ReturnMessage)))
		return 0, 0, errors.New(string(withdrawResponse.Response.ReturnMessage))
	}
	log.Debug("Withdraw end!")
	return channelId.Id, prepareResponse.GetId(), nil
}

// Call exchange's Withdraw API
func PrepareWithdraw(ledgerAddr, externalAddr []byte, amount, outTxId int64) (
	*exPb.PrepareWithdrawResponse, error) {
	ctx := context.Background()
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
func WithdrawRequest(channelId *ledgerPb.ChannelID, ledgerAddr []byte,
	amount int64, prepareResponse *exPb.PrepareWithdrawResponse, privateKey *ecdsa.PrivateKey) (*exPb.WithdrawResponse,
	error) {
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

	ctx := context.Background()
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
func GetTokenBalance(addr []byte, tokenId string) (int64, error) {
	ctx := context.Background()
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
