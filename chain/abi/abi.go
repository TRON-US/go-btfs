package abi

const ERC20SimpleSwapABI = `[
		{
			"anonymous": false,
			"inputs": [],
			"name": "ChequeBounced",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": true,
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"indexed": true,
					"internalType": "address",
					"name": "recipient",
					"type": "address"
				},
				{
					"indexed": true,
					"internalType": "address",
					"name": "caller",
					"type": "address"
				},
				{
					"indexed": false,
					"internalType": "uint256",
					"name": "totalPayout",
					"type": "uint256"
				},
				{
					"indexed": false,
					"internalType": "uint256",
					"name": "cumulativePayout",
					"type": "uint256"
				},
				{
					"indexed": false,
					"internalType": "uint256",
					"name": "callerPayout",
					"type": "uint256"
				}
			],
			"name": "ChequeCashed",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": true,
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"indexed": false,
					"internalType": "uint256",
					"name": "amount",
					"type": "uint256"
				}
			],
			"name": "HardDepositAmountChanged",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": true,
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"indexed": false,
					"internalType": "uint256",
					"name": "decreaseAmount",
					"type": "uint256"
				}
			],
			"name": "HardDepositDecreasePrepared",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": true,
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"indexed": false,
					"internalType": "uint256",
					"name": "timeout",
					"type": "uint256"
				}
			],
			"name": "HardDepositTimeoutChanged",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [],
			"name": "PreWithdraw",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": false,
					"internalType": "uint256",
					"name": "amount",
					"type": "uint256"
				}
			],
			"name": "Withdraw",
			"type": "event"
		},
		{
			"inputs": [],
			"name": "CASHOUT_TYPEHASH",
			"outputs": [
				{
					"internalType": "bytes32",
					"name": "",
					"type": "bytes32"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "CHEQUE_TYPEHASH",
			"outputs": [
				{
					"internalType": "bytes32",
					"name": "",
					"type": "bytes32"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "CUSTOMDECREASETIMEOUT_TYPEHASH",
			"outputs": [
				{
					"internalType": "bytes32",
					"name": "",
					"type": "bytes32"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "EIP712DOMAIN_TYPEHASH",
			"outputs": [
				{
					"internalType": "bytes32",
					"name": "",
					"type": "bytes32"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "balance",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "bounced",
			"outputs": [
				{
					"internalType": "bool",
					"name": "",
					"type": "bool"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"internalType": "address",
					"name": "recipient",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "cumulativePayout",
					"type": "uint256"
				},
				{
					"internalType": "bytes",
					"name": "beneficiarySig",
					"type": "bytes"
				},
				{
					"internalType": "uint256",
					"name": "callerPayout",
					"type": "uint256"
				},
				{
					"internalType": "bytes",
					"name": "issuerSig",
					"type": "bytes"
				}
			],
			"name": "cashCheque",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "recipient",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "cumulativePayout",
					"type": "uint256"
				},
				{
					"internalType": "bytes",
					"name": "issuerSig",
					"type": "bytes"
				}
			],
			"name": "cashChequeBeneficiary",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				}
			],
			"name": "decreaseHardDeposit",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "defaultHardDepositTimeout",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"name": "hardDeposits",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "amount",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "decreaseAmount",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "timeout",
					"type": "uint256"
				},
				{
					"internalType": "uint256",
					"name": "canBeDecreasedAt",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "amount",
					"type": "uint256"
				}
			],
			"name": "increaseHardDeposit",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_issuer",
					"type": "address"
				},
				{
					"internalType": "address",
					"name": "_token",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "_defaultHardDepositTimeout",
					"type": "uint256"
				}
			],
			"name": "init",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "issuer",
			"outputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "liquidBalance",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				}
			],
			"name": "liquidBalanceFor",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"name": "paidOut",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "preWithdraw",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "decreaseAmount",
					"type": "uint256"
				}
			],
			"name": "prepareDecreaseHardDeposit",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "beneficiary",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "hardDepositTimeout",
					"type": "uint256"
				},
				{
					"internalType": "bytes",
					"name": "beneficiarySig",
					"type": "bytes"
				}
			],
			"name": "setCustomHardDepositTimeout",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "token",
			"outputs": [
				{
					"internalType": "contract ERC20",
					"name": "",
					"type": "address"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "totalHardDeposit",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "totalPaidOut",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "uint256",
					"name": "amount",
					"type": "uint256"
				}
			],
			"name": "withdraw",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "withdrawTime",
			"outputs": [
				{
					"internalType": "uint256",
					"name": "",
					"type": "uint256"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

const SimpleSwapFactoryABI = `[
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_ERC20Address",
					"type": "address"
				}
			],
			"stateMutability": "nonpayable",
			"type": "constructor"
		},
		{
			"anonymous": false,
			"inputs": [
				{
					"indexed": false,
					"internalType": "address",
					"name": "contractAddress",
					"type": "address"
				}
			],
			"name": "SimpleSwapDeployed",
			"type": "event"
		},
		{
			"inputs": [],
			"name": "ERC20Address",
			"outputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "issuer",
					"type": "address"
				},
				{
					"internalType": "uint256",
					"name": "defaultHardDepositTimeoutDuration",
					"type": "uint256"
				},
				{
					"internalType": "bytes32",
					"name": "salt",
					"type": "bytes32"
				}
			],
			"name": "deploySimpleSwap",
			"outputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"name": "deployedContracts",
			"outputs": [
				{
					"internalType": "bool",
					"name": "",
					"type": "bool"
				}
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "master",
			"outputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

const Erc20ABI = `[
			{
				"inputs": [
					{
						"internalType": "string",
						"name": "name_",
						"type": "string"
					},
					{
						"internalType": "string",
						"name": "symbol_",
						"type": "string"
					}
				],
				"stateMutability": "nonpayable",
				"type": "constructor"
			},
			{
				"anonymous": false,
				"inputs": [
					{
						"indexed": true,
						"internalType": "address",
						"name": "owner",
						"type": "address"
					},
					{
						"indexed": true,
						"internalType": "address",
						"name": "spender",
						"type": "address"
					},
					{
						"indexed": false,
						"internalType": "uint256",
						"name": "value",
						"type": "uint256"
					}
				],
				"name": "Approval",
				"type": "event"
			},
			{
				"anonymous": false,
				"inputs": [
					{
						"indexed": true,
						"internalType": "address",
						"name": "from",
						"type": "address"
					},
					{
						"indexed": true,
						"internalType": "address",
						"name": "to",
						"type": "address"
					},
					{
						"indexed": false,
						"internalType": "uint256",
						"name": "value",
						"type": "uint256"
					}
				],
				"name": "Transfer",
				"type": "event"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "owner",
						"type": "address"
					},
					{
						"internalType": "address",
						"name": "spender",
						"type": "address"
					}
				],
				"name": "allowance",
				"outputs": [
					{
						"internalType": "uint256",
						"name": "",
						"type": "uint256"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "spender",
						"type": "address"
					},
					{
						"internalType": "uint256",
						"name": "amount",
						"type": "uint256"
					}
				],
				"name": "approve",
				"outputs": [
					{
						"internalType": "bool",
						"name": "",
						"type": "bool"
					}
				],
				"stateMutability": "nonpayable",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "account",
						"type": "address"
					}
				],
				"name": "balanceOf",
				"outputs": [
					{
						"internalType": "uint256",
						"name": "",
						"type": "uint256"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [],
				"name": "decimals",
				"outputs": [
					{
						"internalType": "uint8",
						"name": "",
						"type": "uint8"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "spender",
						"type": "address"
					},
					{
						"internalType": "uint256",
						"name": "subtractedValue",
						"type": "uint256"
					}
				],
				"name": "decreaseAllowance",
				"outputs": [
					{
						"internalType": "bool",
						"name": "",
						"type": "bool"
					}
				],
				"stateMutability": "nonpayable",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "spender",
						"type": "address"
					},
					{
						"internalType": "uint256",
						"name": "addedValue",
						"type": "uint256"
					}
				],
				"name": "increaseAllowance",
				"outputs": [
					{
						"internalType": "bool",
						"name": "",
						"type": "bool"
					}
				],
				"stateMutability": "nonpayable",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "account",
						"type": "address"
					},
					{
						"internalType": "uint256",
						"name": "amount",
						"type": "uint256"
					}
				],
				"name": "mint",
				"outputs": [],
				"stateMutability": "nonpayable",
				"type": "function"
			},
			{
				"inputs": [],
				"name": "name",
				"outputs": [
					{
						"internalType": "string",
						"name": "",
						"type": "string"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [],
				"name": "symbol",
				"outputs": [
					{
						"internalType": "string",
						"name": "",
						"type": "string"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [],
				"name": "totalSupply",
				"outputs": [
					{
						"internalType": "uint256",
						"name": "",
						"type": "uint256"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "recipient",
						"type": "address"
					},
					{
						"internalType": "uint256",
						"name": "amount",
						"type": "uint256"
					}
				],
				"name": "transfer",
				"outputs": [
					{
						"internalType": "bool",
						"name": "",
						"type": "bool"
					}
				],
				"stateMutability": "nonpayable",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "sender",
						"type": "address"
					},
					{
						"internalType": "address",
						"name": "recipient",
						"type": "address"
					},
					{
						"internalType": "uint256",
						"name": "amount",
						"type": "uint256"
					}
				],
				"name": "transferFrom",
				"outputs": [
					{
						"internalType": "bool",
						"name": "",
						"type": "bool"
					}
				],
				"stateMutability": "nonpayable",
				"type": "function"
			}
		]`
const ProxyAbi = `[
			{
				"constant": true,
				"inputs": [],
				"name": "masterCopy",
				"outputs": [
					{
						"internalType": "address",
						"name": "",
						"type": "address"
					}
				],
				"payable": false,
				"stateMutability": "view",
				"type": "function"
			}
		]`

const OracleAbi = `[
			{
				"inputs": [
					{
						"internalType": "uint256",
						"name": "_price",
						"type": "uint256"
					}
				],
				"stateMutability": "nonpayable",
				"type": "constructor"
			},
			{
				"anonymous": false,
				"inputs": [
					{
						"indexed": true,
						"internalType": "address",
						"name": "previousOwner",
						"type": "address"
					},
					{
						"indexed": true,
						"internalType": "address",
						"name": "newOwner",
						"type": "address"
					}
				],
				"name": "OwnershipTransferred",
				"type": "event"
			},
			{
				"anonymous": false,
				"inputs": [
					{
						"indexed": false,
						"internalType": "uint256",
						"name": "price",
						"type": "uint256"
					}
				],
				"name": "PriceUpdate",
				"type": "event"
			},
			{
				"inputs": [],
				"name": "getPrice",
				"outputs": [
					{
						"internalType": "uint256",
						"name": "",
						"type": "uint256"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [],
				"name": "owner",
				"outputs": [
					{
						"internalType": "address",
						"name": "",
						"type": "address"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [],
				"name": "price",
				"outputs": [
					{
						"internalType": "uint256",
						"name": "",
						"type": "uint256"
					}
				],
				"stateMutability": "view",
				"type": "function"
			},
			{
				"inputs": [],
				"name": "renounceOwnership",
				"outputs": [],
				"stateMutability": "nonpayable",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "address",
						"name": "newOwner",
						"type": "address"
					}
				],
				"name": "transferOwnership",
				"outputs": [],
				"stateMutability": "nonpayable",
				"type": "function"
			},
			{
				"inputs": [
					{
						"internalType": "uint256",
						"name": "newPrice",
						"type": "uint256"
					}
				],
				"name": "updatePrice",
				"outputs": [],
				"stateMutability": "nonpayable",
				"type": "function"
			}
		]`
const FactoryDeployedBin = "608060405234801561001057600080fd5b506004361061004c5760003560e01c806315efd8a714610051578063a6021ace14610081578063c70242ad1461009f578063ee97f7f3146100cf575b600080fd5b61006b6004803603810190610066919061044e565b6100ed565b60405161007891906104e8565b60405180910390f35b610089610270565b60405161009691906104e8565b60405180910390f35b6100b960048036038101906100b49190610425565b610296565b6040516100c69190610563565b60405180910390f35b6100d76102b6565b6040516100e491906104e8565b60405180910390f35b600080610144600260009054906101000a900473ffffffffffffffffffffffffffffffffffffffff163385604051602001610129929190610503565b604051602081830303815290604052805190602001206102dc565b90508073ffffffffffffffffffffffffffffffffffffffff166386863ec686600160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16876040518463ffffffff1660e01b81526004016101a59392919061052c565b600060405180830381600087803b1580156101bf57600080fd5b505af11580156101d3573d6000803e3d6000fd5b5050505060016000808373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff0219169083151502179055507fc0ffc525a1c7689549d7f79b49eca900e61ac49b43d977f680bcc3b36224c0048160405161025d91906104e8565b60405180910390a1809150509392505050565b600160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b60006020528060005260406000206000915054906101000a900460ff1681565b600260009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b60006040517f3d602d80600a3d3981f3363d3d373d3d3d363d7300000000000000000000000081528360601b60148201527f5af43d82803e903d91602b57fd5bf300000000000000000000000000000000006028820152826037826000f5915050600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff1614156103e0576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260178152602001807f455243313136373a2063726561746532206661696c656400000000000000000081525060200191505060405180910390fd5b92915050565b6000813590506103f5816105e2565b92915050565b60008135905061040a816105f9565b92915050565b60008135905061041f81610610565b92915050565b60006020828403121561043757600080fd5b6000610445848285016103e6565b91505092915050565b60008060006060848603121561046357600080fd5b6000610471868287016103e6565b935050602061048286828701610410565b9250506040610493868287016103fb565b9150509250925092565b6104a681610590565b82525050565b6104b58161057e565b82525050565b6104c4816105a2565b82525050565b6104d3816105ae565b82525050565b6104e2816105d8565b82525050565b60006020820190506104fd60008301846104ac565b92915050565b6000604082019050610518600083018561049d565b61052560208301846104ca565b9392505050565b600060608201905061054160008301866104ac565b61054e60208301856104ac565b61055b60408301846104d9565b949350505050565b600060208201905061057860008301846104bb565b92915050565b6000610589826105b8565b9050919050565b600061059b826105b8565b9050919050565b60008115159050919050565b6000819050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b6105eb8161057e565b81146105f657600080fd5b50565b610602816105ae565b811461060d57600080fd5b50565b610619816105d8565b811461062457600080fd5b5056fea264697066735822122047e94ba952f42f7890a9455a80e709746635e563d3ada3158c43e5d23c08018064736f6c63430007060033"
