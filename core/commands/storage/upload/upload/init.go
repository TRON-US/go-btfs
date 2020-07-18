package upload

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/challenge"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/escrow"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/tron-us/go-btfs-common/crypto"
	"github.com/tron-us/go-btfs-common/ledger"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"

	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v4"
	cidlib "github.com/ipfs/go-cid"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

const Gap = 1

type RecvContractParams struct {
	ssId                      string
	shardHash                 string
	shardIndex                int
	signedEscrowContractBytes []byte
	signedGuardContractBytes  []byte
}

var (
	isReplacedHost    bool
	isRegularContract bool
)

var StorageUploadInitCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Initialize storage handshake with inquiring client.",
		ShortDescription: `
Storage host opens this endpoint to accept incoming upload/storage requests,
If current host is interested and all validation checks out, host downloads
the shard and replies back to client for the next challenge step.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("file-hash", true, false, "Root file storage node should fetch (the DAG)."),
		cmds.StringArg("shard-hash", true, false, "Shard the storage node should fetch."),
		cmds.StringArg("price", true, false, "Per GiB per day in BTT for storing this shard offered by client."),
		cmds.StringArg("escrow-contract", true, false, "Client's initial escrow contract data."),
		cmds.StringArg("guard-contract-meta", true, false, "Client's initial guard contract meta."),
		cmds.StringArg("storage-length", true, false, "Store file for certain length in days."),
		cmds.StringArg("shard-size", true, false, "Size of each shard received in bytes."),
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
		settings, err := helper.GetHostStorageConfig(ctxParams.Ctx, ctxParams.N)
		if err != nil {
			return err
		}
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		if uint64(price) < settings.StoragePriceAsk {
			return fmt.Errorf("price invalid: want: >=%d, got: %d", settings.StoragePriceAsk, price)
		}
		requestPid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.N)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}
		storeLen, err := strconv.Atoi(req.Arguments[6])
		if err != nil {
			return err
		}
		if uint64(storeLen) < settings.StorageTimeMin {
			return fmt.Errorf("storage length invalid: want: >=%d, got: %d", settings.StorageTimeMin, storeLen)
		}
		ssId := req.Arguments[0]
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[8])
		if err != nil {
			return err
		}
		var halfSignedGuardContBytes []byte
		halfSignedGuardContString := req.Arguments[5]
		halfSignedGuardContBytes = []byte(halfSignedGuardContString)
		halfSignedGuardContract := &guardpb.Contract{}
		err = proto.Unmarshal(halfSignedGuardContBytes, halfSignedGuardContract)
		if err != nil {
			return err
		}
		guardContractMeta := halfSignedGuardContract.ContractMeta

		var peerId string
		if len(req.Arguments) >= 10 {
			peerId = req.Arguments[9]
		}

		var sourceId string
		if guardContractMeta.HostPid != "" {
			isRegularContract = true
			sourceId = halfSignedGuardContract.PreparerPid
		} else if peerId != ctxParams.N.Identity.Pretty() {
			isReplacedHost = true
			sourceId = peerId
		}
		s := halfSignedGuardContract.GetRenterSignature()
		if s == nil {
			s = halfSignedGuardContract.GetPreparerSignature()
		}
		if isRegularContract {
			if peerId == "" {
				pid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.N)
				if !ok {
					return fmt.Errorf("fail to get peer ID from request")
				}
				peerId = pid.String()
			}
		} else {
			peerId = guardContractMeta.RenterPid
		}
		payerPubKey, err := crypto.GetPubKeyFromPeerId(peerId)
		if err != nil {
			return err
		}
		ok, err = crypto.Verify(payerPubKey, &guardContractMeta, s)
		if !ok || err != nil {
			return fmt.Errorf("can't verify guard contract: %v", err)
		}

		storageLength := int(guardContractMeta.RentEnd.Sub(guardContractMeta.RentStart).Hours() / 24)
		if !isRegularContract {
			halfSignedGuardContract.HostPid = ctxParams.N.Identity.Pretty()
			if storeLen != storageLength {
				halfSignedGuardContract.RentEnd = halfSignedGuardContract.RentStart.Add(time.Duration(storeLen*24) * time.Hour)
			}
		}

		halfSignedEscrowContString := req.Arguments[4]
		var signedEscrowContractBytes []byte
		if halfSignedEscrowContString != "" {
			var halfSignedEscrowContBytes []byte
			halfSignedEscrowContBytes = []byte(halfSignedEscrowContString)
			halfSignedEscrowContract := &escrowpb.SignedEscrowContract{}
			err = proto.Unmarshal(halfSignedEscrowContBytes, halfSignedEscrowContract)
			if err != nil {
				return err
			}
			escrowContract := halfSignedEscrowContract.GetContract()
			ok, err = crypto.Verify(payerPubKey, escrowContract, halfSignedEscrowContract.GetBuyerSignature())
			if !ok || err != nil {
				return fmt.Errorf("can't verify escrow contract: %v", err)
			}
			// Verify price
			totalPay := uh.TotalPay(guardContractMeta.ShardFileSize, guardContractMeta.Price, storageLength)
			if escrowContract.Amount != guardContractMeta.Amount || totalPay != guardContractMeta.Amount {
				return errors.New("invalid contract")
			}
			// Sign on the contract
			signedEscrowContractBytes, err = signEscrowContractAndMarshal(escrowContract, halfSignedEscrowContract,
				ctxParams.N.PrivateKey)
			if err != nil {
				return err
			}
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
			var waitTime float64
			recvContract := &RecvContractParams{
				ssId:                      ssId,
				shardHash:                 shardHash,
				shardIndex:                shardIndex,
				signedEscrowContractBytes: signedEscrowContractBytes,
				signedGuardContractBytes:  signedGuardContractBytes,
			}
			if isRegularContract {
				err = downloadShardAndChallenge(requestPid, ctxParams, recvContract, sourceId, halfSignedGuardContract, signedGuardContract)
			} else {
				interval := signedGuardContract.RentStart.Sub(time.Now()).Hours() / 24
				if interval > Gap {
					waitTime = (interval - Gap) * 24 * 60 * 60
				}
				select {
				case <-time.After(time.Duration(waitTime) * time.Second):
					err = downloadShardAndChallenge(requestPid, ctxParams, recvContract, sourceId, halfSignedGuardContract, signedGuardContract)
				}
			}
			if err != nil {
				log.Error(err)
			}
		}()
		return cmds.EmitOnce(res, signedGuardContract)
	},
	Type: guardpb.Contract{},
}

func downloadShardAndChallenge(requestPid peer.ID, ctxParams *uh.ContextParams, param *RecvContractParams, sourceId string, halfSignedGuardContract *guardpb.Contract, signedGuardContract *guardpb.Contract) error {
	guardContractMeta := halfSignedGuardContract.ContractMeta
	shard, err := sessions.GetHostShard(ctxParams, signedGuardContract.ContractId)
	if err != nil {
		return err
	}
	_, err = remote.P2PCall(ctxParams.Ctx, ctxParams.N, requestPid, "/storage/upload/recvcontract",
		param.ssId,
		param.shardHash,
		param.shardIndex,
		param.signedEscrowContractBytes,
		param.signedGuardContractBytes,
	)
	if err != nil {
		return err
	}

	signedContractID, err := signContractID(signedGuardContract.ContractId, ctxParams.N.PrivateKey)
	if err != nil {
		return err
	}
	// check payment
	paidIn := make(chan bool)
	go checkPaymentFromClient(ctxParams, paidIn, signedContractID)
	paid := <-paidIn
	if !paid {
		return fmt.Errorf("contract is not paid: %s", signedGuardContract.ContractId)
	}
	tmp := new(guardpb.Contract)
	err = proto.Unmarshal(param.signedGuardContractBytes, tmp)
	if err != nil {
		return err
	}
	err = shard.Contract(param.signedEscrowContractBytes, tmp)
	if err != nil {
		return err
	}
	if isRegularContract || isReplacedHost {
		err = downloadShardFromClient(ctxParams, halfSignedGuardContract, guardContractMeta.FileHash, param.shardHash, sourceId)
		if err != nil {
			return err
		}
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
	// Need to renew another 5 mins due to downloading shard could have already made
	// req.Context obsolete
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
	if err = shard.Complete(); err != nil {
		return err
	}
	return nil
}

func signEscrowContractAndMarshal(contract *escrowpb.EscrowContract, signedContract *escrowpb.SignedEscrowContract,
	privKey ic.PrivKey) ([]byte, error) {
	sig, err := crypto.Sign(privKey, contract)
	if err != nil {
		return nil, err
	}
	if signedContract == nil {
		signedContract = escrow.NewSignedContract(contract)
	}
	signedContract.SellerSignature = sig
	signedBytes, err := proto.Marshal(signedContract)
	if err != nil {
		return nil, err
	}
	return signedBytes, nil
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

func signGuardContractAndMarshal(meta *guardpb.ContractMeta, cont *guardpb.Contract, privKey ic.PrivKey) ([]byte, error) {
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
	return proto.Marshal(cont)
}

// call escrow service to check if payment is received or not
func checkPaymentFromClient(ctxParams *uh.ContextParams, paidIn chan bool, contractID *escrowpb.SignedContractID) {
	var err error
	paid := false
	err = backoff.Retry(func() error {
		paid, err = isPaidin(ctxParams, contractID)
		if err != nil {
			return err
		}
		if paid {
			paidIn <- true
			return nil
		}
		return errors.New("reach max retry times")
	}, uh.CheckPaymentBo)
	if err != nil {
		log.Error("Check escrow IsPaidin failed", err)
		paidIn <- paid
	}
}

func downloadShardFromClient(ctxParams *uh.ContextParams, guardContract *guardpb.Contract, fileHash string,
	shardHash string, sourceId string) error {

	// Get + pin to make sure it does not get accidentally deleted
	// Sharded scheme as special pin logic to add
	// file root dag + shard root dag + metadata full dag + only this shard dag
	fileCid, err := cidlib.Parse(fileHash)
	if err != nil {
		return err
	}
	shardCid, err := cidlib.Parse(shardHash)
	if err != nil {
		return err
	}
	// Need to compute a time to download shard that's fair for small vs large files
	low := 30 * time.Second
	high := 5 * time.Minute
	scaled := time.Duration(float64(guardContract.ShardFileSize) / float64(units.GiB) * float64(high))
	if scaled < low {
		scaled = low
	} else if scaled > high {
		scaled = high
	}
	// Also need to account for renter going up and down, to give an overall retry time limit
	lowRetry := 30 * time.Minute
	highRetry := 24 * time.Hour
	scaledRetry := time.Duration(float64(guardContract.ShardFileSize) / float64(units.GiB) * float64(highRetry))
	if scaledRetry < lowRetry {
		scaledRetry = lowRetry
	} else if scaledRetry > highRetry {
		scaledRetry = highRetry
	}
	expir := uint64(guardContract.RentEnd.Unix())
	sourcePid, err := peer.IDB58Decode(sourceId)
	if err != nil {
		return nil
	}
	// based on small and large file sizes
	err = backoff.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), scaled)
		defer cancel()

		go func() {
			for {
				select {
				case <-ctx.Done():
					break
				}
				swarmCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				err := ctxParams.Api.Swarm().Connect(swarmCtx, peer.AddrInfo{ID: sourcePid})
				if err == nil {
					return
				}
			}
		}()

		_, err = challenge.NewStorageChallengeResponse(ctx, ctxParams.N, ctxParams.Api, fileCid, shardCid, "", true, expir)
		return err
	}, uh.DownloadShardBo(scaledRetry))

	if err != nil {
		return fmt.Errorf("failed to download shard %s from file %s with contract id %s: [%v]",
			guardContract.ShardHash, guardContract.FileHash, guardContract.ContractId, err)
	}
	return nil
}

func isPaidin(ctxParams *uh.ContextParams, contractID *escrowpb.SignedContractID) (bool, error) {
	var signedPayinRes *escrowpb.SignedPayinStatus
	ctx, _ := helper.NewGoContext(ctxParams.Ctx)
	err := grpc.EscrowClient(ctxParams.Cfg.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.IsPaid(ctx, contractID)
			if err != nil {
				return err
			}
			err = escrow.VerifyEscrowRes(ctxParams.Cfg, res.Status, res.EscrowSignature)
			if err != nil {
				return err
			}
			signedPayinRes = res
			return nil
		})
	if err != nil {
		return false, err
	}
	return signedPayinRes.Status.Paid, nil
}

func signContractID(id string, privKey ic.PrivKey) (*escrowpb.SignedContractID, error) {
	contractID, err := ledger.NewContractID(id, privKey.GetPublic())
	if err != nil {
		return nil, err
	}
	// sign contractID
	sig, err := crypto.Sign(privKey, contractID)
	if err != nil {
		return nil, err
	}
	return ledger.NewSingedContractID(contractID, sig), nil
}
