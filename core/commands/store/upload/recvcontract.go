package upload

import (
	"errors"
	"fmt"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	"github.com/gogo/protobuf/proto"
)

var StorageUploadRecvContractCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "For renter client to receive half signed contracts.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "Session ID which renter uses to store all shards information."),
		cmds.StringArg("shard-hash", true, false, "Shard the storage node should fetch."),
		cmds.StringArg("shard-index", true, false, "Index of shard within the encoding scheme."),
		cmds.StringArg("escrow-contract", true, false, "Signed Escrow contract."),
		cmds.StringArg("guard-contract", true, false, "Signed Guard contract."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		contractId, err := doRecv(req, env)
		fmt.Println("contractId", contractId, "err", err)
		if contractId != "" {
			if ch, ok := shardErrChanMap.Get(contractId); ok {
				go func() {
					fmt.Println("write recv err", req.Arguments[1], err)
					ch.(chan error) <- err
					fmt.Println("done write recv err", req.Arguments[1], err)
				}()
			}
			return err
		}
		return nil
	},
}

func doRecv(req *cmds.Request, env cmds.Environment) (contractId string, err error) {
	ssID := req.Arguments[0]
	ctxParams, err := extractContextParams(req, env)
	if err != nil {
		return
	}
	requestPid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.n)
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
	fmt.Println("recv guardContract preparerPid:", guardContract.PreparerPid)
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
	shard, err := GetRenterShard(ctxParams, ssID, shardHash)
	if err != nil {
		return
	}
	err = shard.contract(escrowContractBytes, guardContract)
	if err != nil {
		return
	}
	return
}
