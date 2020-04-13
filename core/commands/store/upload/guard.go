package upload

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage"

	config "github.com/TRON-US/go-btfs-config"
	cc "github.com/tron-us/go-btfs-common/config"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	cidlib "github.com/ipfs/go-cid"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	guardTimeout = 360 * time.Second
)

var (
	fileMetaChanMaps  = cmap.New()
	questionsChanMaps = cmap.New()
)

func doGuard(rss *RenterSession, res *escrowpb.SignedPayinResult, fileSize int64, offlineSigning bool) {
	rss.to(rssToGuardEvent)
	cts := make([]*guardpb.Contract, 0)
	for _, h := range rss.shardHashes {
		shard, err := GetRenterShard(rss.ctxParams, rss.ssId, h)
		if err != nil {
			//TODO
			return
		}
		contracts, err := shard.contracts()
		if err != nil {
			//TODO
			return
		}
		contracts.SignedGuardContract.EscrowSignature = res.EscrowSignature
		contracts.SignedGuardContract.EscrowSignedTime = res.Result.EscrowSignedTime
		contracts.SignedGuardContract.LastModifyTime = time.Now()
		cts = append(cts, contracts.SignedGuardContract)
	}
	fsStatus, err := newFileStatus(cts, rss.ctxParams.cfg, cts[0].ContractMeta.RenterPid, rss.hash, fileSize)
	if err != nil {
		//TODO
		return
	}
	cb := make(chan []byte)
	fileMetaChanMaps.Set(rss.ssId, cb)
	if !offlineSigning {
		go func() {
			payerPrivKey, err := rss.ctxParams.cfg.Identity.DecodePrivateKey("")
			if err != nil {
				//TODO
				return
			}
			sign, err := crypto.Sign(payerPrivKey, &fsStatus.FileStoreMeta)
			if err != nil {
				//TODO
				return
			}
			cb <- sign
		}()
	}
	signBytes := <-cb
	rss.to(rssToGuardFileMetaSignedEvent)
	fmt.Println("guard", 1)
	fsStatus, err = submitFileMetaHelper(rss.ctx, rss.ctxParams.cfg, fsStatus, signBytes)
	if err != nil {
		fmt.Println("guard 1", err)
		//TODO
		return
	}
	fmt.Println("guard", 2)
	qs, err := PrepFileChallengeQuestions(rss, fsStatus, rss.hash)
	if err != nil {
		//TODO
		return
	}

	fmt.Println("guard", 3)
	fcid, err := cidlib.Parse(rss.hash)
	if err != nil {
		//TODO
		return
	}
	fmt.Println("guard", 4)
	err = SendChallengeQuestions(rss.ctx, rss.ctxParams.cfg, fcid, qs)
	if err != nil {
		//TODO
		//fmt.Errorf("failed to send challenge questions to guard: [%v]", err)
		return
	}
	fmt.Println("guard", 5)
	waitUpload(rss, offlineSigning)
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
	cb.Timeout(guardTimeout)
	return cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
		fmt.Println("fileStatus")
		fmt.Println(fileStatus.RenterPid)
		fmt.Println(fileStatus.FileHash)
		fmt.Println(fileStatus.State)
		fmt.Println(fileStatus.GuardPid)
		fmt.Println(fileStatus.PreparerPid)
		fmt.Println("fileStatus.Contracts[0].PreparerPid")
		fmt.Println(fileStatus.Contracts[0].PreparerPid)
		fmt.Println(fileStatus.Contracts[0].GuardPid)
		fmt.Println(fileStatus.Contracts[0].FileHash)
		fmt.Println(fileStatus.Contracts[0].RenterPid)
		fmt.Println(fileStatus.Contracts[0].HostPid)
		fmt.Println(fileStatus.Contracts[0].ShardHash)
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

// PrepFileChallengeQuestions checks and prepares all shard questions in one setting
func PrepFileChallengeQuestions(rss *RenterSession, fsStatus *guardpb.FileStoreStatus, fileHash string) (
	[]*guardpb.ShardChallengeQuestions, error) {
	var shardHashes []cidlib.Cid
	var hostIDs []string
	for _, c := range fsStatus.Contracts {
		sh, err := cidlib.Parse(c.ShardHash)
		if err != nil {
			return nil, err
		}
		shardHashes = append(shardHashes, sh)
		hostIDs = append(hostIDs, c.HostPid)
	}

	questionsPerShard := cc.GetMinimumQuestionsCountPerShard(fsStatus)
	cid, err := cidlib.Parse(fileHash)
	if err != nil {
		return nil, err
	}
	return PrepCustomFileChallengeQuestions(rss, cid, shardHashes, hostIDs, questionsPerShard)
}

// PrepCustomFileChallengeQuestions is the inner version of PrepFileChallengeQuestions without
// using a real guard file contracts, but rather custom parameters (mostly for manual testing)
func PrepCustomFileChallengeQuestions(rss *RenterSession, fileHash cidlib.Cid, shardHashes []cidlib.Cid,
	hostIDs []string, questionsPerShard int) ([]*guardpb.ShardChallengeQuestions, error) {
	// safety check
	if len(hostIDs) < len(shardHashes) {
		return nil, fmt.Errorf("hosts list must be at least %d", len(shardHashes))
	}
	// generate each shard's questions individually, then combine
	questions := make([]*guardpb.ShardChallengeQuestions, len(shardHashes))
	qc := make(chan questionRes)
	for i, sh := range shardHashes {
		go func(shardIndex int, shardHash cidlib.Cid, hostID string) {
			qs, err := PrepShardChallengeQuestions(rss, fileHash, shardHash, hostID, questionsPerShard)
			qc <- questionRes{
				qs:  qs,
				err: err,
				i:   shardIndex,
			}
		}(i, sh, hostIDs[i])
	}
	for i := 0; i < len(questions); i++ {
		res := <-qc
		if res.err != nil {
			return nil, res.err
		}
		questions[res.i] = res.qs
	}

	return questions, nil
}

type questionRes struct {
	qs  *guardpb.ShardChallengeQuestions
	err error
	i   int
}

// PrepShardChallengeQuestions checks and prepares an amount of random challenge questions
// and returns the necessary guard proto struct
func PrepShardChallengeQuestions(rss *RenterSession, fileHash cidlib.Cid, shardHash cidlib.Cid,
	hostID string, numQuestions int) (*guardpb.ShardChallengeQuestions, error) {
	ctx := rss.ctx
	node := rss.ctxParams.n
	api := rss.ctxParams.api
	var shardQuestions []*guardpb.ChallengeQuestion
	var sc *storage.StorageChallenge
	var err error

	//FIXME: cache challange info
	//if shardInfo != nil {
	//	sc, err = shardInfo.GetChallengeOrNew(ctx, node, api, fileHash)
	//	if err != nil {
	//		return nil, err
	//	}
	//} else {
	// For testing, no session concept, create new challenge
	sc, err = storage.NewStorageChallenge(ctx, node, api, fileHash, shardHash)
	if err != nil {
		return nil, err
	}
	//}
	// Generate questions
	sh := shardHash.String()
	for i := 0; i < numQuestions; i++ {
		err := sc.GenChallenge()
		if err != nil {
			return nil, err
		}
		q := &guardpb.ChallengeQuestion{
			ShardHash:    sh,
			HostPid:      hostID,
			ChunkIndex:   int32(sc.CIndex),
			Nonce:        sc.Nonce,
			ExpectAnswer: sc.Hash,
		}
		shardQuestions = append(shardQuestions, q)
	}
	sq := &guardpb.ShardChallengeQuestions{
		FileHash:      fileHash.String(),
		ShardHash:     sh,
		PreparerPid:   node.Identity.Pretty(),
		QuestionCount: int32(numQuestions),
		Questions:     shardQuestions,
		PrepareTime:   time.Now(),
	}
	cb := make(chan []byte)
	questionsChanMaps.Set(rss.ssId, cb)
	go func() {
		fmt.Println("sign questions:", shardHash)
		sig, err := crypto.Sign(node.PrivateKey, sq)
		if err != nil {
			//TODO
			return
		}
		cb <- sig
	}()
	sig := <-cb
	sq.PreparerSignature = sig
	return sq, nil
}

// SendChallengeQuestions combines all shard questions in a file and sends to guard service
func SendChallengeQuestions(ctx context.Context, cfg *config.Config, fileHash cidlib.Cid,
	questions []*guardpb.ShardChallengeQuestions) error {
	fileQuestions := &guardpb.FileChallengeQuestions{
		FileHash:       fileHash.String(),
		ShardQuestions: questions,
	}
	return sendChallengeQuestions(ctx, cfg, fileQuestions)
}

// sendChallengeQuestions opens a grpc connection, sends questions, and closes (short) connection
func sendChallengeQuestions(ctx context.Context, cfg *config.Config, req *guardpb.FileChallengeQuestions) error {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	return cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
		res, err := client.SendQuestions(ctx, req)
		if err != nil {
			return err
		}
		if res.Code != guardpb.ResponseCode_SUCCESS {
			return fmt.Errorf("failed to send questions: %v", res.Message)
		}
		return nil
	})
}
