package upload

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/guard"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
	cidlib "github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

func doGuard(rss *sessions.RenterSession, res *escrowpb.SignedPayinResult, fileSize int64, offlineSigning bool) error {
	if err := rss.To(sessions.RssToGuardEvent); err != nil {
		return err
	}
	cts := make([]*guardpb.Contract, 0)
	selectedHosts := make([]string, 0)
	for i, h := range rss.ShardHashes {
		shard, err := sessions.GetRenterShard(rss.CtxParams, rss.SsId, h, i)
		if err != nil {
			return err
		}
		contracts, err := shard.Contracts()
		if err != nil {
			return err
		}
		contracts.SignedGuardContract.EscrowSignature = res.EscrowSignature
		contracts.SignedGuardContract.EscrowSignedTime = res.Result.EscrowSignedTime
		contracts.SignedGuardContract.LastModifyTime = time.Now()
		cts = append(cts, contracts.SignedGuardContract)
		selectedHosts = append(selectedHosts, contracts.SignedGuardContract.HostPid)
	}
	fsStatus, err := newFileStatus(cts, rss.CtxParams.Cfg, cts[0].ContractMeta.RenterPid, rss.Hash, fileSize)
	if err != nil {
		return err
	}
	cb := make(chan []byte)
	uh.FileMetaChanMaps.Set(rss.SsId, cb)
	if offlineSigning {
		raw, err := proto.Marshal(&fsStatus.FileStoreMeta)
		if err != nil {
			return err
		}
		err = rss.SaveOfflineSigning(&renterpb.OfflineSigning{
			Raw: raw,
		})
		if err != nil {
			return err
		}
	} else {
		go func() {
			if sig, err := func() ([]byte, error) {
				payerPrivKey, err := rss.CtxParams.Cfg.Identity.DecodePrivateKey("")
				if err != nil {
					return nil, err
				}
				sig, err := crypto.Sign(payerPrivKey, &fsStatus.FileStoreMeta)
				if err != nil {
					return nil, err
				}
				return sig, nil
			}(); err != nil {
				_ = rss.To(sessions.RssToErrorEvent, err)
				return
			} else {
				cb <- sig
			}
		}()
	}
	signBytes := <-cb
	uh.FileMetaChanMaps.Remove(rss.SsId)
	if err := rss.To(sessions.RssToGuardFileMetaSignedEvent); err != nil {
		return err
	}
	fsStatus, err = submitFileMetaHelper(rss.Ctx, rss.CtxParams.Cfg, fsStatus, signBytes)
	if err != nil {
		return err
	}
	qs, err := guard.PrepFileChallengeQuestions(rss, fsStatus, rss.Hash, offlineSigning, fsStatus.RenterPid)
	if err != nil {
		return err
	}

	fcid, err := cidlib.Parse(rss.Hash)
	if err != nil {
		return err
	}
	err = guard.SendChallengeQuestions(rss.Ctx, rss.CtxParams.Cfg, fcid, qs)
	if err != nil {
		return fmt.Errorf("failed to send challenge questions to guard: [%v]", err)
	}
	go func() {
		for {
			select {
			case <-rss.Ctx.Done():
				return
			default:
				wg := sync.WaitGroup{}
				for _, h := range selectedHosts {
					wg.Add(1)
					go func(host string) {
						ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
						id, _ := peer.IDB58Decode(host)
						rss.CtxParams.Api.Swarm().Connect(ctx, peer.AddrInfo{ID: id})
						wg.Done()
					}(h)
				}
				wg.Wait()
				time.Sleep(5 * time.Second)
			}
		}
	}()
	return waitUpload(rss, offlineSigning, fsStatus.RenterPid)
}

func newFileStatus(contracts []*guardpb.Contract, configuration *config.Config,
	renterId string, fileHash string, fileSize int64) (*guardpb.FileStoreStatus, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	var (
		rentStart   time.Time
		rentEnd     time.Time
		preparerPid = renterId
		renterPid   = renterId
		rentalState = guardpb.FileStoreStatus_NEW
	)
	if len(contracts) > 0 {
		rentStart = contracts[0].RentStart
		rentEnd = contracts[0].RentEnd
		preparerPid = contracts[0].PreparerPid
		renterPid = contracts[0].RenterPid
		if contracts[0].PreparerPid != contracts[0].RenterPid {
			rentalState = guardpb.FileStoreStatus_PARTIAL_NEW
		}
	}

	fileStoreMeta := guardpb.FileStoreMeta{
		RenterPid:        renterPid,
		FileHash:         fileHash,
		FileSize:         fileSize,
		RentStart:        rentStart,
		RentEnd:          rentEnd,
		CheckFrequency:   0,
		GuardFee:         0,
		EscrowFee:        0,
		ShardCount:       int32(len(contracts)),
		MinimumShards:    0,
		RecoverThreshold: 0,
		EscrowPid:        escrowPid.Pretty(),
		GuardPid:         guardPid.Pretty(),
	}

	return &guardpb.FileStoreStatus{
		FileStoreMeta:     fileStoreMeta,
		State:             0,
		Contracts:         contracts,
		RenterSignature:   nil,
		GuardReceiveTime:  time.Time{},
		ChangeLog:         nil,
		CurrentTime:       time.Now(),
		GuardSignature:    nil,
		RentalState:       rentalState,
		PreparerPid:       preparerPid,
		PreparerSignature: nil,
	}, nil
}

func submitFileMetaHelper(ctx context.Context, configuration *config.Config,
	fileStatus *guardpb.FileStoreStatus, sign []byte) (*guardpb.FileStoreStatus, error) {
	if fileStatus.PreparerPid == fileStatus.RenterPid {
		fileStatus.RenterSignature = sign
	} else {
		fileStatus.RenterSignature = sign
		fileStatus.PreparerSignature = sign
	}

	err := submitFileStatus(ctx, configuration, fileStatus)
	if err != nil {
		return nil, err
	}

	return fileStatus, nil
}

func submitFileStatus(ctx context.Context, cfg *config.Config,
	fileStatus *guardpb.FileStoreStatus) error {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guard.GuardTimeout)
	return cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
		res, err := client.SubmitFileStoreMeta(ctx, fileStatus)
		if err != nil {
			return err
		}
		if res.Code != guardpb.ResponseCode_SUCCESS {
			return fmt.Errorf("failed to execute submit file status to gurad: %v", res.Message)
		}
		return nil
	})
}
