package host

import (
	"fmt"
	"strconv"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage/renter/guard"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/hub"

	"github.com/tron-us/go-btfs-common/crypto"

	"github.com/cenkalti/backoff/v3"
	cidlib "github.com/ipfs/go-cid"
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
	},
	RunTimeout: 5 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		fmt.Println("init...")
		// check flags
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		ssID := req.Arguments[0]
		fileHash, err := cidlib.Parse(req.Arguments[1])
		if err != nil {
			return err
		}
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[8])
		if err != nil {
			return err
		}
		shardSize, err := strconv.ParseInt(req.Arguments[7], 10, 64)
		if err != nil {
			return err
		}
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		halfSignedEscrowContBytes := req.Arguments[4]
		halfSignedGuardContBytes := req.Arguments[5]
		fmt.Println("ssID", ssID, "fileHash", fileHash, "shardHash", shardHash, "shardIndex", shardIndex,
			"shardSize", shardSize)
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		settings, err := hub.GetSettings(req.Context, cfg.Services.HubDomain, n.Identity.Pretty(), n.Repo.Datastore())
		if err != nil {
			return err
		}
		if uint64(price) < settings.StoragePriceAsk {
			return fmt.Errorf("price invalid: want: >=%d, got: %d", settings.StoragePriceAsk, price)
		}
		pid, ok := remote.GetStreamRequestRemotePeerID(req, n)
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

		halfSignedGuardContract, err := guard.UnmarshalGuardContract([]byte(halfSignedGuardContBytes))
		if err != nil {
			return err
		}

		// review contract and send back to client
		halfSignedEscrowContract, err := escrow.UnmarshalEscrowContract([]byte(halfSignedEscrowContBytes))
		if err != nil {
			return err
		}
		if err != nil {
			return err
		}

		escrowContract := halfSignedEscrowContract.GetContract()
		guardContractMeta := halfSignedGuardContract.ContractMeta
		// get renter's public key
		payerPubKey, err := pid.ExtractPublicKey()
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

		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		fmt.Println("api", api)

		// Sign on the contract
		signedEscrowContractBytes, err := escrow.SignContractAndMarshal(escrowContract, halfSignedEscrowContract, n.PrivateKey, false)
		if err != nil {
			return err
		}
		signedGuardContractBytes, err := guard.SignedContractAndMarshal(&guardContractMeta, halfSignedGuardContract,
			n.PrivateKey, false, false, pid.Pretty(), pid.Pretty())
		if err != nil {
			return err
		}
		go func() {
			//FIXME: receive recvcontract request before uploadinit response in renter side.
			time.Sleep(500 * time.Millisecond)
			_, err = remote.P2PCall(req.Context, n, pid, "/storage/upload/recvcontract",
				ssID,
				shardHash,
				strconv.Itoa(shardIndex),
				signedEscrowContractBytes,
				signedGuardContractBytes,
			)
		}()
		return nil
	},
}
