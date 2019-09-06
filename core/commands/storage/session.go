package storage

import (
	"context"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core"
	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/libp2p/go-libp2p-core/peer"
)

var SessionMap map[string]*Session

type Session struct {
	sync.Mutex

	Time      time.Time
	FileHash  string
	Status    string
	ChunkInfo map[string]*Chunk // mapping chunkHash with Chunk info
}

type Chunk struct {
	sync.Mutex

	Challenge *StorageChallenge
	ChannelID *ledgerPb.ChannelID
	Payer     peer.ID
	Receiver  peer.ID
	Price     int64
	State     string
	Err       error
}

func init() {
	SessionMap = make(map[string]*Session)
}

func NewSessionID() (string, error) {
	seid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return seid.String(), nil
}

func (s *Session) NewSession(ssID string) {
	s.Lock()
	defer s.Unlock()

	s.Time = time.Now()
	s.Status = "init"
	s.ChunkInfo = make(map[string]*Chunk)
	SessionMap[ssID] = s
}

func (s *Session) SetFileHash(fileHash string) {
	s.Lock()
	defer s.Unlock()

	s.FileHash = fileHash
}

func (s *Session) NewChunk(hash string, payerPid peer.ID, recvPid peer.ID, channelID *ledgerPb.ChannelID, price int64) (*Chunk, error) {
	s.Lock()
	defer s.Unlock()

	chunk, ok := s.ChunkInfo[hash]
	if !ok {
		chunk = &Chunk{
			ChannelID: channelID,
			Payer:     payerPid,
			Receiver:  recvPid,
			Price:     price,
			State:     "init",
		}
		s.ChunkInfo[hash] = chunk
	}
	s.Time = time.Now()
	return chunk, nil
}

func (c *Chunk) SetChallenge(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI, cid cidlib.Cid) (*StorageChallenge, error) {
	c.Lock()
	defer c.Unlock()

	var sch *StorageChallenge
	var err error
	// if the chunk hasn't been generated challenge before
	if c.Challenge == nil {
		sch, err = NewStorageChallenge(ctx, n, api, cid)
		if err != nil {
			return nil, err
		}
	}
	c.Challenge = sch
	if err = sch.GenChallenge(); err != nil {
		return nil, err
	}
	return sch, nil
}

func (c *Chunk) UpdateChallenge(sch *StorageChallenge) {
	c.Lock()
	defer c.Unlock()

	c.Challenge = sch
}
