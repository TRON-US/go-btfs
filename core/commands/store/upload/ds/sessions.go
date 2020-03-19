package ds

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/escrow"
	sessionpb "github.com/TRON-US/go-btfs/protos/session"

	config "github.com/TRON-US/go-btfs-config"
	iface "github.com/TRON-US/interface-go-btfs-core"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	"github.com/tron-us/protobuf/proto"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/prometheus/common/log"
)

const (
	sessionKeyPrefix   = "/btfs/%s/renter/sessions/%s/"
	sessionInMemKey    = sessionKeyPrefix
	sessionMetadataKey = sessionKeyPrefix + "metadata"
	session_status_key = sessionKeyPrefix + "status"
)

var (
	sessionsInMem = cmap.New()
	fsmEvents     = fsm.Events{
		{Name: "e-submit", Src: []string{"init"}, Dst: "submit"},
		{Name: "e-pay", Src: []string{"submit"}, Dst: "pay"},
		{Name: "e-guard", Src: []string{"pay"}, Dst: "guard"},
		{Name: "e-wait-upload", Src: []string{"guard"}, Dst: "wait-upload"},
		{Name: "e-complete", Src: []string{"wait-upload"}, Dst: "complete"},
		{Name: "e-error", Src: []string{"init", "submit", "pay", "guard", "wait-upload"}, Dst: "error"},
	}
)

type Session struct {
	Id        string
	PeerId    string
	Datastore datastore.Datastore
	Context   context.Context
	cancel    context.CancelFunc
	Config    *config.Config
	N         *core.IpfsNode
	Api       iface.CoreAPI
	fsm       *fsm.FSM
}

type SessionInitParams struct {
	Context     context.Context
	Config      *config.Config
	N           *core.IpfsNode
	Api         iface.CoreAPI
	Datastore   datastore.Datastore
	RenterId    string
	FileHash    string
	ShardHashes []string
}

func GetSession(sessionId string, peerId string, params *SessionInitParams) (*Session, error) {
	// NewSession
	if sessionId == "" {
		sessionId = uuid.New().String()
	}
	k := fmt.Sprintf(sessionInMemKey, peerId, sessionId)
	var s *Session
	if tmp, ok := sessionsInMem.Get(k); ok {
		s = tmp.(*Session)
		status, err := s.GetStatus()
		if err != nil {
			return nil, err
		}
		s.fsm.SetState(status.Status)
	} else {
		ctx, cancel := storage.NewGoContext(params.Context)
		s = &Session{
			Id:        sessionId,
			PeerId:    peerId,
			Context:   ctx,
			cancel:    cancel,
			N:         params.N,
			Api:       params.Api,
			Config:    params.Config,
			Datastore: params.Datastore,
		}
		s.fsm = fsm.NewFSM("init", fsmEvents, fsm.Callbacks{
			"enter_state": s.enterState,
		})
		err := s.init(params)
		if err != nil {
			return nil, err
		}
		sessionsInMem.Set(k, s)
	}
	return s, nil
}

func (f *Session) init(params *SessionInitParams) error {
	status := &sessionpb.Status{
		Status:  "init",
		Message: "",
	}
	metadata := &sessionpb.Metadata{
		TimeCreate:  time.Now().UTC(),
		RenterId:    params.RenterId,
		FileHash:    params.FileHash,
		ShardHashes: params.ShardHashes,
	}
	ks := []string{fmt.Sprintf(session_status_key, f.PeerId, f.Id),
		fmt.Sprintf(sessionMetadataKey, f.PeerId, f.Id),
	}
	vs := []proto.Message{
		status,
		metadata,
	}
	return Batch(f.Datastore, ks, vs)
}

func (f *Session) GetStatus() (*sessionpb.Status, error) {
	sk := fmt.Sprintf(session_status_key, f.PeerId, f.Id)
	st := &sessionpb.Status{}
	err := Get(f.Datastore, sk, st)
	if err == datastore.ErrNotFound {
		return st, nil
	}
	return st, err
}

func (f *Session) GetMetadata() (*sessionpb.Metadata, error) {
	mk := fmt.Sprintf(sessionMetadataKey, f.PeerId, f.Id)
	md := &sessionpb.Metadata{}
	err := Get(f.Datastore, mk, md)
	if err == datastore.ErrNotFound {
		return md, nil
	}
	return md, err
}

func (f *Session) enterState(e *fsm.Event) {
	log.Info("session:", f.Id, ",enter status:", e.Dst)
	msg := ""
	switch e.Dst {
	case "error":
		msg = e.Args[0].(error).Error()
		log.Error(msg)
		f.onError()
	}
	f.setStatus(e.Dst, msg)
}

func (f *Session) onError() {
	f.cancel()
}

func (f *Session) Submit() {
	f.fsm.Event("e-submit")
}

func (f *Session) Pay() {
	f.fsm.Event("e-pay")
}

func (f *Session) Guard() {
	f.fsm.Event("e-guard")
}

func (f *Session) WaitUpload() {
	f.fsm.Event("e-wait-upload")
}

func (f *Session) Complete() {
	f.fsm.Event("e-complete")
}

func (f *Session) Error(err error) {
	f.fsm.Event("e-error", err)
}

func (f *Session) setStatus(s string, msg string) error {
	status := &sessionpb.Status{
		Status:  s,
		Message: msg,
	}
	return Save(f.Datastore, fmt.Sprintf(session_status_key, f.PeerId, f.Id), status)
}

func (f *Session) GetCompleteShardsNum() (int, int, error) {
	md, err := f.GetMetadata()
	var completeNum, errorNum int
	if err != nil {
		return 0, 0, err
	}
	for _, h := range md.ShardHashes {
		shard, err := GetShard(f.PeerId, f.Id, h, &ShardInitParams{
			Context:   f.Context,
			Datastore: f.Datastore,
		})
		if err != nil {
			continue
		}
		status, err := shard.Status()
		if err != nil {
			continue
		}
		//FIXME
		if status.Status == "complete" || status.Status == "contract" {
			completeNum++
		} else if status.Status == "error" {
			errorNum++
			return completeNum, errorNum, nil
		}
	}
	return completeNum, errorNum, nil
}

func (s *Session) PrepareContractFromShard() ([]*escrowpb.SignedEscrowContract, int64, error) {
	var signedContracts []*escrowpb.SignedEscrowContract
	var totalPrice int64
	md, err := s.GetMetadata()
	if err != nil {
		return nil, 0, err
	}
	for _, hash := range md.ShardHashes {
		shard, err := GetShard(s.PeerId, s.Id, hash, &ShardInitParams{
			Context:   s.Context,
			Datastore: s.Datastore,
		})
		if err != nil {
			return nil, 0, err
		}
		c, err := shard.SignedCongtracts()
		if err != nil {
			return nil, 0, err
		}
		sc, err := escrow.UnmarshalEscrowContract(c.SignedEscrowContract)
		if err != nil {
			return nil, 0, err
		}
		signedContracts = append(signedContracts, sc)
		totalPrice += c.GuardContract.Amount
	}
	return signedContracts, totalPrice, nil
}
