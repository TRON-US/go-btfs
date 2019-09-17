package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core"
	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/libp2p/go-libp2p-core/peer"
)

var GlobalSession *SessionMap

type SessionMap struct {
	sync.Mutex
	Map map[string]*Session
}

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
	Time      time.Time
}

func init() {
	GlobalSession = &SessionMap{}
	GlobalSession.Map = make(map[string]*Session)
}

func (sm *SessionMap) PutSession(ssID string, ss *Session) {
	sm.Lock()
	defer sm.Unlock()

	if ss == nil {
		ss = &Session{}
	}
	sm.Map[ssID] = ss
}

func (sm *SessionMap) GetSession(ssID string) (*Session, error) {
	sm.Lock()
	defer sm.Unlock()

	if sm.Map[ssID] == nil {
		return nil, fmt.Errorf("session id doesn't exist")
	}
	return sm.Map[ssID], nil
}

func (sm *SessionMap) GetOrDefault(ssID string) *Session {
	sm.Lock()
	defer sm.Unlock()

	if sm.Map[ssID] == nil {
		ss := &Session{}
		ss.new()
		sm.Map[ssID] = ss
		return ss
	}
	return sm.Map[ssID]
}

func NewSessionID() (string, error) {
	seid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return seid.String(), nil
}

func (ss *Session) new() {
	ss.Lock()
	defer ss.Unlock()

	ss.Time = time.Now()
	ss.Status = "init"
	ss.ChunkInfo = make(map[string]*Chunk)
}

func (ss *Session) SetFileHash(fileHash string) {
	ss.Lock()
	defer ss.Unlock()

	ss.FileHash = fileHash
}

func (ss *Session) PutChunk(hash string, chunk *Chunk) {
	ss.Lock()
	defer ss.Unlock()

	ss.ChunkInfo[hash] = chunk
	ss.Time = time.Now()
}

func (ss *Session) GetChunk(hash string) (*Chunk, error) {
	ss.Lock()
	defer ss.Unlock()

	if ss.ChunkInfo[hash] == nil {
		return nil, fmt.Errorf("chunk hash doesn't exist ")
	}
	return ss.ChunkInfo[hash], nil
}

func (ss *Session) GetOrDefault(hash string) *Chunk {
	ss.Lock()
	defer ss.Unlock()

	if ss.ChunkInfo[hash] == nil {
		c := &Chunk{}
		c.Time = time.Now()
		c.State = "init"
		ss.ChunkInfo[hash] = c
		return c
	}
	return ss.ChunkInfo[hash]
}

func (c *Chunk) NewChunk(payerPid peer.ID, recvPid peer.ID, channelID *ledgerPb.ChannelID, price int64) {
	c.Lock()
	defer c.Unlock()

	c.ChannelID = channelID
	c.Payer = payerPid
	c.Receiver = recvPid
	c.Price = price
	c.State = "init"
	c.Time = time.Now()
}

// used on client to record a new challenge
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
		c.Challenge = sch
	} else {
		sch = c.Challenge
	}

	if err = sch.GenChallenge(); err != nil {
		return nil, err
	}
	c.Time = time.Now()
	return sch, nil
}

// usually used on host, to record host challenge info
func (c *Chunk) UpdateChallenge(sch *StorageChallenge) {
	c.Lock()
	defer c.Unlock()

	c.Challenge = sch
	c.Time = time.Now()
}
