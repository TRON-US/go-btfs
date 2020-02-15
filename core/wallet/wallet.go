package wallet

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"errors"
	"fmt"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/escrow"
	exPb "github.com/tron-us/go-btfs-common/protos/exchange"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	eth "github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-core/crypto"
)

var log = logging.Logger("core/wallet")

var (
	WithdrawMinAmount int64  = 1
	WithdrawMaxAmount int64  = 10000
	DepositMinAmount  int64  = 1
	DepositMaxAmount  int64  = 10000
	TokenId           string = "1002000"
	hostWallet        Wallet

	escrowService   string
	exchangeService string
	solidityService string
)

type Wallet struct {
	privKeyIC     ic.PrivKey
	privateKey    *ecdsa.PrivateKey
	tronAddress   []byte // 41***
	ledgerAddress []byte // address in ledger
}

// withdraw from ledger to tron
func (wallet *Wallet) Withdraw(amount int64, configuration *config.Config) error {
	if wallet.privateKey == nil {
		log.Error("wallet is not initialized")
		return errors.New("wallet is not initialized")
	}

	if amount < WithdrawMinAmount || amount > WithdrawMaxAmount {
		return errors.New(fmt.Sprintf("withdraw amount should between %d ~ %d", WithdrawMinAmount, WithdrawMaxAmount))
	}

	ctx := context.Background()
	// get ledger balance before withdraw
	ledgerBalance, err := escrow.Balance(ctx, configuration)
	if err != nil {
		return errors.New(fmt.Sprintf("Failed to get ledger balance, reason: %v", err))
	}
	log.Info(fmt.Sprintf("Get ledger account success, balance: [%d]", ledgerBalance))

	if amount > ledgerBalance {
		return errors.New(fmt.Sprintf("not enough ledger balance, current balance is %d", ledgerBalance))
	}

	// Doing withdraw request.
	channelId, id, err := Withdraw(wallet.ledgerAddress, wallet.tronAddress, amount, wallet.privateKey)
	if err != nil {
		log.Error("Failed to Withdraw, ERR[%v]\n", err)
		return err
	}

	log.Info("Withdraw submitted! ChannelId: [%d], id [%d]\n", channelId, id)
	return nil
}

func (wallet *Wallet) Deposit(amount int64) error {
	if wallet.privateKey == nil {
		log.Error("wallet is not initialized")
		return errors.New("wallet is not initialized")
	}

	if amount < DepositMinAmount || amount > DepositMaxAmount {
		return errors.New(fmt.Sprintf("deposit amount should between %d ~ %d", DepositMinAmount, DepositMaxAmount))
	}

	prepareResponse, err := Deposit(wallet.ledgerAddress, amount, wallet.privateKey)
	if err != nil {
		log.Error("Failed to Deposit, ERR[%v]\n", err)
		return err
	}

	log.Info("Deposit Submitted: Id [%d]\n", prepareResponse.GetId())
	return nil
}

//GetBalance both on ledger and Tron.
func (wallet *Wallet) GetBalance(configuration *config.Config) (int64, int64, error) {
	if wallet.ledgerAddress == nil || len(wallet.ledgerAddress) == 0 {
		log.Error("wallet is not initialized")
		return 0, 0, errors.New("wallet is not initialized")
	}

	ctx := context.Background()
	// get tron balance
	tronBalance, err := GetTokenBalance(wallet.tronAddress, TokenId)
	if err != nil {
		return 0, 0,
			errors.New(fmt.Sprintf("Failed to get exchange tron balance, reason: %v", err))
	}
	log.Info(fmt.Sprintf("Get exchange tron account success, balance: [%d]", tronBalance))

	// get ledger balance from escrow
	ledgerBalance, err := escrow.Balance(ctx, configuration)
	if err != nil {
		return 0, 0,
			errors.New(fmt.Sprintf("Failed to get ledger balance, reason: %v", err))
	}
	log.Info(fmt.Sprintf("Get ledger account success, balance: [%d]", ledgerBalance))

	return tronBalance, ledgerBalance, nil
}

// activate account on tron block chain
// using wallet tronAddress 41***
func (wallet *Wallet) ActivateAccount() error {
	if wallet.tronAddress == nil {
		log.Error("wallet is not initialized")
		return errors.New("wallet is not initialized")
	}

	ctx := context.Background()
	err := grpc.ExchangeClient(exchangeService).WithContext(ctx,
		func(ctx context.Context, client exPb.ExchangeClient) error {
			response, err := client.ActivateAccountOnChain(ctx,
				&exPb.ActivateAccountRequest{Address: wallet.tronAddress})
			if err != nil {
				return err
			}
			log.Info("wallet activate account succeed: ", response)
			return nil
		})
	if err != nil {
		log.Error("wallet activate account error: ", err)
		return err
	}
	return nil
}

func Init(configuration *config.Config) {
	if configuration == nil {
		log.Error("init wallet failed, input nil configuration")
		return
	}

	// get service name
	escrowService = configuration.Services.EscrowDomain
	exchangeService = configuration.Services.ExchangeDomain
	solidityService = configuration.Services.SolidityDomain

	// get key
	privKeyIC, err := configuration.Identity.DecodePrivateKey("")
	if err != nil {
		log.Error("wallet get private key failed")
		return
	}
	// base64 key
	privKeyRaw, err := privKeyIC.Raw()
	if err != nil {
		log.Error("wallet get private key raw failed")
		return
	}
	// hex key
	hexPrivKey := hex.EncodeToString(privKeyRaw)
	log.Debug("wallet private key (hex) is:", hexPrivKey)

	// hex key to ecdsa
	privateKey, err := eth.HexToECDSA(hexPrivKey)
	if err != nil {
		log.Error("error when convent private key to edca, ERR[%v]\n", err)
		return
	}
	if privateKey == nil {
		log.Error("wallet get private key ecdsa failed")
		return
	}
	hostWallet.privateKey = privateKey

	// tron key 41****
	addr, err := PublicKeyToAddress(privateKey.PublicKey)
	if err != nil {
		log.Error("wallet get tron address failed, ERR[%v]\n ", err)
		return
	}
	addBytes := addr.Bytes()
	hostWallet.tronAddress = addBytes
	hostWallet.ledgerAddress = elliptic.Marshal(elliptic.P256(), privateKey.PublicKey.X, privateKey.PublicKey.Y)
	log.Info("wallet ledger address: [%s]\n", hex.EncodeToString(hostWallet.ledgerAddress))
	log.Info("wallet tron address: [%s]\n", hex.EncodeToString(hostWallet.tronAddress))
}

func HostWallet() *Wallet {
	return &hostWallet
}
