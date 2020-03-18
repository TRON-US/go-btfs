package upload

import (
	"context"
	"errors"
	"fmt"
	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/guard"
	"github.com/cenkalti/backoff/v3"
	"github.com/dustin/go-humanize"
	cidlib "github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/prometheus/common/log"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"strconv"
	"time"
)

var bo = func() *backoff.ExponentialBackOff {
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

		var (
			requestPid peer.ID
			renterPid  peer.ID
			ok         bool
		)

		ssID := req.Arguments[0]
		fileHash, err := cidlib.Parse(req.Arguments[1])
		if err != nil {
			return err
		}
		fmt.Println("fileHash", fileHash)
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[8])
		if err != nil {
			return err
		}
		shardSize, err := strconv.ParseInt(req.Arguments[7], 10, 64)
		if err != nil {
			return err
		}

		// Check existing storage has enough left
		cfgRoot, err := cmdenv.GetConfigRoot(env)
		if err != nil {
			return err
		}
		n, err := cmdenv.GetNode(env)
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
		payerPubKey, err := renterPid.ExtractPublicKey()
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
		signedEscrowContractBytes, err := escrow.SignContractAndMarshal(escrowContract, nil, n.PrivateKey, false)
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
		grpc.GuardClient(cfg.Services.GuardDomain).WithContext(req.Context,
			func(ctx context.Context, client guardPb.GuardServiceClient) error {
				challenge, err := client.ReadyForChallenge(ctx, in)
				if err != nil {
					fmt.Println("ready challenge error", err)
					return err
				}
				fmt.Println("code", challenge.Code, "msg", challenge.Message)
				return nil
			})
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
	}, bo)
	if err != nil {
		log.Error("Check escrow IsPaidin failed", err)
		paidIn <- paid
	}
}
