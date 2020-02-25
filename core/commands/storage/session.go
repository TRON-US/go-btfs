package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/alecthomas/units"
	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	// prefixes for datastore persistency keys
	HostStoragePrefix   = "/host-storage/"
	RenterStoragePrefix = "/renter-storage/"
	// secondary path segments after prefix for datastore persistency keys
	FileContractsStoreSeg = "file-contracts/"
	ShardsStoreSeg        = "shards/"
)

const (
	// session mode
	RegularMode = iota
	RepairMode
	OfflineSignMode
)

const (
	// host sync mode
	NonCustomMode = iota
	CustomMode
)

const (
	// shard state
	UninitializedState = iota
	SignReadyEscrowState
	SignProcessEscrowState
	SignReadyGuardState
	SignProcessGaurdState
	InitState
	ContractState
	CompleteState
)

// WARNING! the above states should have corresponding session statuses below for retryProcessOffSign()

const (
	// session status
	UninitializedStatus = iota
	InitSignReadyForEscrowStatus
	InitSignProcessForEscrowStatus
	InitSignReadyForGuardStatus
	InitSignProcessForGuardStatus
	InitStatus // if offline signing, set this flag when all the shards are complete in offline signing
	SubmitStatus
	BalanceSignReadyStatus
	BalanceSignProcessStatus
	PayChannelSignReadyStatus
	PayChannelSignProcessStatus
	PayChannelStatus
	PayReqSignReadyStatus
	PayReqSignProcessStatus
	PayStatus
	GuardSignReadyStatus
	GuardSignProcessStatus
	GuardStatus
	CompleteStatus // This should be the send to the last entry for controlSessionTimeout() to work.
	ErrStatus      // This should be the last entry for StdSessionStateFlow array to work
)

const (
	// offline signing status
	OfflineRetrySignUninitialized = iota
	OfflineRetrySignReady
	OfflineRetrySignProcess
)

var log = logging.Logger("core/commands/storage")

var GlobalSession *SessionMap

//var StdStateFlow [3]*FlowControl
var StdStateFlow [CompleteState + 1]*FlowControl
var StdSessionStateFlow [ErrStatus + 1]*FlowControl
var StdRetrySignStateFlow [OfflineRetrySignProcess + 1]*FlowControl
var helpTextMap = map[int]string{
	UninitializedStatus:            "Uninitialized",
	InitSignReadyForEscrowStatus:   "Init-sign for Escrow is ready",
	InitSignProcessForEscrowStatus: "Init-sign for Escrow is in process",
	InitSignReadyForGuardStatus:    "Init-sign for Guard is ready",
	InitSignProcessForGuardStatus:  "Init-sign for Guard is in process",
	InitStatus:                     "Initialization",
	SubmitStatus:                   "Submission is done",
	BalanceSignReadyStatus:         "Balance-sign for Escrow is ready",
	BalanceSignProcessStatus:       "Balance-sign for Escrow is in process",
	PayChannelSignReadyStatus:      "Pay-channel-sign for Escrow is done",
	PayChannelSignProcessStatus:    "Pay-channel-sign for Escrow is in process",
	PayChannelStatus:               "Pay-channel-sign for Escrow is done",
	PayReqSignReadyStatus:          "Pay-request-sign for Escrow is done",
	PayReqSignProcessStatus:        "Pay-request-sign for Escrow is in process",
	PayStatus:                      "Pay-sign for Escrow is done",
	GuardSignReadyStatus:           "Pay-sign for Guard is ready",
	GuardSignProcessStatus:         "Pay-sign for Guard is in process",
	GuardStatus:                    "Guard operation is done",
	CompleteStatus:                 "Session is complete",
	ErrStatus:                      "Error occurred",
}

const DEBUG = false

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
	EscrowContracts   []*escrowpb.EscrowContract
	GuardContracts    []*guardpb.Contract
	Renter            peer.ID
	FileHash          cidlib.Cid
	StatusIndex       int
	Status            string
	StatusMessage     string            // most likely error or notice
	ShardInfo         map[string]*Shard // mapping (shard hash + index) with shard info
	CompleteChunks    int
	CompleteContracts int

	RetryQueue             *RetryQueue            `json:"-"`
	SessionStatusChan      chan StatusChanMessage `json:"-"`
	SessionStatusReplyChan chan int               `json:"-"`
	RetryMonitorCtx        context.Context

	RunMode         int
	RetrySignStatus string
	OfflineCB       *OfflineControlBlock
}
type OfflineControlBlock struct {
	OfflinePeerID           peer.ID
	OfflineNonceTimestamp   uint64
	OfflineSessionSignature string
	OffSignReadyShards      int
	OfflineUnsigned         *GetUnsignedRes
	OfflineSigned           string
	OfflineSignEscrowChan   chan string `json:"-"` // Use this for broadcasting session status change
	OfflineSignGuardChan    chan string `json:"-"`
	OfflineInitSigDoneChan  chan string `json:"-"`
	OfflinePaySignChan      chan string `json:"-"`
}

type Contract struct {
	Key          string `json:"key"`
	ContractData string `json:"contract"`
}

type GetUnsignedRes struct {
	Opcode   string
	Unsigned string
	Price    int64
}

type StatusChanMessage struct {
	CurrentStep int
	Succeed     bool
	Err         error
	SyncMode    bool
}

type Shard struct {
	sync.Mutex

	ShardHash                cidlib.Cid
	ShardIndex               int
	ContractID               string
	SignedEscrowContract     []byte
	Receiver                 peer.ID
	Price                    int64
	TotalPay                 int64
	State                    int
	ShardSize                int64
	StorageLength            int64
	ContractLength           time.Duration
	StartTime                time.Time
	Err                      error
	Challenge                *StorageChallenge
	CandidateHost            *HostNode
	UnsignedEscrowContract   *escrowpb.EscrowContract
	UnsignedGuardContract    *guardpb.ContractMeta
	SignedBytes              []byte
	HalfSignedEscrowContract []byte
	HalfSignedGuardContract  []byte

	RetryChan chan *StepRetryChanMessage `json:"-"`
}

type StepRetryChanMessage struct {
	CurrentStep       int
	Succeed           bool
	ClientErr         error
	HostErr           error
	SessionTimeOutErr error
}

func init() {
	GlobalSession = &SessionMap{}

	GlobalSession.Map = make(map[string]*FileContracts)
	// uninitialized shard state
	StdStateFlow[UninitializedState] = &FlowControl{
		State:   "uninitialized",
		TimeOut: 3 * time.Minute}
	StdStateFlow[SignReadyEscrowState] = &FlowControl{
		State:   "signReadyEscrow",
		TimeOut: 3 * time.Minute}
	StdStateFlow[SignProcessEscrowState] = &FlowControl{
		State:   "signProcessEscrow",
		TimeOut: 3 * time.Minute}
	StdStateFlow[SignReadyGuardState] = &FlowControl{
		State:   "signReadyGuard",
		TimeOut: 3 * time.Minute}
	StdStateFlow[SignProcessGaurdState] = &FlowControl{
		State:   "signProcessGuard",
		TimeOut: 3 * time.Minute}
	// init shard state
	StdStateFlow[InitState] = &FlowControl{
		State:   "init",
		TimeOut: 30 * time.Second}
	StdStateFlow[ContractState] = &FlowControl{
		State:   "contract",
		TimeOut: 1 * time.Minute}
	StdStateFlow[CompleteState] = &FlowControl{
		State:   "complete",
		TimeOut: 1 * time.Minute}
	// uninitialized session status
	// Note that "init"'s Timeout should be larger than the total of
	// each StdStatFlow state * RetryLimit.
	StdSessionStateFlow[UninitializedStatus] = &FlowControl{
		State: "uninitialized"}
	StdSessionStateFlow[InitSignReadyForEscrowStatus] = &FlowControl{
		State: "initSignReadyEscrow"}
	StdSessionStateFlow[InitSignProcessForEscrowStatus] = &FlowControl{
		State: "initSignProcessEscrow"}
	StdSessionStateFlow[InitSignReadyForGuardStatus] = &FlowControl{
		State: "initSignReadyGuard"}
	StdSessionStateFlow[InitSignProcessForGuardStatus] = &FlowControl{
		State: "initSignProcessGuard"}
	StdSessionStateFlow[InitStatus] = &FlowControl{
		State:   "init",
		TimeOut: 3 * time.Minute}
	StdSessionStateFlow[SubmitStatus] = &FlowControl{
		State:   "submit",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[BalanceSignReadyStatus] = &FlowControl{
		State:   "balanceSignReady",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[BalanceSignProcessStatus] = &FlowControl{
		State:   "balanceSignDProcess",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[PayChannelSignReadyStatus] = &FlowControl{
		State:   "payChannelSignReady",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[PayChannelSignProcessStatus] = &FlowControl{
		State:   "payChannelSignProcess",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[PayChannelStatus] = &FlowControl{
		State:   "paychannel",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[PayReqSignReadyStatus] = &FlowControl{
		State:   "payRequestSignReady",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[PayReqSignProcessStatus] = &FlowControl{
		State:   "payRequestSignProcess",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[PayStatus] = &FlowControl{
		State:   "pay",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[GuardSignReadyStatus] = &FlowControl{
		State:   "guardSignReady",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[GuardSignProcessStatus] = &FlowControl{
		State:   "guardSignProcess",
		TimeOut: 5 * time.Minute}
	StdSessionStateFlow[GuardStatus] = &FlowControl{
		State:   "guard",
		TimeOut: 15 * time.Minute}
	StdSessionStateFlow[CompleteStatus] = &FlowControl{
		State: "complete",
		// end, no timeout
	}
	StdSessionStateFlow[ErrStatus] = &FlowControl{
		State: "error",
		// end, no timeout
	}
	StdRetrySignStateFlow[OfflineRetrySignUninitialized] = &FlowControl{
		State:   "retrySignUninitialized",
		TimeOut: 3 * time.Minute}
	StdRetrySignStateFlow[OfflineRetrySignReady] = &FlowControl{
		State:   "retrySignReady",
		TimeOut: 3 * time.Minute}
	StdRetrySignStateFlow[OfflineRetrySignProcess] = &FlowControl{
		State:   "retrySignProcess",
		TimeOut: 3 * time.Minute}
}

func (sm *SessionMap) PutSession(ssID string, ss *FileContracts) {
	sm.Lock()
	defer sm.Unlock()

	if ss == nil {
		ss = &FileContracts{}
	}
	sm.Map[ssID] = ss
}

func (sm *SessionMap) GetSessionWithoutLock(node *core.IpfsNode, prefix, ssID string) (*FileContracts, error) {
	return sm.doGetSession(node, prefix, ssID)
}

func (sm *SessionMap) doGetSession(node *core.IpfsNode, prefix, ssID string) (*FileContracts, error) {
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

func (sm *SessionMap) GetSession(node *core.IpfsNode, prefix, ssID string) (*FileContracts, error) {
	sm.Lock()
	defer sm.Unlock()
	return sm.doGetSession(node, prefix, ssID)
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
	if prefix == HostStoragePrefix {
		if len(shardHash) != 46 {
			shardHash = shardHash[:46]
		}
	}
	key := ds.NewKey(prefix + ShardsStoreSeg + shardHash)
	value, err := rds.Get(key)
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
		if prefix == HostStoragePrefix {
			if len(shardHash) != 46 {
				shardHash = shardHash[:46]
			}
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
		ID:                     id,
		Renter:                 pid,
		Time:                   time.Now(),
		StatusIndex:            0,
		Status:                 "init",
		ShardInfo:              make(map[string]*Shard),
		SessionStatusChan:      make(chan StatusChanMessage),
		SessionStatusReplyChan: make(chan int),
	}
}

func (ss *FileContracts) Initialize(rootHash cidlib.Cid, runMode int) int {
	ss.Lock()
	defer ss.Unlock()

	ss.FileHash = rootHash
	ss.RunMode = runMode
	initSessionStatus := InitStatus
	if ss.IsOffSignRunmode() {
		// Note that we are updating the "init" status timeout for offline signing.
		StdSessionStateFlow[InitState].TimeOut = 15 * time.Minute
		ss.OfflineCB = new(OfflineControlBlock)
		initSessionStatus = UninitializedStatus
		ss.initOfflineSignChannels()
	}
	ss.StatusIndex = initSessionStatus
	ss.Status = StdSessionStateFlow[initSessionStatus].State
	ss.StatusMessage = helpTextMap[initSessionStatus]

	return initSessionStatus
}

func (ss *FileContracts) initOfflineSignChannels() error {
	if !ss.IsOffSignRunmode() {
		return errors.New("it is not offline sign mode")
	}
	if ss.OfflineCB == nil {
		return errors.New("offline control block is nil")
	}
	offCB := ss.OfflineCB
	offCB.OfflineSignEscrowChan = nil
	offCB.OfflineSignEscrowChan = make(chan string)
	offCB.OfflineSignGuardChan = nil
	offCB.OfflineSignGuardChan = make(chan string)
	offCB.OfflineInitSigDoneChan = nil
	offCB.OfflineInitSigDoneChan = make(chan string)
	offCB.OfflinePaySignChan = nil
	offCB.OfflinePaySignChan = make(chan string)

	return nil
}

func (ss *FileContracts) MoveToNextSessionStatus(sessionState StatusChanMessage) int {
	currentSessionStatus := sessionState.CurrentStep
	if ss.RunMode != OfflineSignMode {
		// If the current runMode is not OfflineSignMode, then
		// we need to skip offline sign mode related steps from the session status table.
		// The following switch is doing that task.
		switch currentSessionStatus {
		case InitStatus:
			currentSessionStatus = SubmitStatus
		case SubmitStatus:
			currentSessionStatus = PayStatus
		case PayStatus:
			currentSessionStatus = GuardStatus
		case GuardStatus:
			currentSessionStatus = CompleteStatus
		}
	} else {
		currentSessionStatus += 1
	}
	ss.SetStatus(currentSessionStatus)
	return currentSessionStatus
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
		ss.StatusIndex = targetStatus
		return true
	}
}

func (ss *FileContracts) SyncToSessionStatusMessage() {
	ss.SessionStatusReplyChan <- 1
}

func (ss *FileContracts) SessionEnded() bool {
	return StdSessionStateFlow[CompleteStatus].State == ss.Status ||
		StdSessionStateFlow[ErrStatus].State == ss.Status
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
	return ss.FileHash
}

func (ss *FileContracts) IncrementContract(shard *Shard, signedEscrowContract []byte,
	guardContract *guardpb.Contract) error {
	ss.Lock()
	defer ss.Unlock()

	// expand guard contracts storage according to number of shards
	if shard.ShardIndex >= len(ss.GuardContracts) {
		gcs := make([]*guardpb.Contract, shard.ShardIndex+1)
		copy(gcs, ss.GuardContracts)
		ss.GuardContracts = gcs
	}
	// insert contract according to shard order
	ss.GuardContracts[shard.ShardIndex] = guardContract

	if shard.SetSignedEscrowContract(signedEscrowContract) {
		ss.CompleteContracts++
		return nil
	}
	return fmt.Errorf("escrow contract is already set")
}

func (ss *FileContracts) IncrementAndCompareContract(last int, shard *Shard, signedEscrowContract []byte,
	guardContract *guardpb.Contract) (bool, error) {
	ss.Lock()
	defer ss.Unlock()

	// expand guard contracts storage according to number of shards
	if shard.ShardIndex >= len(ss.GuardContracts) {
		gcs := make([]*guardpb.Contract, shard.ShardIndex+1)
		copy(gcs, ss.GuardContracts)
		ss.GuardContracts = gcs
	}
	// insert contract according to shard order
	ss.GuardContracts[shard.ShardIndex] = guardContract

	if shard.SetSignedEscrowContract(signedEscrowContract) {
		ss.CompleteContracts++
		if ss.CompleteContracts == last {
			return true, nil
		}
		return false, nil
	}
	return false, fmt.Errorf("escrow contract is already set")
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

	ss.StatusIndex = status
	ss.Status = StdSessionStateFlow[status].State
	ss.StatusMessage = helpTextMap[status]
}

func (ss *FileContracts) SetRetrySignStatus(status int) {
	ss.Lock()
	defer ss.Unlock()

	ss.RetrySignStatus = StdRetrySignStateFlow[status].State
}

func (ss *FileContracts) SetStatusWithError(status int, err error) {
	ss.Lock()
	defer ss.Unlock()

	ss.StatusIndex = status
	ss.Status = StdSessionStateFlow[status].State
	ss.StatusMessage = err.Error()
}

func (ss *FileContracts) GetStatus(status int) (string, error) {
	ss.Lock()
	defer ss.Unlock()

	if status < 0 || status >= len(StdSessionStateFlow) {
		return "", fmt.Errorf("given index %d is out of array boundary", status)
	}
	return StdSessionStateFlow[status].State, nil
}

func (ss *FileContracts) GetCurrentStatus() int {
	ss.Lock()
	defer ss.Unlock()

	return ss.StatusIndex
}

func (ss *FileContracts) SetRunMode(m int) {
	ss.Lock()
	defer ss.Unlock()

	ss.RunMode = m
}

func (ss *FileContracts) GetRunMode() int {
	ss.Lock()
	defer ss.Unlock()

	return ss.RunMode
}

func (ss *FileContracts) GetStatusAndMessage() (string, string) {
	return ss.Status, ss.StatusMessage
}

func (ss *FileContracts) GetShard(hash string, index int) (*Shard, error) {
	ss.Lock()
	defer ss.Unlock()

	shardKey := GetShardKey(hash, index)
	si, ok := ss.ShardInfo[shardKey]
	if !ok {
		return nil, fmt.Errorf("shard hash key doesn't exist: %s", shardKey)
	}
	return si, nil
}

func (ss *FileContracts) RemoveShard(hash string) {
	ss.Lock()
	defer ss.Unlock()

	delete(ss.ShardInfo, hash)
}

func GetShardKey(shardHash string, shardIndex int) string {
	return shardHash + strconv.Itoa(shardIndex)
}

func (ss *FileContracts) GetOrDefault(shardHash string, shardIndex int,
	shardSize int64, length int64, contractId string) (*Shard, error) {
	ss.Lock()
	defer ss.Unlock()

	shardKey := GetShardKey(shardHash, shardIndex)
	if s, ok := ss.ShardInfo[shardKey]; !ok {
		c := &Shard{}
		sh, err := cidlib.Parse(shardHash)
		if err != nil {
			return nil, err
		}
		c.ShardHash = sh
		c.ShardIndex = shardIndex
		c.RetryChan = make(chan *StepRetryChanMessage)
		c.StartTime = time.Now()
		c.State = InitState
		c.ShardSize = shardSize
		c.StorageLength = length
		c.ContractLength = time.Duration(length*24) * time.Hour
		if err := c.SetContractID(contractId); err != nil {
			return nil, err
		}
		ss.ShardInfo[shardKey] = c
		return c, nil
	} else {
		return s, nil
	}
}

func (ss *FileContracts) IsOffSignRunmode() bool {
	return ss.RunMode == OfflineSignMode
}

func (ss *FileContracts) SetFOfflinePeerID(peerId peer.ID) error {
	ss.Lock()
	defer ss.Unlock()

	if ss.OfflineCB == nil {
		return errors.New("offline control block is nil")
	}
	ss.OfflineCB.OfflinePeerID = peerId
	return nil
}

func (ss *FileContracts) GetOfflinePeerID() (peer.ID, error) {
	ss.Lock()
	defer ss.Unlock()

	if ss.OfflineCB == nil {
		return "", errors.New("offline control block is nil")
	}
	return ss.OfflineCB.OfflinePeerID, nil
}

func (ss *FileContracts) SetFOfflineNonceTimestamp(nonceTimestamp uint64) error {
	ss.Lock()
	defer ss.Unlock()

	if ss.OfflineCB == nil {
		return errors.New("offline control block is nil")
	}
	ss.OfflineCB.OfflineNonceTimestamp = nonceTimestamp
	return nil
}

func (ss *FileContracts) GetOfflineNonceTimestamp() uint64 {
	ss.Lock()
	defer ss.Unlock()

	return ss.OfflineCB.OfflineNonceTimestamp
}

func (ss *FileContracts) SetFOfflineSessionSignature(sessionSignature string) {
	ss.Lock()
	defer ss.Unlock()

	ss.OfflineCB.OfflineSessionSignature = sessionSignature
}

func (ss *FileContracts) GetOfflineSessionSignature() string {
	ss.Lock()
	defer ss.Unlock()

	return ss.OfflineCB.OfflineSessionSignature
}

func (ss *FileContracts) GetOffSignReadyShards() int {
	ss.Lock()
	defer ss.Unlock()

	return ss.OfflineCB.OffSignReadyShards
}

func (ss *FileContracts) SetOffSignReadyShards(v int) {
	ss.Lock()
	defer ss.Unlock()

	ss.OfflineCB.OffSignReadyShards = v
}

func (ss *FileContracts) IncrementOffSignReadyShards() {
	ss.Lock()
	defer ss.Unlock()

	ss.OfflineCB.OffSignReadyShards++
}

func (ss *FileContracts) IncrementAndCompareOffSignReadyShards(last int) (bool, error) {
	ss.Lock()
	defer ss.Unlock()

	if ss.OfflineCB.OffSignReadyShards >= last {
		return false, fmt.Errorf("the current value is already range end: [%d]", last)
	}
	ss.OfflineCB.OffSignReadyShards++
	if ss.OfflineCB.OffSignReadyShards == last {
		return true, nil
	}
	return false, nil
}

func (ss *FileContracts) NewOfflineUnsigned() {
	ss.OfflineCB.OfflineUnsigned = new(GetUnsignedRes)
}

func (ss *FileContracts) ResetOfflineUnsigned() {
	ss.OfflineCB.OfflineUnsigned.Opcode = ""
	ss.OfflineCB.OfflineUnsigned.Unsigned = ""
	ss.OfflineCB.OfflineUnsigned.Price = 0
}

func (ss *FileContracts) UpdateSessionStatus(status int, succeed bool, err error) {
	if err != nil {
		log.Error("session error:", err)
	}
	if succeed {
		ss.MoveToNextSessionStatus(StatusChanMessage{
			CurrentStep: status,
			Succeed:     succeed,
			Err:         err,
		})
	} else {
		ss.SetStatusWithError(ErrStatus, err)
	}
}

func (ss *FileContracts) SendSessionStatusChan(status int, succeed bool, err error) {
	ss.sendSessionStatusChan(status, succeed, err, false, context.TODO())
}

func (ss *FileContracts) SendSessionStatusChanPerMode(status int, succeed bool, err error, ctx context.Context) {
	if ss.RunMode == OfflineSignMode {
		ss.sendSessionStatusChan(status, succeed, err, true, ctx)
	} else {
		ss.sendSessionStatusChan(status, succeed, err, false, context.TODO())
	}
}

func (ss *FileContracts) SendSessionStatusChanSafely(status int, succeed bool, err error, ctx context.Context) {
	ss.sendSessionStatusChan(status, succeed, err, true, ctx)
}

func (ss *FileContracts) sendSessionStatusChan(status int, succeed bool, err error, syncMode bool, ctx context.Context) {
	if err != nil {
		log.Error("session error:", err)
	}
	ss.SessionStatusChan <- StatusChanMessage{
		CurrentStep: status,
		Succeed:     succeed,
		Err:         err,
		SyncMode:    syncMode,
	}
	if syncMode {
		select {
		case <-ss.SessionStatusReplyChan:
		case <-ctx.Done():
			return
		}
	}
}

func (c *Shard) SendStepStateChan(state int, succeed bool, clientErr error, hostErr error) {
	if clientErr != nil {
		log.Error("renter error:", clientErr)
	}
	if hostErr != nil {
		log.Error("host error:", hostErr)
	}
	c.RetryChan <- &StepRetryChanMessage{
		CurrentStep: state,
		Succeed:     succeed,
		ClientErr:   clientErr,
		HostErr:     hostErr,
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

func (c *Shard) SetContractID(contractID string) (err error) {
	c.Lock()
	defer c.Unlock()

	if contractID == "" {
		if contractID, err = NewSessionID(); err != nil {
			return
		}
	}

	c.ContractID = contractID
	return
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

func (c *Shard) SetSignedEscrowContract(contract []byte) bool {
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
	fileHash cidlib.Cid, init bool, expir uint64) (*StorageChallenge, error) {
	c.Lock()
	defer c.Unlock()

	if c.Challenge == nil {
		sc, err := NewStorageChallengeResponse(ctx, node, api, fileHash, c.ShardHash, "", init, expir)
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
	return StdStateFlow[c.State].State
}

func (c *Shard) GetState() int {
	return c.State
}

func (c *Shard) GetInitialState(runMode int) int {
	if runMode == OfflineSignMode {
		return UninitializedState
	} else {
		return InitState
	}
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
