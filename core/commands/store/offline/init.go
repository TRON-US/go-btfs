package upload

import (
	"context"
	"errors"
	"fmt"
	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v3"
	cidlib "github.com/ipfs/go-cid"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
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
		fmt.Println("init...")
		ctxParams, err := extractContextParams(req, env)
		if err != nil {
			fmt.Println(1, err)
			return err
		}
		if !ctxParams.cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			fmt.Println(2, err)
			return err
		}
		settings, err := storage.GetHostStorageConfig(ctxParams.ctx, ctxParams.n)
		if err != nil {
			fmt.Println(3, err)
			return err
		}
		if uint64(price) < settings.StoragePriceAsk {
			fmt.Println(4, err)
			return fmt.Errorf("price invalid: want: >=%d, got: %d", settings.StoragePriceAsk, price)
		}
		requestPid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.n)
		if !ok {
			fmt.Println(5, err)
			return fmt.Errorf("fail to get peer ID from request")
		}
		storeLen, err := strconv.Atoi(req.Arguments[6])
		if err != nil {
			fmt.Println(6, err)
			return err
		}
		if uint64(storeLen) < settings.StorageTimeMin {
			fmt.Println(7, err)
			return fmt.Errorf("store length invalid: want: >=%d, got: %d", settings.StorageTimeMin, storeLen)
		}
		ssId := req.Arguments[0]
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[8])
		if err != nil {
			fmt.Println(8, err)
			return err
		}

		halfSignedEscrowContString := req.Arguments[4]
		halfSignedGuardContString := req.Arguments[5]

		var halfSignedEscrowContBytes, halfSignedGuardContBytes []byte
		halfSignedEscrowContBytes = []byte(halfSignedEscrowContString)
		halfSignedGuardContBytes = []byte(halfSignedGuardContString)
		halfSignedGuardContract := &guardpb.Contract{}
		err = proto.Unmarshal(halfSignedGuardContBytes, halfSignedGuardContract)
		if err != nil {
			fmt.Println(9, err)
			return err
		}

		halfSignedEscrowContract := &escrowpb.SignedEscrowContract{}
		err = proto.Unmarshal(halfSignedEscrowContBytes, halfSignedEscrowContract)
		if err != nil {
			fmt.Println(10, err)
			return err
		}

		escrowContract := halfSignedEscrowContract.GetContract()
		guardContractMeta := halfSignedGuardContract.ContractMeta
		// get renter's public key
		payerPubKey, err := requestPid.ExtractPublicKey()
		if err != nil {
			fmt.Println(11, err)
			return err
		}
		ok, err = crypto.Verify(payerPubKey, escrowContract, halfSignedEscrowContract.GetBuyerSignature())
		if !ok || err != nil {
			fmt.Println(12, err)
			return fmt.Errorf("can't verify escrow contract: %v", err)
		}
		s := halfSignedGuardContract.GetRenterSignature()
		if s == nil {
			s = halfSignedGuardContract.GetPreparerSignature()
		}
		ok, err = crypto.Verify(payerPubKey, &guardContractMeta, s)
		if !ok || err != nil {
			fmt.Println(13, err)
			return fmt.Errorf("can't verify guard contract: %v", err)
		}

		// Sign on the contract
		signedEscrowContractBytes, err := signEscrowContractAndMarshal(escrowContract, halfSignedEscrowContract,
			ctxParams.n.PrivateKey)
		if err != nil {
			fmt.Println(14, err)
			return err
		}
		signedGuardContractBytes, err := signGuardContractAndMarshal(&guardContractMeta, nil, ctxParams.n.PrivateKey)
		if err != nil {
			fmt.Println(15, err)
			return err
		}
		go func() {
			tmp := func() error {
				_, err = remote.P2PCall(ctxParams.ctx, ctxParams.n, requestPid, "/storage/upload/recvcontract",
					ssId,
					shardHash,
					shardIndex,
					signedEscrowContractBytes,
					signedGuardContractBytes,
				)
				if err != nil {
					fmt.Println(16, err)
					return err
				}
				return nil
				// check payment
				signedContractID, err := escrow.SignContractID(escrowContract.ContractId, ctxParams.n.PrivateKey)
				if err != nil {
					fmt.Println(17, err)
					return err
				}

				paidIn := make(chan bool)
				go checkPaymentFromClient(ctxParams, paidIn, signedContractID)
				paid := <-paidIn
				if !paid {
					fmt.Println(18, err)
					return errors.New("contract is not paid:" + escrowContract.ContractId)
				}
				downloadShardFromClient(ctxParams, halfSignedGuardContract, req.Arguments[1], shardHash)
				in := &guardpb.ReadyForChallengeRequest{
					RenterPid:   guardContractMeta.RenterPid,
					FileHash:    guardContractMeta.FileHash,
					ShardHash:   guardContractMeta.ShardHash,
					ContractId:  guardContractMeta.ContractId,
					HostPid:     guardContractMeta.HostPid,
					PrepareTime: guardContractMeta.RentStart,
				}
				sign, err := crypto.Sign(ctxParams.n.PrivateKey, in)
				if err != nil {
					fmt.Println(19, err)
					return err
				}
				in.Signature = sign
				fmt.Println("before ready for challenge")
				err = grpc.GuardClient(ctxParams.cfg.Services.GuardDomain).WithContext(req.Context,
					func(ctx context.Context, client guardpb.GuardServiceClient) error {
						_, err = client.ReadyForChallenge(ctx, in)
						if err != nil {
							fmt.Println(20, err)
							return err
						}
						return nil
					})
				if err != nil {
					fmt.Println(21, err)
					return err
				}
				fmt.Println("after ready for challenge")
				return nil
			}()
			//TODO
			log.Error(tmp)
		}()
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

// call escrow service to check if payment is received or not
func checkPaymentFromClient(ctxParams *ContextParams, paidIn chan bool, contractID *escrowpb.SignedContractID) {
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
	}, checkPaymentBo)
	if err != nil {
		log.Error("Check escrow IsPaidin failed", err)
		paidIn <- paid
	}
}

func downloadShardFromClient(ctxParams *ContextParams, guardContract *guardpb.Contract, fileHash string, shardHash string) {
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
	_, err = storage.NewStorageChallengeResponse(ctx, ctxParams.n, ctxParams.api, fileCid, shardCid, "", true, expir)
	if err != nil {
		log.Errorf("failed to download shard %s from file %s with contract id %s: [%v]",
			guardContract.ShardHash, guardContract.FileHash, guardContract.ContractId, err)
		return
	}
}

func isPaidin(ctxParams *ContextParams, contractID *escrowpb.SignedContractID) (bool, error) {
	var signedPayinRes *escrowpb.SignedPayinStatus
	err := grpc.EscrowClient(ctxParams.cfg.Services.EscrowDomain).WithContext(ctxParams.ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.IsPaid(ctx, contractID)
			if err != nil {
				return err
			}
			err = verifyEscrowRes(ctxParams.cfg, res.Status, res.EscrowSignature)
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
