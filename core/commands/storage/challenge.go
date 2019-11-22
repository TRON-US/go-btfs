package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"
	"sync"

	core "github.com/TRON-US/go-btfs/core"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	path "github.com/TRON-US/interface-go-btfs-core/path"
	cid "github.com/ipfs/go-cid"

	uuid "github.com/google/uuid"
)

type StorageChallenge struct {
	Ctx  context.Context
	Node *core.IpfsNode
	API  coreiface.CoreAPI

	// Internal fields for parsing chunks
	seenCIDs map[string]bool
	allCIDs  []cid.Cid // Not storing node for faster fetch
	sync.Mutex

	// Selections and generations for challenge
	ID     string  // Challenge ID (randomly generated on creation)
	SID    cid.Cid // Shard hash in multihash format (selected shard on each challenge request)
	CIndex int     // Chunk index within each shard (selected index on each challenge request)
	CID    cid.Cid // Chunk hash in multihash format (selected chunk on each challenge request)
	Nonce  string  // Random nonce for each challenge request (uuidv4)
	Hash   string  // Generated SHA-256 hash (chunk bytes + nonce bytes) for proof-of-file-existence
}

// newStorageChallengeHelper creates a challenge object with new ID, resolves the cid path
// and initializes underlying CIDs to be ready for challenge generation.
// When used by storage client: challengeID is "", will be randomly genenerated
// When used by storage host: challengeID is a valid uuid v4
func newStorageChallengeHelper(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	shardHash cid.Cid, challengeID string) (*StorageChallenge, error) {
	if challengeID == "" {
		chid, err := uuid.NewRandom()
		if err != nil {
			return nil, err
		}
		challengeID = chid.String()
	}
	sc := &StorageChallenge{
		Ctx:      ctx,
		Node:     node,
		API:      api,
		seenCIDs: map[string]bool{},
		ID:       challengeID,
		SID:      shardHash,
	}
	if err := sc.getAllCIDsRecursive(shardHash); err != nil {
		return nil, err
	}
	return sc, nil
}

func NewStorageChallenge(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	shardHash cid.Cid) (*StorageChallenge, error) {
	return newStorageChallengeHelper(ctx, node, api, shardHash, "")
}

func NewStorageChallengeResponse(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	shardHash cid.Cid, challengeID string) (*StorageChallenge, error) {
	return newStorageChallengeHelper(ctx, node, api, shardHash, challengeID)
}

// getAllCIDsRecursive traverses the full DAG to find all cids and
// stores them in allCIDs for quick challenge regeneration/retry.
func (sc *StorageChallenge) getAllCIDsRecursive(blockHash cid.Cid) error {
	ncs := string(blockHash.Bytes()) // shorter/faster key
	// Already seen
	if _, ok := sc.seenCIDs[ncs]; ok {
		return nil
	}
	// Check if can be resolved
	rp, err := sc.API.ResolvePath(sc.Ctx, path.IpfsPath(blockHash))
	if err != nil {
		return err
	}
	// Mark as seen
	sc.seenCIDs[ncs] = true
	sc.allCIDs = append(sc.allCIDs, blockHash)
	// Recurse
	links, err := sc.API.Object().Links(sc.Ctx, rp)
	if err != nil {
		return err
	}
	for _, l := range links {
		if err := sc.getAllCIDsRecursive(l.Cid); err != nil {
			return err
		}
	}
	return nil
}

// GenChallenge picks a random cid from the internal cid list,
// then generates a random nonce, combined for the final hash.
// It is safe to call this function multiple times to re-gen a new challenge
// while re-using the same data structure and ID.
func (sc *StorageChallenge) GenChallenge() error {
	sc.Lock()
	defer sc.Unlock()

	// Randomly select a (new) CID
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(sc.allCIDs))))
	if err != nil {
		return err
	}
	sc.CID = sc.allCIDs[n.Int64()]
	sc.CIndex = int(n.Int64())

	// Fetch the raw data
	r, _, err := sc.API.Object().Data(sc.Ctx, path.IpfsPath(sc.CID), true, false)
	if err != nil {
		return err
	}

	nonce, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	// Give user readable string
	sc.Nonce = nonce.String()

	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return err
	}
	// But use bytes for hashing
	nb := [16]byte(nonce)
	h.Write(nb[:])
	sc.Hash = fmt.Sprintf("%x", h.Sum(nil))

	return nil
}

// SolveChallenge solves the given challenge chunk index + nonce's request
// and records the solved/responding value in Hash field.
func (sc *StorageChallenge) SolveChallenge(chIndex int, chNonce string) error {
	sc.Lock()
	defer sc.Unlock()

	// Get the cid for the chunk
	if chIndex < 0 || chIndex >= len(sc.allCIDs) {
		return fmt.Errorf("chunk index is out of range")
	}

	// Fetch the raw data
	chHash := sc.allCIDs[chIndex]
	r, _, err := sc.API.Object().Data(sc.Ctx, path.IpfsPath(chHash), true, false)
	if err != nil {
		return err
	}
	sc.CID = chHash
	sc.CIndex = chIndex

	// Decode nonce
	nonce, err := uuid.Parse(chNonce)
	if err != nil {
		return err
	}
	sc.Nonce = chNonce

	// Re-hash to solve challenge
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return err
	}
	nb := [16]byte(nonce)
	h.Write(nb[:])
	sc.Hash = fmt.Sprintf("%x", h.Sum(nil))

	return nil
}
