package upload

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
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
)

var once sync.Once

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
		go once.Do(func() {
			for {
				ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
				ctxParams.Api.Swarm().Connect(ctx, peer.AddrInfo{ID: "16Uiu2HAm3Mw7YsZS3f5KH8VS4fnmdKxgQ1NNPugX7DpM5TQZP9uw"})
				time.Sleep(500 * time.Millisecond)
			}
		})
		if !ctxParams.Cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		settings, err := helper.GetHostStorageConfig(ctxParams.Ctx, ctxParams.N)
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

		halfSignedEscrowContString := req.Arguments[4]
		halfSignedGuardContString := req.Arguments[5]

		var halfSignedEscrowContBytes, halfSignedGuardContBytes []byte
		halfSignedEscrowContBytes = []byte(halfSignedEscrowContString)
		halfSignedGuardContBytes = []byte(halfSignedGuardContString)
		halfSignedGuardContract := &guardpb.Contract{}
		err = proto.Unmarshal(halfSignedGuardContBytes, halfSignedGuardContract)
		if err != nil {
			return err
		}

		halfSignedEscrowContract := &escrowpb.SignedEscrowContract{}
		err = proto.Unmarshal(halfSignedEscrowContBytes, halfSignedEscrowContract)
		if err != nil {
			return err
		}

		escrowContract := halfSignedEscrowContract.GetContract()
		guardContractMeta := halfSignedGuardContract.ContractMeta
		// get renter's public key
		pid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.N)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}
		var peerId string
		if peerId = pid.String(); len(req.Arguments) >= 10 {
			peerId = req.Arguments[9]
		}
		payerPubKey, err := crypto.GetPubKeyFromPeerId(peerId)
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

		// Verify price
		storageLength := guardContractMeta.RentEnd.Sub(guardContractMeta.RentStart).Hours() / 24
		totalPay := uh.TotalPay(guardContractMeta.ShardFileSize, guardContractMeta.Price, int(storageLength))
		if escrowContract.Amount != guardContractMeta.Amount || totalPay != guardContractMeta.Amount {
			return errors.New("invalid contract")
		}

		// Sign on the contract
		signedEscrowContractBytes, err := signEscrowContractAndMarshal(escrowContract, halfSignedEscrowContract,
			ctxParams.N.PrivateKey)
		if err != nil {
			return err
		}
		signedGuardContractBytes, err := signGuardContractAndMarshal(&guardContractMeta, halfSignedGuardContract, ctxParams.N.PrivateKey)
		if err != nil {
			return err
		}
		go func() {
			tmp := func() error {
				shard, err := sessions.GetHostShard(ctxParams, escrowContract.ContractId)
				if err != nil {
					return err
				}
				_, err = remote.P2PCall(ctxParams.Ctx, ctxParams.N, ctxParams.Api, requestPid, "/storage/upload/recvcontract",
					ssId,
					shardHash,
					shardIndex,
					signedEscrowContractBytes,
					signedGuardContractBytes,
				)
				if err != nil {
					return err
				}

				// check payment
				signedContractID, err := signContractID(escrowContract.ContractId, ctxParams.N.PrivateKey)
				if err != nil {
					return err
				}
				// check payment

				paidIn := make(chan bool)
				go checkPaymentFromClient(ctxParams, paidIn, signedContractID)
				paid := <-paidIn
				if !paid {
					return fmt.Errorf("contract is not paid: %s", escrowContract.ContractId)
				}
				tmp := new(guardpb.Contract)
				err = proto.Unmarshal(signedGuardContractBytes, tmp)
				if err != nil {
					return err
				}
				err = shard.Contract(signedEscrowContractBytes, tmp)
				if err != nil {
					return err
				}
				fmt.Println("do downloadShardFromClient...")
				err = downloadShardFromClient(ctxParams, halfSignedGuardContract, req.Arguments[1], shardHash)
				fmt.Println("done downloadShardFromClient...")
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
				// Need to renew another 5 mins due to downloading shard could have already made
				// req.Context obsolete
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				fmt.Println("do upload to guard...")
				err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctx,
					func(ctx context.Context, client guardpb.GuardServiceClient) error {
						_, err = client.ReadyForChallenge(ctx, in)
						if err != nil {
							return err
						}
						return nil
					})
				fmt.Println("done upload to guard...", "err", err)
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
		signedContract = escrow.NewSignedContract(contract)
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
	shardHash string) error {

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

	err = backoff.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), scaled)
		defer cancel()
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
