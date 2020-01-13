package guard

import (
	"context"
	"fmt"
	"time"

	core "github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	cc "github.com/tron-us/go-btfs-common/config"
	ccrypto "github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	config "github.com/TRON-US/go-btfs-config"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/ipfs/go-cid"
)

// PrepShardChallengeQuestions checks and prepares an amount of random challenge questions
// and returns the necessary guard proto struct
func PrepShardChallengeQuestions(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	fileHash cid.Cid, shardInfo *storage.Shard, shardHash cid.Cid,
	hostID string, numQuestions int) (*guardpb.ShardChallengeQuestions, error) {
	var shardQuestions []*guardpb.ChallengeQuestion
	var sc *storage.StorageChallenge
	var err error
	if shardInfo != nil {
		sc, err = shardInfo.GetChallengeOrNew(ctx, node, api, fileHash)
		if err != nil {
			return nil, err
		}
	} else {
		// For testing, no session concept, create new challenge
		sc, err = storage.NewStorageChallenge(ctx, node, api, fileHash, shardHash)
		if err != nil {
			return nil, err
		}
	}
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
	sig, err := ccrypto.Sign(node.PrivateKey, sq)
	if err != nil {
		return nil, err
	}
	sq.PreparerSignature = sig
	return sq, nil
}

type questionRes struct {
	qs  *guardpb.ShardChallengeQuestions
	err error
	i   int
}

// PrepFileChallengeQuestions checks and prepares all shard questions in one setting
func PrepFileChallengeQuestions(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI,
	ss *storage.FileContracts, fsStatus *guardpb.FileStoreStatus) ([]*guardpb.ShardChallengeQuestions, error) {
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
	return PrepCustomFileChallengeQuestions(ctx, n, api, ss, ss.FileHash, shardHashes, hostIDs, questionsPerShard)
}

// PrepCustomFileChallengeQuestions is the inner version of PrepFileChallengeQuestions without
// using a real guard file contracts, but rather custom parameters (mostly for manual testing)
func PrepCustomFileChallengeQuestions(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI,
	ss *storage.FileContracts, fileHash cid.Cid, shardHashes []cid.Cid,
	hostIDs []string, questionsPerShard int) ([]*guardpb.ShardChallengeQuestions, error) {
	// safety check
	if len(hostIDs) < len(shardHashes) {
		return nil, fmt.Errorf("hosts list must be at least %d", len(shardHashes))
	}
	// generate each shard's questions individually, then combine
	questions := make([]*guardpb.ShardChallengeQuestions, len(shardHashes))
	qc := make(chan questionRes)
	for i, sh := range shardHashes {
		var si *storage.Shard
		if ss != nil {
			var err error
			si, err = ss.GetShard(sh.String(), i)
			if err != nil {
				return nil, err
			}
		}
		go func(shardIndex int, shardInfo *storage.Shard, shardHash cid.Cid, hostID string) {
			qs, err := PrepShardChallengeQuestions(ctx, n, api,
				fileHash, shardInfo, shardHash, hostID, questionsPerShard)
			qc <- questionRes{
				qs:  qs,
				err: err,
				i:   shardIndex,
			}
		}(i, si, sh, hostIDs[i])
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

// SendChallengeQuestions combines all shard questions in a file and sends to guard service
func SendChallengeQuestions(ctx context.Context, cfg *config.Config, fileHash cid.Cid,
	questions []*guardpb.ShardChallengeQuestions) error {
	fileQuestions := &guardpb.FileChallengeQuestions{
		FileHash:       fileHash.String(),
		ShardQuestions: questions,
	}
	return sendChallengeQuestions(ctx, cfg, fileQuestions)
}
