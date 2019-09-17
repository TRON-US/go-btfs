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
	cid "github.com/ipfs/go-cid"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	path "github.com/TRON-US/interface-go-btfs-core/path"

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
	ID    string  // Challenge ID (randomly generated on creation)
	CID   cid.Cid // Chunk hash in multihash format (selected chunk on each challenge request)
	Nonce string  // Random nonce for each challenge request (uuidv4)
	Hash  string  // Generated SHA-256 hash (chunk bytes + nonce bytes) for proof-of-file-existence
}

// NewStorageChallenge creates a challenge object with new ID, resolves the cid path
// and initializes underlying CIDs to be ready for challenge generation.
// Used by storage client.
func NewStorageChallenge(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	fileHash cid.Cid) (*StorageChallenge, error) {
	chid, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	sc := &StorageChallenge{
		Ctx:      ctx,
		Node:     node,
		API:      api,
		seenCIDs: map[string]bool{},
		ID:       chid.String(),
	}
	if err := sc.getAllCIDsRecursive(fileHash); err != nil {
		return nil, err
	}
	return sc, nil
}

// NewStorageChallengeResponse creates (rebuilds) a plain challenge object with only initializing
// the basic components.
// Used by storage host.
func NewStorageChallengeResponse(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	challengeID string) *StorageChallenge {
	return &StorageChallenge{
		Ctx:  ctx,
		Node: node,
		API:  api,
		ID:   challengeID,
	}
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

	// Fetch the raw data
	r, err := sc.API.Object().Data(sc.Ctx, path.IpfsPath(sc.CID))
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

// SolveChallenge solves the given challenge block hash + nonce's request
// and records the solved/responding value in Hash field.
func (sc *StorageChallenge) SolveChallenge(chHash cid.Cid, chNonce string) error {
	sc.Lock()
	defer sc.Unlock()

	// Fetch the raw data
	r, err := sc.API.Object().Data(sc.Ctx, path.IpfsPath(chHash))
	if err != nil {
		return err
	}
	sc.CID = chHash

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
