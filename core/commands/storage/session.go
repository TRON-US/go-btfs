package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/alecthomas/units"
	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
)

var GlobalSession *SessionMap
var StdStateFlow [3]*FlowControl
var StdSessionStateFlow [6]*FlowControl

const (
	// prefixes for datastore persistency keys
	HostStoragePrefix   = "/host-storage/"
	RenterStoragePrefix = "/renter-storage/"
	// secondary path segments after prefix for datastore persistency keys
	FileContractsStoreSeg = "file-contracts/"
	ShardsStoreSeg        = "shards/"

	// shard state
	InitState     = 0
	ContractState = 1
	CompleteState = 2

	// session status
	InitStatus     = 0
	SubmitStatus   = 1
	PayStatus      = 2
	GuardStatus    = 3
	CompleteStatus = 4
	ErrStatus      = 5
)

type FlowControl struct {
	State   string
	TimeOut time.Duration
}

type SessionMap struct {
	sync.Mutex
	Map map[string]*FileContracts
}

type FileContracts struct {
	sync.Mutex

	ID                string
	Time              time.Time
	GuardContracts    []*guardpb.Contract
	Renter            peer.ID
	FileHash          cidlib.Cid
	Status            string
	StatusMessage     string            // most likely error or notice
	ShardInfo         map[string]*Shard // mapping chunkHash with Shards info
	CompleteChunks    int
	CompleteContracts int

	RetryQueue        *RetryQueue     `json:"-"`
	SessionStatusChan chan StatusChan `json:"-"`
}

type StatusChan struct {
	CurrentStep int
	Succeed     bool
	Err         error
}

type Shard struct {
	sync.Mutex

	ShardHash            cidlib.Cid
	ShardIndex           int
	ContractID           string
	SignedEscrowContract []byte
	Receiver             peer.ID
	Price                int64
	TotalPay             int64
	State                int
	ShardSize            int64
	StorageLength        int64
	ContractLength       time.Duration
	StartTime            time.Time
	Err                  error
	Challenge            *StorageChallenge

	RetryChan chan *StepRetryChan `json:"-"`
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

	GlobalSession.Map = make(map[string]*FileContracts)
	// init chunk state
	StdStateFlow[InitState] = &FlowControl{
		State:   "init",
		TimeOut: 5 * time.Minute}
	StdStateFlow[ContractState] = &FlowControl{
		State:   "contract",
		TimeOut: 5 * time.Minute}
	StdStateFlow[CompleteState] = &FlowControl{
		State:   "complete",
		TimeOut: time.Minute}
	// init session status
	StdSessionStateFlow[InitStatus] = &FlowControl{
		State:   "init",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[SubmitStatus] = &FlowControl{
		State:   "submit",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[PayStatus] = &FlowControl{
		State:   "pay",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[GuardStatus] = &FlowControl{
		State:   "guard",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[CompleteStatus] = &FlowControl{
		State: "complete"}
	StdSessionStateFlow[ErrStatus] = &FlowControl{
		State: "error",
	}
}

func (sm *SessionMap) PutSession(ssID string, ss *FileContracts) {
	sm.Lock()
	defer sm.Unlock()

	if ss == nil {
		ss = &FileContracts{}
	}
	sm.Map[ssID] = ss
}

func (sm *SessionMap) GetSession(node *core.IpfsNode, prefix, ssID string) (*FileContracts, error) {
	sm.Lock()
	defer sm.Unlock()

	if sm.Map[ssID] == nil {
		f, err := GetFileMetaFromDatastore(node, prefix, ssID)
		if err != nil {
			return nil, err
		}
		if f == nil {
			return nil, fmt.Errorf("session id doesn't exist")
		}
		return f, nil
	}
	return sm.Map[ssID], nil
}

func (sm *SessionMap) GetOrDefault(ssID string, pid peer.ID) *FileContracts {
	sm.Lock()
	defer sm.Unlock()

	if sm.Map[ssID] == nil {
		ss := NewFileContracts(ssID, pid)
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
			ss.RemoveShard(chunkHash)
		}
		if len(ss.ShardInfo) == 0 {
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

// GetFileMetaFromDatastore retrieves persisted session/contract information from datastore
func GetFileMetaFromDatastore(node *core.IpfsNode, prefix, ssID string) (*FileContracts, error) {
	rds := node.Repo.Datastore()
	value, err := rds.Get(ds.NewKey(prefix + FileContractsStoreSeg + ssID))
	if err != nil {
		return nil, err
	}

	f := new(FileContracts)
	err = json.Unmarshal(value, f)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// GetShardInfoFromDatastore retrieves persisted shard information from datastore
func GetShardInfoFromDatastore(node *core.IpfsNode, prefix, shardHash string) (*Shard, error) {
	rds := node.Repo.Datastore()
	value, err := rds.Get(ds.NewKey(prefix + ShardsStoreSeg + shardHash))
	if err != nil {
		return nil, err
	}

	s := new(Shard)
	err = json.Unmarshal(value, s)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// PersistFileMetaToDatastore saves session/contract information into datastore
func PersistFileMetaToDatastore(node *core.IpfsNode, prefix string, ssID string) error {
	rds := node.Repo.Datastore()
	ss, err := GlobalSession.GetSession(node, prefix, ssID)
	if err != nil {
		return err
	}
	fileContractsBytes, err := json.Marshal(ss)
	if err != nil {
		return err
	}
	err = rds.Put(ds.NewKey(prefix+FileContractsStoreSeg+ssID), fileContractsBytes)
	if err != nil {
		return err
	}
	for shardHash, shardInfo := range ss.ShardInfo {
		shardBytes, err := json.Marshal(shardInfo)
		if err != nil {
			return err
		}
		err = rds.Put(ds.NewKey(prefix+ShardsStoreSeg+shardHash), shardBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func NewFileContracts(id string, pid peer.ID) *FileContracts {
	return &FileContracts{
		ID:                id,
		Renter:            pid,
		Time:              time.Now(),
		Status:            "init",
		ShardInfo:         make(map[string]*Shard),
		SessionStatusChan: make(chan StatusChan),
	}
}

func (ss *FileContracts) CompareAndSwap(desiredStatus int, targetStatus int) bool {
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

func (ss *FileContracts) SetRetryQueue(q *RetryQueue) {
	ss.Lock()
	defer ss.Unlock()

	ss.RetryQueue = q
}

func (ss *FileContracts) GetRetryQueue() *RetryQueue {
	ss.Lock()
	defer ss.Unlock()

	return ss.RetryQueue
}

func (ss *FileContracts) UpdateCompleteShardNum(diff int) {
	ss.Lock()
	defer ss.Unlock()

	ss.CompleteChunks += diff
}

func (ss *FileContracts) GetCompleteShards() int {
	ss.Lock()
	defer ss.Unlock()

	return ss.CompleteChunks
}

func (ss *FileContracts) SetFileHash(fileHash cidlib.Cid) {

	ss.Lock()
	defer ss.Unlock()

	ss.FileHash = fileHash
}

func (ss *FileContracts) GetFileHash() cidlib.Cid {

	ss.Lock()
	defer ss.Unlock()

	return ss.FileHash
}

func (ss *FileContracts) IncrementContract(shardKey string, contracts []byte, guardContract *guardpb.Contract) (bool, error) {
	ss.Lock()
	defer ss.Unlock()

	ss.GuardContracts = append(ss.GuardContracts, guardContract)
	chunk := ss.ShardInfo[shardKey]
	if chunk == nil {
		return false, fmt.Errorf("shard does not exists")
	}
	if chunk.SetSignedContract(contracts) {
		ss.CompleteContracts++
		return true, nil
	}
	return false, nil
}

func (ss *FileContracts) GetGuardContracts() []*guardpb.Contract {
	ss.Lock()
	defer ss.Unlock()

	return ss.GuardContracts
}

func (ss *FileContracts) GetCompleteContractNum() int {
	ss.Lock()
	defer ss.Unlock()

	return ss.CompleteContracts
}

func (ss *FileContracts) SetStatus(status int) {
	ss.Lock()
	defer ss.Unlock()

	ss.Status = StdSessionStateFlow[status].State
}

func (ss *FileContracts) SetStatusWithError(status int, err error) {
	ss.Lock()
	defer ss.Unlock()

	ss.Status = StdSessionStateFlow[status].State
	ss.StatusMessage = err.Error()
}

func (ss *FileContracts) GetStatusAndMessage() (string, string) {
	ss.Lock()
	defer ss.Unlock()

	return ss.Status, ss.StatusMessage
}

func (ss *FileContracts) GetShard(hash string) (*Shard, error) {
	ss.Lock()
	defer ss.Unlock()

	si, ok := ss.ShardInfo[hash]
	if !ok {
		return nil, fmt.Errorf("chunk hash doesn't exist: %s", hash)
	}
	return si, nil
}

func (ss *FileContracts) RemoveShard(hash string) {
	ss.Lock()
	defer ss.Unlock()

	delete(ss.ShardInfo, hash)
}

func (ss *FileContracts) GetOrDefault(shardHash string, shardIndex int, shardSize int64, length int64) (*Shard, error) {
	ss.Lock()
	defer ss.Unlock()

	shardKey := shardHash + strconv.Itoa(shardIndex)
	if ss.ShardInfo[shardKey] == nil {
		c := &Shard{}
		sh, err := cidlib.Parse(shardHash)
		if err != nil {
			return nil, err
		}
		c.ShardHash = sh
		c.ShardIndex = shardIndex
		c.RetryChan = make(chan *StepRetryChan)
		c.StartTime = time.Now()
		c.State = InitState
		c.ShardSize = shardSize
		c.StorageLength = length
		c.ContractLength = time.Duration(length*24) * time.Hour
		if err := c.SetContractID(); err != nil {
			return nil, err
		}
		ss.ShardInfo[shardKey] = c
		return c, nil
	} else {
		return ss.ShardInfo[shardKey], nil
	}
}

func (c *Shard) SetPrice(price int64) {
	c.Lock()
	defer c.Unlock()

	c.Price = price
	totalPay := int64(float64(c.ShardSize) / float64(units.GiB) * float64(price) * float64(c.StorageLength))
	if totalPay == 0 {
		c.TotalPay = 1
	} else {
		c.TotalPay = totalPay
	}
}

func (c *Shard) SetContractID() error {
	c.Lock()
	defer c.Unlock()

	contractID, err := NewSessionID()
	if err != nil {
		return err
	}
	c.ContractID = contractID
	return nil
}

func (c *Shard) GetContractID() string {
	c.Lock()
	defer c.Unlock()

	return c.ContractID
}

func (c *Shard) UpdateShard(recvPid peer.ID) {
	c.Lock()
	defer c.Unlock()

	c.Receiver = recvPid
	c.StartTime = time.Now()
}

func (c *Shard) SetSignedContract(contract []byte) bool {
	c.Lock()
	defer c.Unlock()

	if c.SignedEscrowContract == nil {
		c.SignedEscrowContract = contract
		return true
	} else {
		return false
	}
}

func (c *Shard) GetChallengeOrNew(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	fileHash cidlib.Cid) (*StorageChallenge, error) {
	c.Lock()
	defer c.Unlock()

	if c.Challenge == nil {
		sc, err := NewStorageChallenge(ctx, node, api, fileHash, c.ShardHash)
		if err != nil {
			return nil, err
		}
		c.Challenge = sc
	}
	return c.Challenge, nil
}

func (c *Shard) GetChallengeResponseOrNew(ctx context.Context, node *core.IpfsNode, api coreiface.CoreAPI,
	fileHash cidlib.Cid, init bool) (*StorageChallenge, error) {
	c.Lock()
	defer c.Unlock()

	if c.Challenge == nil {
		sc, err := NewStorageChallengeResponse(ctx, node, api, fileHash, c.ShardHash, "", init)
		if err != nil {
			return nil, err
		}
		c.Challenge = sc
	}
	return c.Challenge, nil
}

func (c *Shard) SetState(state int) {
	c.Lock()
	defer c.Unlock()

	c.State = state
	c.StartTime = time.Now()
}

func (c *Shard) GetStateStr() string {
	c.Lock()
	defer c.Unlock()

	return StdStateFlow[c.State].State
}

func (c *Shard) GetState() int {
	c.Lock()
	defer c.Unlock()

	return c.State
}

func (c *Shard) GetTotalAmount() int64 {
	c.Lock()
	defer c.Unlock()

	return c.TotalPay
}

func (c *Shard) SetTime(time time.Time) {
	c.Lock()
	defer c.Unlock()

	c.StartTime = time
}

func (c *Shard) GetTime() time.Time {
	c.Lock()
	defer c.Unlock()

	return c.StartTime
}
