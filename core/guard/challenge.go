package guard

import (
	"context"
	"time"

	core "github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	ccrypto "github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	config "github.com/TRON-US/go-btfs-config"
	"github.com/ipfs/go-cid"
)

// PrepShardChallengeQuestions checks and prepares an amount of random challenge questions
// and returns the necessary guard proto struct
func PrepShardChallengeQuestions(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	fileHash, shardHash cid.Cid, hostID string, numQuestions int) (*guardpb.ShardChallengeQuestions, error) {
	var shardQuestions []*guardpb.ChallengeQuestion
	// Generate questions
	sc, err := storage.NewStorageChallenge(ctx, node, api, shardHash)
	if err != nil {
		return nil, err
	}
	sh := []byte(shardHash.String())
	for i := 0; i < numQuestions; i++ {
		err := sc.GenChallenge()
		if err != nil {
			return nil, err
		}
		q := &guardpb.ChallengeQuestion{
			ShardHash:    sh,
			HostAddress:  []byte(hostID),
			ChunkIndex:   int32(sc.CIndex),
			RandomNonce:  []byte(sc.Nonce),
			ExpectAnswer: []byte(sc.Hash),
		}
		shardQuestions = append(shardQuestions, q)
	}
	now := time.Now()
	sq := &guardpb.ShardChallengeQuestions{
		FileHash:        []byte(fileHash.String()),
		ShardHash:       sh,
		PreparerAddress: []byte(node.Identity.Pretty()),
		QuestionCount:   int32(numQuestions),
		Questions:       shardQuestions,
		PrepareTime:     &now,
	}
	sig, err := ccrypto.Sign(node.PrivateKey, sq)
	if err != nil {
		return nil, err
	}
	sq.PreparerSignature = sig
	return sq, nil
}

// SendChallengeQuestions combines all shard questions in a file and sends to guard service
func SendChallengeQuestions(ctx context.Context, cfg *config.Config, fileHash cid.Cid,
	questions []*guardpb.ShardChallengeQuestions) error {
	fileQuestions := &guardpb.FileChallengeQuestions{
		FileHash:       []byte(fileHash.String()),
		ShardQuestions: questions,
	}
	return sendChallengeQuestions(ctx, cfg, fileQuestions)
}
