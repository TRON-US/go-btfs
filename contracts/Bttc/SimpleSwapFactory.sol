// SPDX-License-Identifier: BSD-3-Clause
pragma solidity =0.7.6;
pragma abicoder v2;
import "./SimpleSwap.sol";
import "https://github.com/OpenZeppelin/openzeppelin-contracts/blob/release-v3.4/contracts/proxy/Clones.sol";

/**
@title Factory contract for SimpleSwap
@author The Swarm Authors
@notice This contract deploys SimpleSwap contracts
*/
contract SimpleSwapFactory {

  /* event fired on every new SimpleSwap deployment */
  event SimpleSwapDeployed(address contractAddress);

  /* mapping to keep track of which contracts were deployed by this factory */
  mapping (address => bool) public deployedContracts;

  /* address of the TRC20-token, to be used by the to-be-deployed chequebooks */
  address public TokenAddress;
  /* address of the code contract from which all chequebooks are cloned */
  address public master;

  constructor(address _TokenAddress) {
    TokenAddress = _TokenAddress;
    SimpleSwap _master = new SimpleSwap();
    // set the issuer of the master contract to prevent misuse
    _master.init(address(1), address(0));
    master = address(_master);
  }
  /**
  @notice creates a clone of the master SimpleSwap contract
  @param issuer the issuer of cheques for the new chequebook
  @param salt salt to include in create2 to enable the same address to deploy multiple chequebooks
  */
  function deploySimpleSwap(address issuer, bytes32 salt)
  public returns (address) {    
    address contractAddress = Clones.cloneDeterministic(master, keccak256(abi.encode(msg.sender, salt)));
    SimpleSwap(contractAddress).init(issuer, TokenAddress);
    deployedContracts[contractAddress] = true;
    emit SimpleSwapDeployed(contractAddress);
    return contractAddress;
  }
}
