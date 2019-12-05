package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core"
	ledgerPb "github.com/tron-us/go-btfs-common/protos/ledger"

	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

var GlobalSession *SessionMap
var StdChunkStateFlow [7]*FlowControl
var StdSessionStateFlow [4]*FlowControl

const (
	// chunk state
	InitState      = 0
	UploadState    = 1
	ChallengeState = 2
	SolveState     = 3
	VerifyState    = 4
	PaymentState   = 5
	CompleteState  = 6

	// session status
	InitStatus     = 0
	UploadStatus   = 1
	CompleteStatus = 2
	ErrStatus      = 3
)

type FlowControl struct {
	State   string
	TimeOut time.Duration
}

type SessionMap struct {
	sync.Mutex
	Map map[string]*Session
}

type Session struct {
	sync.Mutex

	Time              time.Time
	FileHash          cidlib.Cid
	Status            string
	ChunkInfo         map[string]*Chunk // mapping chunkHash with Chunk info
	CompleteChunks    int
	CompleteContracts int
	RetryQueue        *RetryQueue

	SessionStatusChan chan StatusChan
}

type StatusChan struct {
	CurrentStep int
	Succeed     bool
	Err         error
}

type Chunk struct {
	sync.Mutex

	Challenge      *StorageChallenge
	ChannelID      *ledgerPb.ChannelID
	SignedContract []byte
	Payer          peer.ID
	Receiver       peer.ID
	Price          int64
	State          int
	Proof          string
	Time           time.Time
	Err            error

	RetryChan chan *StepRetryChan
}

type StepRetryChan struct {
	CurrentStep       int
	Succeed           bool
	ClientErr         error
	HostErr           error
	SessionTimeOutErr error
}

func init() {
	GlobalSession = &SessionMap{}
	GlobalSession.Map = make(map[string]*Session)
	// init chunk state
	StdChunkStateFlow[InitState] = &FlowControl{
		State:   "init",
		TimeOut: 10 * time.Second}
	StdChunkStateFlow[UploadState] = &FlowControl{
		State:   "upload",
		TimeOut: 10 * time.Second}
	StdChunkStateFlow[ChallengeState] = &FlowControl{
		State:   "challenge",
		TimeOut: 10 * time.Second}
	StdChunkStateFlow[SolveState] = &FlowControl{
		State:   "solve",
		TimeOut: 30 * time.Second}
	StdChunkStateFlow[VerifyState] = &FlowControl{
		State:   "verify",
		TimeOut: time.Second}
	StdChunkStateFlow[PaymentState] = &FlowControl{
		State:   "payment",
		TimeOut: 10 * time.Second}
	StdChunkStateFlow[CompleteState] = &FlowControl{
		State:   "complete",
		TimeOut: 5 * time.Second}
	// init session status
	StdSessionStateFlow[InitStatus] = &FlowControl{
		State:   "init",
		TimeOut: time.Minute}
	StdSessionStateFlow[UploadStatus] = &FlowControl{
		State:   "upload",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[CompleteStatus] = &FlowControl{
		State: "complete"}
	StdSessionStateFlow[ErrStatus] = &FlowControl{
		State: "error",
	}
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

func (sm *SessionMap) Remove(ssID string, chunkHash string) {
	sm.Lock()
	defer sm.Unlock()

	if ss := sm.Map[ssID]; ss != nil {
		if chunkHash != "" {
			ss.RemoveChunk(chunkHash)
		}
		if len(ss.ChunkInfo) == 0 {
			delete(sm.Map, ssID)
		}
	}
}

func NewSessionID() (string, error) {
	ssid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return ssid.String(), nil
}

func (ss *Session) new() {
	ss.Lock()
	defer ss.Unlock()

	ss.Time = time.Now()
	ss.Status = "init"
	ss.ChunkInfo = make(map[string]*Chunk)
	ss.SessionStatusChan = make(chan StatusChan)
}

func (ss *Session) CompareAndSwap(desiredStatus int, targetStatus int) bool {
	ss.Lock()
	defer ss.Unlock()

	// if current status isn't expected status,
	// can't setting new status
	if StdSessionStateFlow[desiredStatus].State != ss.Status {
		return false
	} else {
		ss.Status = StdSessionStateFlow[targetStatus].State
		return true
	}
}

func (ss *Session) SetRetryQueue(q *RetryQueue) {
	ss.Lock()
	defer ss.Unlock()

	ss.RetryQueue = q
}

func (ss *Session) GetRetryQueue() *RetryQueue {
	ss.Lock()
	defer ss.Unlock()

	return ss.RetryQueue
}

func (ss *Session) UpdateCompleteChunkNum(diff int) {
	ss.Lock()
	defer ss.Unlock()

	ss.CompleteChunks += diff
}

func (ss *Session) GetCompleteChunks() int {
	ss.Lock()
	defer ss.Unlock()

	return ss.CompleteChunks
}

func (ss *Session) SetFileHash(fileHash cidlib.Cid) {
	ss.Lock()
	defer ss.Unlock()

	ss.FileHash = fileHash
}

func (ss *Session) GetFileHash() cidlib.Cid {
	ss.Lock()
	defer ss.Unlock()

	return ss.FileHash
}

func (ss *Session) IncrementContract(chunkHash string, contracts []byte) error {
	ss.Lock()
	defer ss.Unlock()

	chunk := ss.ChunkInfo[chunkHash]
	if chunk == nil {
		return fmt.Errorf("chunk does not exists")
	}
	chunk.SetSignedContract(contracts)
	ss.CompleteContracts++
	return nil
}

func (ss *Session) GetCompleteContractNum() int {
	ss.Lock()
	defer ss.Unlock()

	return ss.CompleteContracts
}

func (ss *Session) SetStatus(status int) {
	ss.Lock()
	defer ss.Unlock()

	ss.Status = StdSessionStateFlow[status].State
}
func (ss *Session) GetStatus() string {
	ss.Lock()
	defer ss.Unlock()

	return ss.Status
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

func (ss *Session) RemoveChunk(hash string) {
	ss.Lock()
	defer ss.Unlock()

	if ss.ChunkInfo[hash] != nil {
		delete(ss.ChunkInfo, hash)
	}
}

func (ss *Session) GetOrDefault(hash string) *Chunk {
	ss.Lock()
	defer ss.Unlock()

	if ss.ChunkInfo[hash] == nil {
		c := &Chunk{}
		c.RetryChan = make(chan *StepRetryChan)
		c.Time = time.Now()
		c.State = InitState
		ss.ChunkInfo[hash] = c
		return c
	}
	return ss.ChunkInfo[hash]
}

func (c *Chunk) UpdateChunk(payerPid peer.ID, recvPid peer.ID, price int64) {
	c.Lock()
	defer c.Unlock()

	c.Payer = payerPid
	c.Receiver = recvPid
	c.Price = price
	c.Time = time.Now()
}

func (c *Chunk) SetSignedContract(contract []byte) {
	c.Lock()
	defer c.Unlock()

	c.SignedContract = contract
}

// used on client to record a new challenge
func (c *Chunk) SetChallenge(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI,
	rootCid, shardCid cidlib.Cid) (*StorageChallenge, error) {
	c.Lock()
	defer c.Unlock()

	var sch *StorageChallenge
	var err error
	// if the chunk hasn't been generated challenge before
	if c.Challenge == nil {
		sch, err = NewStorageChallenge(ctx, n, api, rootCid, shardCid)
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

func (c *Chunk) SetState(state int) {
	c.Lock()
	defer c.Unlock()

	c.State = state
	c.Time = time.Now()
}

func (c *Chunk) GetState() string {
	c.Lock()
	defer c.Unlock()

	return StdChunkStateFlow[c.State].State
}

func (c *Chunk) SetPrice(price int64) {
	c.Lock()
	defer c.Unlock()

	c.Price = price
}

func (c *Chunk) GetPrice() int64 {
	c.Lock()
	defer c.Unlock()

	return c.Price
}

func (c *Chunk) SetProof(proof string) {
	c.Lock()
	defer c.Unlock()

	c.Proof = proof
}

func (c *Chunk) GetProof() string {
	c.Lock()
	defer c.Unlock()

	return c.Proof
}

func (c *Chunk) SetTime(time time.Time) {
	c.Lock()
	defer c.Unlock()

	c.Time = time
}

func (c *Chunk) GetTime() time.Time {
	c.Lock()
	defer c.Unlock()

	return c.Time
}
