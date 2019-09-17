package storage

import (
	"context"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
)

var SessionMap map[string]*Session

type Session struct {
	sync.Mutex

	Time      time.Time
	FileHash  string
	Status    string
	Chunk     []*Chunks // will merge chunk with challenge in session ticket
	Challenge map[string]*StorageChallenge
}

type Chunks struct {
	ChunkHash string
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

func (s *Session) NewSession(ssID string, fileHash string) error {
	s.Lock()
	defer s.Unlock()

	ch := make(map[string]*StorageChallenge)
	session := &Session{
		Time:      time.Now(),
		Status:    "init",
		FileHash:  fileHash,
		Challenge: ch,
	}
	SessionMap[ssID] = session
	return nil
}

func (s *Session) SetChallenge(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI, cid cid.Cid) (*StorageChallenge, error) {
	s.Lock()
	defer s.Unlock()

	hash := cid.String()
	var sch *StorageChallenge
	var err error
	// if the chunk hasn't been generated challenge before
	if _, ok := s.Challenge[hash]; !ok {
		sch, err = NewStorageChallenge(ctx, n, api, cid)
		if err != nil {
			return nil, err
		}
		s.Challenge[hash] = sch
	} else {
		sch = s.Challenge[hash]
	}

	if err = sch.GenChallenge(); err != nil {
		return nil, err
	}
	s.Time = time.Now()
	return sch, nil
}
