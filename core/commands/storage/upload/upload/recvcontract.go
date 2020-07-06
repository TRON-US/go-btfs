package upload

import (
	"errors"
	"strconv"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	"github.com/gogo/protobuf/proto"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

var StorageUploadRecvContractCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "For renter client to receive half signed contracts.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "Session ID which renter uses to storage all shards information."),
		cmds.StringArg("shard-hash", true, false, "Shard the storage node should fetch."),
		cmds.StringArg("shard-index", true, false, "Index of shard within the encoding scheme."),
		cmds.StringArg("escrow-contract", true, false, "Signed Escrow contract."),
		cmds.StringArg("guard-contract", true, false, "Signed Guard contract."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		contractId, err := doRecv(req, env)
		if contractId != "" {
			if ch, ok := ShardErrChanMap.Get(contractId); ok {
				go func() {
					ch.(chan error) <- err
				}()
			}
			if err != nil {
				return err
			}
		}
		return nil
	},
}

func doRecv(req *cmds.Request, env cmds.Environment) (contractId string, err error) {
	ssID := req.Arguments[0]
	ctxParams, err := helper.ExtractContextParams(req, env)
	if err != nil {
		return
	}
	requestPid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.N)
	if !ok {
		err = errors.New("failed to get remote peer id")
		return
	}
	rpk, err := requestPid.ExtractPublicKey()
	if err != nil {
		return
	}

	escrowContractBytes := []byte(req.Arguments[3])
	guardContractBytes := []byte(req.Arguments[4])
	guardContract := new(guardpb.Contract)
	err = proto.Unmarshal(guardContractBytes, guardContract)
	if err != nil {
		return
	}
	bytes, err := proto.Marshal(&guardContract.ContractMeta)
	if err != nil {
		return
	}
	valid, err := rpk.Verify(bytes, guardContract.HostSignature)
	if err != nil {
		return
	}
	if !valid || guardContract.ContractMeta.GetHostPid() != requestPid.Pretty() {
		err = errors.New("invalid guard contract bytes")
		return
	}
	contractId = guardContract.ContractMeta.ContractId

	shardHash := req.Arguments[1]
	index, err := strconv.Atoi(req.Arguments[2])
	if err != nil {
		return
	}
	shard, err := sessions.GetRenterShard(ctxParams, ssID, shardHash, index)
	if err != nil {
		return
	}
	if len(escrowContractBytes) == 0 {
		hostPid, err := peer.IDB58Decode(guardContract.HostPid)
		if err != nil {
			return contractId, err
		}
		hostPubKey, err := hostPid.ExtractPublicKey()
		if err != nil {
			return contractId, err
		}
		hostAddr, err := ic.RawFull(hostPubKey)
		if err != nil {
			return contractId, err
		}
		signedContracts, err := shard.Contracts()
		if err != nil {
			return contractId, err
		}
		signedEscrowContract := &escrowpb.SignedEscrowContract{}
		err = proto.Unmarshal(signedContracts.SignedEscrowContract, signedEscrowContract)
		if err != nil {
			return contractId, err
		}
		signedEscrowContract.Contract.SellerAddress = hostAddr
		SignedEscrowContractBytes, err := proto.Marshal(signedEscrowContract)
		if err != nil {
			return contractId, err
		}
		escrowContractBytes = SignedEscrowContractBytes
	}
	_ = shard.Contract(escrowContractBytes, guardContract)
	return
}
