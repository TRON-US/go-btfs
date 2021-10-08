package config

import (
	"github.com/ethereum/go-ethereum/common"
)

var (
	// chain ID
	ethChainID  = int64(5)
	tronChainID = int64(100)
	btccChainID = int64(100)
	testChainID = int64(1337)
	// start block
	ethStartBlock = uint64(10000)

	tronStartBlock = uint64(4933174)
	bttcStartBlock = uint64(4933174)
	// factory address
	ethFactoryAddress = common.HexToAddress("0x5E6802d9e7C8CD43BB7C96524fDD50FE8460B92c")
	ethOracleAddress  = common.HexToAddress("0xFB6a65aF1bb250EAf3f58C420912B0b6eA05Ea7a")

	tronFactoryAddress = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")
	tronOracleAddress  = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")

	bttcFactoryAddress = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")
	bttcOracleAddress  = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")

	// initailDeposit
	ethInitialDeposit  = "100000000000000"
	tronInitialDeposit = "100"
	bttcInitialDeposit = "100"
	testInitialDeposit = "100"

	// deploy gas
	ethDeploymentGas  = "10"
	tronDeploymentGas = "10"
	bttcDeploymentGas = "10"
	testDeploymentGas = "10"
)

type ChainConfig struct {
	StartBlock         uint64
	CurrentFactory     common.Address
	PriceOracleAddress common.Address
	InitailDeposit     string
	DeploymentGas      string
}

func GetChainConfig(chainID int64) (*ChainConfig, bool) {
	var cfg ChainConfig
	switch chainID {
	case ethChainID:
		cfg.StartBlock = ethStartBlock
		cfg.CurrentFactory = ethFactoryAddress
		cfg.PriceOracleAddress = ethOracleAddress
		cfg.DeploymentGas = ethDeploymentGas
		cfg.InitailDeposit = ethInitialDeposit
		return &cfg, true
	case tronChainID:
		cfg.StartBlock = tronStartBlock
		cfg.CurrentFactory = tronFactoryAddress
		cfg.PriceOracleAddress = tronOracleAddress
		cfg.DeploymentGas = tronDeploymentGas
		cfg.InitailDeposit = tronInitialDeposit
		return &cfg, true
	case btccChainID:
		cfg.StartBlock = bttcStartBlock
		cfg.CurrentFactory = bttcFactoryAddress
		cfg.PriceOracleAddress = bttcOracleAddress
		cfg.DeploymentGas = bttcDeploymentGas
		cfg.InitailDeposit = bttcInitialDeposit
		return &cfg, true
	case testChainID:
		cfg.StartBlock = ethStartBlock
		cfg.CurrentFactory = ethFactoryAddress
		cfg.PriceOracleAddress = ethOracleAddress
		cfg.DeploymentGas = testDeploymentGas
		cfg.InitailDeposit = testInitialDeposit
		return &cfg, true

	default:
		return &cfg, false
	}
}
