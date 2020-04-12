package upload

import (
	"fmt"
	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/guard"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/protobuf/proto"
	"strconv"
	"time"
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
		cmds.StringArg("offline-peer-id", false, false, "Peer id when offline sign is used."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ctxParams, err := extractContextParams(req, env)
		if err != nil {
			return err
		}
		if !ctxParams.cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		settings, err := storage.GetHostStorageConfig(ctxParams.ctx, ctxParams.n)
		if err != nil {
			return err
		}
		if uint64(price) < settings.StoragePriceAsk {
			return fmt.Errorf("price invalid: want: >=%d, got: %d", settings.StoragePriceAsk, price)
		}
		requestPid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.n)
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
		ssId := req.Arguments[0]
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[8])
		if err != nil {
			return err
		}

		halfSignedEscrowContString := req.Arguments[4]
		halfSignedGuardContString := req.Arguments[5]
		var halfSignedEscrowContBytes, halfSignedGuardContBytes []byte
		halfSignedEscrowContBytes = []byte(halfSignedEscrowContString)
		halfSignedGuardContBytes = []byte(halfSignedGuardContString)
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
		signedEscrowContractBytes, err := signEscrowContractAndMarshal(escrowContract, halfSignedEscrowContract,
			ctxParams.n.PrivateKey)
		if err != nil {
			return err
		}
		signedGuardContractBytes, err := signGuardContractAndMarshal(&guardContractMeta, nil, ctxParams.n.PrivateKey)
		if err != nil {
			return err
		}
		_, err = remote.P2PCall(ctxParams.ctx, ctxParams.n, requestPid, "/storage/upload/recvcontract",
			ssId,
			shardHash,
			shardIndex,
			signedEscrowContractBytes,
			signedGuardContractBytes,
		)
		if err != nil {
			return err
		}

		return nil
	},
}

func signEscrowContractAndMarshal(contract *escrowpb.EscrowContract, signedContract *escrowpb.SignedEscrowContract,
	privKey ic.PrivKey) ([]byte, error) {
	sig, err := crypto.Sign(privKey, contract)
	if err != nil {
		return nil, err
	}
	if signedContract == nil {
		signedContract = newSignedContract(contract)
	}
	signedContract.SellerSignature = sig
	signedBytes, err := proto.Marshal(signedContract)
	if err != nil {
		return nil, err
	}
	return signedBytes, nil
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
