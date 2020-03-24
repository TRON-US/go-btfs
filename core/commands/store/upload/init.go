package upload

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/commands/store/upload/ds"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/guard"
	shardpb "github.com/TRON-US/go-btfs/protos/shard"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v3"
	"github.com/dustin/go-humanize"
	cidlib "github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/prometheus/common/log"
)

var checkPaymentBo = func() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 10 * time.Second
	bo.MaxElapsedTime = 5 * time.Minute
	bo.Multiplier = 1.5
	bo.MaxInterval = 60 * time.Second
	return bo
}()

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
		cmds.StringArg("offline-peer-id", false, false, "Peer id when offline sign is used."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// check flags
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		var (
			requestPid peer.ID
			ok         bool
		)

		ssID := req.Arguments[0]
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[8])
		if err != nil {
			return err
		}
		shardSize, err := strconv.ParseInt(req.Arguments[7], 10, 64)
		if err != nil {
			return err
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		sh, err := ds.GetShard(n.Identity.Pretty(), nodepb.ContractStat_HOST.String(), ssID, shardHash,
			&ds.ShardInitParams{
				Context:   req.Context,
				Datastore: n.Repo.Datastore(),
			})
		if err != nil {
			return err
		}
		// Check existing storage has enough left
		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}
		max, err := storage.CheckAndValidateHostStorageMax(cfgRoot, n.Repo, nil, true)
		if err != nil {
			return err
		}
		su, err := n.Repo.GetStorageUsage()
		if err != nil {
			return err
		}
		actualLeft := max - su
		if uint64(shardSize) > actualLeft {
			return fmt.Errorf("storage not enough: needs %s but only %s left",
				humanize.Bytes(uint64(shardSize)), humanize.Bytes(actualLeft))
		}

		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		halfSignedEscrowContString := req.Arguments[4]
		halfSignedGuardContString := req.Arguments[5]
		var halfSignedEscrowContBytes, halfSignedGuardContBytes []byte
		halfSignedEscrowContBytes = []byte(halfSignedEscrowContString)
		halfSignedGuardContBytes = []byte(halfSignedGuardContString)

		settings, err := storage.GetHostStorageConfig(req.Context, n)
		if err != nil {
			return err
		}
		if uint64(price) < settings.StoragePriceAsk {
			return fmt.Errorf("price invalid: want: >=%d, got: %d", settings.StoragePriceAsk, price)
		}
		requestPid, ok = remote.GetStreamRequestRemotePeerID(req, n)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}
		storeLen, err := strconv.Atoi(req.Arguments[6])
		if err != nil {
			return err
		}
		if uint64(storeLen) < settings.StorageTimeMin {
			return fmt.Errorf("store length invalid: want: >=%d, got: %d", settings.StorageTimeMin, storeLen)
		}
		halfSignedGuardContract, err := guard.UnmarshalGuardContract(halfSignedGuardContBytes)
		if err != nil {
			return err
		}
		// review contract and send back to client
		halfSignedEscrowContract, err := escrow.UnmarshalEscrowContract(halfSignedEscrowContBytes)
		if err != nil {
			return err
		}
		escrowContract := halfSignedEscrowContract.GetContract()
		guardContractMeta := halfSignedGuardContract.ContractMeta
		// get renter's public key
		payerPubKey, err := requestPid.ExtractPublicKey()
		if err != nil {
			return err
		}
		ok, err = crypto.Verify(payerPubKey, escrowContract, halfSignedEscrowContract.GetBuyerSignature())
		if !ok || err != nil {
			return fmt.Errorf("can't verify escrow contract: %v", err)
		}
		s := halfSignedGuardContract.GetRenterSignature()
		if s == nil {
			s = halfSignedGuardContract.GetPreparerSignature()
		}
		ok, err = crypto.Verify(payerPubKey, &guardContractMeta, s)
		if !ok || err != nil {
			return fmt.Errorf("can't verify guard contract: %v", err)
		}

		// Sign on the contract
		signedEscrowContractBytes, err := escrow.SignContractAndMarshal(escrowContract, halfSignedEscrowContract, n.PrivateKey, false)
		if err != nil {
			return err
		}
		signedGuardContractBytes, err := guard.SignedContractAndMarshal(&guardContractMeta, nil, halfSignedGuardContract,
			n.PrivateKey, false, false, guardContractMeta.RenterPid, guardContractMeta.RenterPid)
		if err != nil {
			return err
		}
		_, err = remote.P2PCall(req.Context, n, requestPid, "/storage/upload/recvcontract",
			ssID,
			shardHash,
			shardIndex,
			signedEscrowContractBytes,
			signedGuardContractBytes,
		)
		if err != nil {
			return err
		}
		contract, err := guard.UnmarshalGuardContract(signedGuardContractBytes)
		if err != nil {
			return err
		}
		sh.Contract(&shardpb.SingedContracts{
			SignedEscrowContract: signedEscrowContractBytes,
			GuardContract:        contract,
		})
		// check payment
		signedContractID, err := escrow.SignContractID(escrowContract.ContractId, n.PrivateKey)
		if err != nil {
			return err
		}

		paidIn := make(chan bool)
		go checkPaymentFromClient(req.Context, paidIn, signedContractID, cfg)
		paid := <-paidIn
		if !paid {
			return errors.New("contract is not paid:" + escrowContract.ContractId)
		}
		fmt.Println("init before download shard")
		downloadShardFromClient(n, api, contract, req.Arguments[1], shardHash)
		fmt.Println("init after download shard")
		in := &guardPb.ReadyForChallengeRequest{
			RenterPid:   guardContractMeta.RenterPid,
			FileHash:    guardContractMeta.FileHash,
			ShardHash:   guardContractMeta.ShardHash,
			ContractId:  guardContractMeta.ContractId,
			HostPid:     guardContractMeta.HostPid,
			PrepareTime: guardContractMeta.RentStart,
		}
		sign, err := crypto.Sign(n.PrivateKey, in)
		if err != nil {
			return err
		}
		in.Signature = sign
		err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(req.Context,
			func(ctx context.Context, client guardPb.GuardServiceClient) error {
				_, err = client.ReadyForChallenge(ctx, in)
				if err != nil {
					return err
				}
				return nil
			})
		if err != nil {
			return err
		}
		sh.Complete()
		return nil
	},
}

// call escrow service to check if payment is received or not
func checkPaymentFromClient(ctx context.Context, paidIn chan bool,
	contractID *escrowpb.SignedContractID, configuration *config.Config) {
	var err error
	paid := false
	err = backoff.Retry(func() error {
		paid, err = escrow.IsPaidin(ctx, configuration, contractID)
		if err != nil {
			return err
		}
		if paid {
			paidIn <- true
			return nil
		}
		return errors.New("reach max retry times")
	}, checkPaymentBo)
	if err != nil {
		log.Error("Check escrow IsPaidin failed", err)
		paidIn <- paid
	}
}

func downloadShardFromClient(n *core.IpfsNode, api coreiface.CoreAPI,
	guardContract *guardPb.Contract,
	fileHash string, shardHash string) {
	// Need to compute a time that's fair for small vs large files
	// TODO: use backoff to achieve pause/resume cases for host downloads
	low := 30 * time.Second
	high := 5 * time.Minute
	scaled := time.Duration(float64(guardContract.ShardFileSize) / float64(units.GiB) * float64(high))
	if scaled < low {
		scaled = low
	} else if scaled > high {
		scaled = high
	}
	ctx, _ := context.WithTimeout(context.Background(), scaled)
	expir := uint64(guardContract.RentEnd.Unix())
	// Get + pin to make sure it does not get accidentally deleted
	// Sharded scheme as special pin logic to add
	// file root dag + shard root dag + metadata full dag + only this shard dag
	fileCid, err := cidlib.Parse(fileHash)
	if err != nil {
		return
	}
	shardCid, err := cidlib.Parse(shardHash)
	if err != nil {
		return
	}
	_, err = storage.NewStorageChallengeResponse(ctx, n, api, fileCid, shardCid, "", true, expir)
	if err != nil {
		log.Errorf("failed to download shard %s from file %s with contract id %s: [%v]",
			guardContract.ShardHash, guardContract.FileHash, guardContract.ContractId, err)
		return
	}
}
