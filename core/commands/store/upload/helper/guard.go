package helper

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/escrow"

	config "github.com/TRON-US/go-btfs-config"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	cc "github.com/tron-us/go-btfs-common/config"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowPb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardPb "github.com/tron-us/go-btfs-common/protos/guard"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/ipfs/go-cid"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/prometheus/common/log"
)

type ContractParams struct {
	ContractId    string
	RenterPid     string
	HostPid       string
	ShardIndex    int32
	ShardHash     string
	ShardSize     int64
	FileHash      string
	StartTime     time.Time
	StorageLength int64
	Price         int64
	TotalPay      int64
}

func NewContract(conf *config.Config, params *ContractParams) (*guardPb.ContractMeta, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(conf)
	if err != nil {
		return nil, err
	}
	return &guardPb.ContractMeta{
		ContractId:    params.ContractId,
		RenterPid:     params.RenterPid,
		HostPid:       params.HostPid,
		ShardHash:     params.ShardHash,
		ShardIndex:    params.ShardIndex,
		ShardFileSize: params.ShardSize,
		FileHash:      params.FileHash,
		RentStart:     params.StartTime,
		RentEnd:       params.StartTime.Add(time.Duration(params.StorageLength*24) * time.Hour),
		GuardPid:      guardPid.Pretty(),
		EscrowPid:     escrowPid.Pretty(),
		Price:         params.Price,
		Amount:        params.TotalPay, // TODO: CHANGE and aLL other optional fields
	}, nil
}

func getGuardAndEscrowPid(configuration *config.Config) (peer.ID, peer.ID, error) {
	escrowPubKeys := configuration.Services.EscrowPubKeys
	if len(escrowPubKeys) == 0 {
		return "", "", fmt.Errorf("missing escrow public key in config")
	}
	guardPubKeys := configuration.Services.GuardPubKeys
	if len(guardPubKeys) == 0 {
		return "", "", fmt.Errorf("missing guard public key in config")
	}
	escrowPid, err := pidFromString(escrowPubKeys[0])
	if err != nil {
		log.Error("parse escrow config failed", escrowPubKeys[0])
		return "", "", err
	}
	guardPid, err := pidFromString(guardPubKeys[0])
	if err != nil {
		log.Error("parse guard config failed", guardPubKeys[1])
		return "", "", err
	}
	return guardPid, escrowPid, err
}

func pidFromString(key string) (peer.ID, error) {
	pubKey, err := escrow.ConvertPubKeyFromString(key)
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pubKey)
}

const (
	guardTimeout = 360 * time.Second
)

func PrepAndUploadFileMeta(ctx context.Context, contracts []*guardPb.Contract, payinRes *escrowPb.SignedPayinResult,
	payerPriKey ic.PrivKey, configuration *config.Config, renterId string, fileHash string,
	fileSize int64) (*guardPb.FileStoreStatus,
	error) {
	sig := payinRes.EscrowSignature
	for _, guardContract := range contracts {
		guardContract.EscrowSignature = sig
		guardContract.EscrowSignedTime = payinRes.Result.EscrowSignedTime
		guardContract.LastModifyTime = time.Now()
	}

	fileStatus, err := NewFileStatus(contracts, configuration, renterId, fileHash, fileSize)
	if err != nil {
		return nil, err
	}

	sign, err := crypto.Sign(payerPriKey, &fileStatus.FileStoreMeta)
	if err != nil {
		return nil, err
	}
	if fileStatus.PreparerPid == fileStatus.RenterPid {
		fileStatus.RenterSignature = sign
	} else {
		fileStatus.RenterSignature = sign
		fileStatus.PreparerSignature = sign
	}

	err = submitFileStatus(ctx, configuration, fileStatus)
	if err != nil {
		return nil, err
	}

	return fileStatus, nil
}

func NewFileStatus(contracts []*guardPb.Contract, configuration *config.Config,
	renterId string, fileHash string, fileSize int64) (*guardPb.FileStoreStatus, error) {
	guardPid, escrowPid, err := getGuardAndEscrowPid(configuration)
	if err != nil {
		return nil, err
	}
	var (
		rentStart   time.Time
		rentEnd     time.Time
		preparerPid = renterId
		renterPid   = renterId
		rentalState = guardPb.FileStoreStatus_NEW
	)
	if len(contracts) > 0 {
		rentStart = contracts[0].RentStart
		rentEnd = contracts[0].RentEnd
		preparerPid = contracts[0].PreparerPid
		renterPid = contracts[0].RenterPid
		if contracts[0].PreparerPid != contracts[0].RenterPid {
			rentalState = guardPb.FileStoreStatus_PARTIAL_NEW
		}
	}

	fileStoreMeta := guardPb.FileStoreMeta{
		RenterPid:        renterPid,
		FileHash:         fileHash, //TODO need to check
		FileSize:         fileSize,
		RentStart:        rentStart, //TODO need to revise later
		RentEnd:          rentEnd,   //TODO need to revise later
		CheckFrequency:   0,
		GuardFee:         0,
		EscrowFee:        0,
		ShardCount:       int32(len(contracts)),
		MinimumShards:    0,
		RecoverThreshold: 0,
		EscrowPid:        escrowPid.Pretty(),
		GuardPid:         guardPid.Pretty(),
	}

	return &guardPb.FileStoreStatus{
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

func submitFileStatus(ctx context.Context, cfg *config.Config,
	fileStatus *guardPb.FileStoreStatus) error {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	return cb.WithContext(ctx, func(ctx context.Context, client guardPb.GuardServiceClient) error {
		res, err := client.SubmitFileStoreMeta(ctx, fileStatus)
		if err != nil {
			return err
		}
		if res.Code != guardPb.ResponseCode_SUCCESS {
			return fmt.Errorf("failed to execute submit file status to gurad: %v", res.Message)
		}
		return nil
	})
}

// PrepFileChallengeQuestions checks and prepares all shard questions in one setting
func PrepFileChallengeQuestions(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI,
	fsStatus *guardPb.FileStoreStatus, fileHash string, peerId string,
	sessionId string) ([]*guardPb.ShardChallengeQuestions,
	error) {
	var shardHashes []cid.Cid
	var hostIDs []string
	for _, c := range fsStatus.Contracts {
		sh, err := cid.Parse(c.ShardHash)
		if err != nil {
			return nil, err
		}
		shardHashes = append(shardHashes, sh)
		hostIDs = append(hostIDs, c.HostPid)
	}

	questionsPerShard := cc.GetMinimumQuestionsCountPerShard(fsStatus)
	cid, err := cid.Parse(fileHash)
	if err != nil {
		return nil, err
	}
	return PrepCustomFileChallengeQuestions(ctx, n, api, cid, peerId, sessionId, shardHashes, hostIDs,
		questionsPerShard)
}

// PrepCustomFileChallengeQuestions is the inner version of PrepFileChallengeQuestions without
// using a real guard file contracts, but rather custom parameters (mostly for manual testing)
func PrepCustomFileChallengeQuestions(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI,
	fileHash cid.Cid, peerId string, sessionId string, shardHashes []cid.Cid,
	hostIDs []string, questionsPerShard int) ([]*guardPb.ShardChallengeQuestions, error) {
	// safety check
	if len(hostIDs) < len(shardHashes) {
		return nil, fmt.Errorf("hosts list must be at least %d", len(shardHashes))
	}
	// generate each shard's questions individually, then combine
	questions := make([]*guardPb.ShardChallengeQuestions, len(shardHashes))
	qc := make(chan questionRes)
	for i, sh := range shardHashes {
		go func(shardIndex int, shardHash cid.Cid, hostID string) {
			qs, err := PrepShardChallengeQuestions(ctx, n, api, fileHash, shardHash, hostID, questionsPerShard)
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
	qs  *guardPb.ShardChallengeQuestions
	err error
	i   int
}

// PrepShardChallengeQuestions checks and prepares an amount of random challenge questions
// and returns the necessary guard proto struct
func PrepShardChallengeQuestions(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	fileHash cid.Cid, shardHash cid.Cid,
	hostID string, numQuestions int) (*guardPb.ShardChallengeQuestions, error) {
	var shardQuestions []*guardPb.ChallengeQuestion
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
		q := &guardPb.ChallengeQuestion{
			ShardHash:    sh,
			HostPid:      hostID,
			ChunkIndex:   int32(sc.CIndex),
			Nonce:        sc.Nonce,
			ExpectAnswer: sc.Hash,
		}
		shardQuestions = append(shardQuestions, q)
	}
	sq := &guardPb.ShardChallengeQuestions{
		FileHash:      fileHash.String(),
		ShardHash:     sh,
		PreparerPid:   node.Identity.Pretty(),
		QuestionCount: int32(numQuestions),
		Questions:     shardQuestions,
		PrepareTime:   time.Now(),
	}
	sig, err := crypto.Sign(node.PrivateKey, sq)
	if err != nil {
		return nil, err
	}
	sq.PreparerSignature = sig
	return sq, nil
}

// SendChallengeQuestions combines all shard questions in a file and sends to guard service
func SendChallengeQuestions(ctx context.Context, cfg *config.Config, fileHash cid.Cid,
	questions []*guardPb.ShardChallengeQuestions) error {
	fileQuestions := &guardPb.FileChallengeQuestions{
		FileHash:       fileHash.String(),
		ShardQuestions: questions,
	}
	return sendChallengeQuestions(ctx, cfg, fileQuestions)
}

// sendChallengeQuestions opens a grpc connection, sends questions, and closes (short) connection
func sendChallengeQuestions(ctx context.Context, cfg *config.Config,
	req *guardPb.FileChallengeQuestions) error {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	return cb.WithContext(ctx, func(ctx context.Context, client guardPb.GuardServiceClient) error {
		res, err := client.SendQuestions(ctx, req)
		if err != nil {
			return err
		}
		if res.Code != guardPb.ResponseCode_SUCCESS {
			return fmt.Errorf("failed to send questions: %v", res.Message)
		}
		return nil
	})
}
