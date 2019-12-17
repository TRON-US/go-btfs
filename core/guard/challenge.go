package guard

import (
	"context"
	"fmt"
	"time"

	core "github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	ccrypto "github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	config "github.com/TRON-US/go-btfs-config"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/ipfs/go-cid"
)

// PrepShardChallengeQuestions checks and prepares an amount of random challenge questions
// and returns the necessary guard proto struct
func PrepShardChallengeQuestions(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	fileHash, shardHash cid.Cid, hostID string, numQuestions int) (*guardpb.ShardChallengeQuestions, error) {
	var shardQuestions []*guardpb.ChallengeQuestion
	// Generate questions
	sc, err := storage.NewStorageChallenge(ctx, node, api, fileHash, shardHash)
	if err != nil {
		return nil, err
	}
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
	fileHash cid.Cid, shardHashes []cid.Cid, hostIDs []string,
	questionsPerShard int) ([]*guardpb.ShardChallengeQuestions, error) {
	if len(hostIDs) < len(shardHashes) {
		return nil, fmt.Errorf("hosts list must be at least %d", len(shardHashes))
	}
	// generate each shard's questions individually, then combine
	questions := make([]*guardpb.ShardChallengeQuestions, len(shardHashes))
	qc := make(chan questionRes)
	for i, sh := range shardHashes {
		go func(shardIndex int, shardHash cid.Cid) {
			qs, err := PrepShardChallengeQuestions(ctx, n, api,
				fileHash, shardHash, hostIDs[i], questionsPerShard)
			qc <- questionRes{
				qs:  qs,
				err: err,
				i:   shardIndex,
			}
		}(i, sh)
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
