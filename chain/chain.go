package chain

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/accounting"
	"github.com/TRON-US/go-btfs/chain/config"
	"github.com/TRON-US/go-btfs/settlement"
	"github.com/TRON-US/go-btfs/settlement/swap"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	"github.com/TRON-US/go-btfs/settlement/swap/priceoracle"
	"github.com/TRON-US/go-btfs/settlement/swap/swapprotocol"
	"github.com/TRON-US/go-btfs/transaction"
	"github.com/TRON-US/go-btfs/transaction/crypto"
	"github.com/TRON-US/go-btfs/transaction/sctx"
	"github.com/TRON-US/go-btfs/transaction/storage"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	logging "github.com/ipfs/go-log"
)

var (
	log          = logging.Logger("chain")
	ChainObject  ChainInfo
	SettleObject SettleInfo
)

const (
	MaxDelay          = 1 * time.Minute
	CancellationDepth = 6
)

type ChainInfo struct {
	Chainconfig        config.ChainConfig
	Backend            transaction.Backend
	OverlayAddress     common.Address
	Signer             crypto.Signer
	ChainID            int64
	TransactionMonitor transaction.Monitor
	TransactionService transaction.Service
}

type SettleInfo struct {
	Factory           chequebook.Factory
	ChequebookService chequebook.Service
	ChequeStore       chequebook.ChequeStore
	CashoutService    chequebook.CashoutService
	SwapService       *swap.Service
	OracleService     priceoracle.Service
}

// InitChain will initialize the Ethereum backend at the given endpoint and
// set up the Transaction Service to interact with it using the provided signer.
func InitChain(
	ctx context.Context,
	stateStore storage.StateStorer,
	endpoint string,
	signer crypto.Signer,
	pollingInterval time.Duration,
) (*ChainInfo, error) {
	backend, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, fmt.Errorf("dial eth client: %w", err)
	}

	chainID, err := backend.ChainID(ctx)
	if err != nil {
		log.Infof("could not connect to backend at %v. In a swap-enabled network a working blockchain node (for goerli network in production) is required. Check your node or specify another node using --swap-endpoint.", endpoint)
		return nil, fmt.Errorf("get chain id: %w", err)
	}

	chainconfig, _ := config.GetChainConfig(chainID.Int64())

	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, fmt.Errorf("eth address: %w", err)
	}

	transactionMonitor := transaction.NewMonitor(backend, overlayEthAddress, pollingInterval, CancellationDepth)

	transactionService, err := transaction.NewService(backend, signer, stateStore, chainID, transactionMonitor)
	if err != nil {
		return nil, fmt.Errorf("new transaction service: %w", err)
	}

	ChainObject = ChainInfo{
		Chainconfig:        *chainconfig,
		Backend:            backend,
		OverlayAddress:     overlayEthAddress,
		ChainID:            chainID.Int64(),
		TransactionMonitor: transactionMonitor,
		TransactionService: transactionService,
	}

	return &ChainObject, nil
}

func InitSettlement(
	ctx context.Context,
	stateStore storage.StateStorer,
	chaininfo *ChainInfo,
	initialDeposit string,
	deployGasPrice string,
	chainID uint64,
) (*SettleInfo, error) {
	//InitChequebookFactory
	factory, err := initChequebookFactory(chaininfo.Backend, chaininfo.ChainID, chaininfo.TransactionService, chaininfo.Chainconfig.CurrentFactory.String())

	if err != nil {
		return nil, errors.New("init chequebook factory error")
	}

	//InitChequebookService
	chequebookService, err := initChequebookService(
		ctx,
		stateStore,
		chaininfo.Signer,
		chaininfo.ChainID,
		chaininfo.Backend,
		chaininfo.OverlayAddress,
		chaininfo.TransactionService,
		factory,
		initialDeposit,
		deployGasPrice,
	)

	if err != nil {
		return nil, errors.New("init chequebook service error")
	}

	//initChequeStoreCashout
	chequeStore, cashoutService := initChequeStoreCashout(
		stateStore,
		chaininfo.Backend,
		factory,
		chaininfo.ChainID,
		chaininfo.OverlayAddress,
		chaininfo.TransactionService,
	)

	//new accounting
	accounting, err := accounting.NewAccounting(stateStore)

	if err != nil {
		return nil, errors.New("new accounting service error")
	}

	//InitSwap
	swapService, priceOracleService, err := initSwap(
		stateStore,
		chainID,
		chaininfo.OverlayAddress,
		chequebookService,
		chequeStore,
		cashoutService,
		accounting,
		chaininfo.Chainconfig.PriceOracleAddress.String(),
		chaininfo.ChainID,
		chaininfo.TransactionService,
	)

	if err != nil {
		return nil, errors.New("init swap service error")
	}

	SettleObject = SettleInfo{
		Factory:           factory,
		ChequebookService: chequebookService,
		ChequeStore:       chequeStore,
		CashoutService:    cashoutService,
		SwapService:       swapService,
		OracleService:     priceOracleService,
	}

	return &SettleObject, nil
}

// InitChequebookFactory will initialize the chequebook factory with the given
// chain backend.
func initChequebookFactory(
	backend transaction.Backend,
	chainID int64,
	transactionService transaction.Service,
	factoryAddress string,
) (chequebook.Factory, error) {
	var currentFactory common.Address

	chainCfg, found := config.GetChainConfig(chainID)

	foundFactory := chainCfg.CurrentFactory
	if factoryAddress == "" {
		if !found {
			return nil, fmt.Errorf("no known factory address for this network (chain id: %d)", chainID)
		}
		currentFactory = foundFactory
		log.Infof("using default factory address for chain id %d: %x", chainID, currentFactory)
	} else if !common.IsHexAddress(factoryAddress) {
		return nil, errors.New("malformed factory address")
	} else {
		currentFactory = common.HexToAddress(factoryAddress)
		log.Infof("using custom factory address: %x", currentFactory)
	}

	return chequebook.NewFactory(
		backend,
		transactionService,
		currentFactory,
	), nil
}

// InitChequebookService will initialize the chequebook service with the given
// chequebook factory and chain backend.
func initChequebookService(
	ctx context.Context,
	stateStore storage.StateStorer,
	signer crypto.Signer,
	chainID int64,
	backend transaction.Backend,
	overlayEthAddress common.Address,
	transactionService transaction.Service,
	chequebookFactory chequebook.Factory,
	initialDeposit string,
	deployGasPrice string,
) (chequebook.Service, error) {
	chequeSigner := chequebook.NewChequeSigner(signer, chainID)

	deposit, ok := new(big.Int).SetString(initialDeposit, 10)
	if !ok {
		return nil, fmt.Errorf("initial swap deposit \"%s\" cannot be parsed", initialDeposit)
	}

	if deployGasPrice != "" {
		gasPrice, ok := new(big.Int).SetString(deployGasPrice, 10)
		if !ok {
			return nil, fmt.Errorf("deploy gas price \"%s\" cannot be parsed", deployGasPrice)
		}
		ctx = sctx.SetGasPrice(ctx, gasPrice)
	}

	chequebookService, err := chequebook.Init(
		ctx,
		chequebookFactory,
		stateStore,
		deposit,
		transactionService,
		backend,
		chainID,
		overlayEthAddress,
		chequeSigner,
	)
	if err != nil {
		return nil, fmt.Errorf("chequebook init: %w", err)
	}

	return chequebookService, nil
}

func initChequeStoreCashout(
	stateStore storage.StateStorer,
	swapBackend transaction.Backend,
	chequebookFactory chequebook.Factory,
	chainID int64,
	overlayEthAddress common.Address,
	transactionService transaction.Service,
) (chequebook.ChequeStore, chequebook.CashoutService) {
	chequeStore := chequebook.NewChequeStore(
		stateStore,
		chequebookFactory,
		chainID,
		overlayEthAddress,
		transactionService,
		chequebook.RecoverCheque,
	)

	cashout := chequebook.NewCashoutService(
		stateStore,
		swapBackend,
		transactionService,
		chequeStore,
	)

	return chequeStore, cashout
}

// InitSwap will initialize and register the swap service.
func initSwap(
	stateStore storage.StateStorer,
	networkID uint64,
	overlayEthAddress common.Address,
	chequebookService chequebook.Service,
	chequeStore chequebook.ChequeStore,
	cashoutService chequebook.CashoutService,
	accounting settlement.Accounting,
	priceOracleAddress string,
	chainID int64,
	transactionService transaction.Service,
) (*swap.Service, priceoracle.Service, error) {

	var currentPriceOracleAddress common.Address
	if priceOracleAddress == "" {
		chainCfg, found := config.GetChainConfig(chainID)
		currentPriceOracleAddress = chainCfg.PriceOracleAddress
		if !found {
			return nil, nil, errors.New("no known price oracle address for this network")
		}
	} else {
		currentPriceOracleAddress = common.HexToAddress(priceOracleAddress)
	}

	priceOracle := priceoracle.New(currentPriceOracleAddress, transactionService, 300)
	priceOracle.Start()
	swapProtocol := swapprotocol.New(overlayEthAddress, priceOracle)
	swapAddressBook := swap.NewAddressbook(stateStore)

	swapService := swap.New(
		swapProtocol,
		stateStore,
		chequebookService,
		chequeStore,
		swapAddressBook,
		networkID,
		cashoutService,
		accounting,
	)

	swapProtocol.SetSwap(swapService)
	swapprotocol.SwapProtocol = swapProtocol

	return swapService, priceOracle, nil
}

func GetTxHash(stateStore storage.StateStorer, trxString string) ([]byte, error) {

	if trxString != "" {
		txHashTrimmed := strings.TrimPrefix(trxString, "0x")
		if len(txHashTrimmed) != 64 {
			return nil, errors.New("invalid length")
		}
		txHash, err := hex.DecodeString(txHashTrimmed)
		if err != nil {
			return nil, err
		}
		log.Infof("using the provided transaction hash %x", txHash)
		return txHash, nil
	}

	var txHash common.Hash
	key := chequebook.ChequebookDeploymentKey
	if err := stateStore.Get(key, &txHash); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errors.New("chequebook deployment transaction hash not found, please specify the transaction hash manually")
		}
		return nil, err
	}

	log.Infof("using the chequebook transaction hash %x", txHash)
	return txHash.Bytes(), nil
}

func GetTxNextBlock(ctx context.Context, backend transaction.Backend, monitor transaction.Monitor, duration time.Duration, trx []byte, blockHash string) ([]byte, error) {

	if blockHash != "" {
		blockHashTrimmed := strings.TrimPrefix(blockHash, "0x")
		if len(blockHashTrimmed) != 64 {
			return nil, errors.New("invalid length")
		}
		blockHash, err := hex.DecodeString(blockHashTrimmed)
		if err != nil {
			return nil, err
		}
		log.Infof("using the provided block hash %x", blockHash)
		return blockHash, nil
	}

	// if not found in statestore, fetch from chain
	tx, err := backend.TransactionReceipt(ctx, common.BytesToHash(trx))
	if err != nil {
		return nil, err
	}

	block, err := transaction.WaitBlock(ctx, backend, duration, big.NewInt(0).Add(tx.BlockNumber, big.NewInt(1)))
	if err != nil {
		return nil, err
	}

	hash := block.Hash()
	hashBytes := hash.Bytes()

	log.Infof("using the next block hash from the blockchain %x", hashBytes)

	return hashBytes, nil
}
