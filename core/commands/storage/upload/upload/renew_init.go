package upload

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"strconv"
	"time"

	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"

	ic "github.com/libp2p/go-libp2p-core/crypto"
)

var StorageRenewInitCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Host returns the signed contract.",
		ShortDescription: `
Storage host agrees with the extended duration and signs the new contract, 
finally returns the signed contract.
`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("file-hash", true, false, "Root file storage node should fetch (the DAG)."), // could be used for download shards
		cmds.StringArg("shard-hash", true, false, "Shard the storage node should fetch."),
		cmds.StringArg("guard-contract-meta", true, false, "Client's initial guard contract meta."),
		cmds.StringArg("storage-length", true, false, "Store file for certain length in days."), //6
		cmds.StringArg("shard-index", true, false, "Index of shard within the encoding scheme."),
		cmds.StringArg("upload-peer-id", false, false, "Peer id when upload sign is used."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ctxParams, err := uh.ExtractContextParams(req, env)
		if err != nil {
			return err
		}
		if !ctxParams.Cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}
		requestPid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.N)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}
		storageLength, err := strconv.Atoi(req.Arguments[4])
		if err != nil {
			return err
		}

		settings, err := helper.GetHostStorageConfig(ctxParams.Ctx, ctxParams.N)
		if err != nil {
			return err
		}
		if uint64(storageLength) < settings.StorageTimeMin {
			return fmt.Errorf("storage length invalid: want: >=%d, got: %d", settings.StorageTimeMin, storageLength)
		}
		ssId := req.Arguments[0]
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[5])
		if err != nil {
			return err
		}
		halfSignedGuardContString := req.Arguments[3]
		var halfSignedGuardContBytes []byte
		halfSignedGuardContBytes = []byte(halfSignedGuardContString)
		halfSignedGuardContract := &guardpb.Contract{}
		err = proto.Unmarshal(halfSignedGuardContBytes, halfSignedGuardContract)
		if err != nil {
			return err
		}
		guardContractMeta := halfSignedGuardContract.ContractMeta
		pid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.N)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}
		var peerId string
		if peerId = pid.String(); len(req.Arguments) >= 7 {
			peerId = req.Arguments[6]
		}
		payerPubKey, err := crypto.GetPubKeyFromPeerId(peerId)
		if err != nil {
			return err
		}
		s := halfSignedGuardContract.GetRenterSignature()
		if s == nil {
			s = halfSignedGuardContract.GetPreparerSignature()
		}
		ok, err = crypto.Verify(payerPubKey, &guardContractMeta, s)
		if !ok || err != nil {
			return fmt.Errorf("can't verify guard contract: %v", err)
		}
		signedGuardContract, err := signGuardContract(&guardContractMeta, halfSignedGuardContract, ctxParams.N.PrivateKey)
		if err != nil {
			return err
		}
		signedGuardContractBytes, err := proto.Marshal(signedGuardContract)
		if err != nil {
			return err
		}
		go func() {
			tmp := func() error {
				shard, err := sessions.GetHostShard(ctxParams, signedGuardContract.ContractId)
				if err != nil {
					return err
				}
				_, err = remote.P2PCall(ctxParams.Ctx, ctxParams.N, requestPid, "/storage/upload/recvcontract",
					ssId,
					shardHash,
					shardIndex,
					nil,
					signedGuardContractBytes,
				)
				if err != nil {
					return err
				}

				err = shard.Contract(nil, signedGuardContract)
				if err != nil {
					return err
				}
				in := &guardpb.ReadyForChallengeRequest{
					RenterPid:   guardContractMeta.RenterPid,
					FileHash:    guardContractMeta.FileHash,
					ShardHash:   guardContractMeta.ShardHash,
					ContractId:  guardContractMeta.ContractId,
					HostPid:     guardContractMeta.HostPid,
					PrepareTime: guardContractMeta.RentStart,
				}
				sign, err := crypto.Sign(ctxParams.N.PrivateKey, in)
				if err != nil {
					return err
				}
				in.Signature = sign
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctx,
					func(ctx context.Context, client guardpb.GuardServiceClient) error {
						_, err = client.ReadyForChallenge(ctx, in)
						if err != nil {
							return err
						}
						return nil
					})
				if err != nil {
					return err
				}
				if err := shard.Complete(); err != nil {
					return err
				}
				return nil
			}()
			if tmp != nil {
				log.Error(tmp)
			}
		}()
		return cmds.EmitOnce(res, signedGuardContract)
	},
	Type: guardpb.Contract{},
}

func signGuardContract(meta *guardpb.ContractMeta, cont *guardpb.Contract, privKey ic.PrivKey) (*guardpb.Contract, error) {
	signedBytes, err := crypto.Sign(privKey, meta)
	if err != nil {
		return nil, err
	}
	if cont == nil {
		cont = &guardpb.Contract{
			ContractMeta:   *meta,
			LastModifyTime: time.Now(),
		}
	} else {
		cont.LastModifyTime = time.Now()
	}
	cont.HostSignature = signedBytes
	return cont, err
}
