package renter

import (
	"context"
	"errors"
	"fmt"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage/ds"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/guard"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/cenkalti/backoff/v3"
	"github.com/ethereum/go-ethereum/log"
	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/tron-us/go-btfs-common/crypto"
	sessionpb "github.com/tron-us/go-btfs-common/protos/storage/session"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"
	"time"
)

var (
	fsmEvents = fsm.Events{
		{Name: "toInit", Src: []string{""}, Dst: "init"},
		{Name: "toSubmit", Src: []string{"init"}, Dst: "submit"},
		{Name: "toPay", Src: []string{"submit"}, Dst: "pay"},
		{Name: "toGuard", Src: []string{"pay"}, Dst: "guard"},
		{Name: "toWaitUpload", Src: []string{"guard"}, Dst: "wait-upload"},
		{Name: "toComplete", Src: []string{"wait-upload"}, Dst: "complete"},
		{Name: "toError", Src: []string{"init", "submit", "pay", "guard", "wait-upload"}, Dst: "error"},
	}
	sessionsInMem = cmap.New()
)

const (
	sessionInMemKey       = "/btfs/%s/v0.0.1/renter/sessions/%s"
	sessionMetadataKey    = "/btfs/%s/v0.0.1/renter/sessions/%s/metadata"
	session_status_key    = "/btfs/%s/v0.0.1/renter/sessions/%s/status"
	session_contracts_key = "/btfs/%s/v0.0.1/renter/sessions/%s/contracts"
)

var bo = func() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 10 * time.Second
	bo.MaxElapsedTime = 24 * time.Hour
	bo.Multiplier = 1.5
	bo.MaxInterval = 5 * time.Minute
	return bo
}()

type Session struct {
	Id     string
	peerId string
	ctx    context.Context
	cfg    *config.Config
	fsm    *fsm.FSM
	ds     datastore.Datastore
	n      *core.IpfsNode
	api    coreiface.CoreAPI
}

func GetSession(ctx context.Context, ds datastore.Datastore, cfg *config.Config, n *core.IpfsNode,
	api coreiface.CoreAPI, peerId string, sessionId string) (*Session, error) {
	if sessionId == "" {
		sessionId = uuid.New().String()
	}
	k := fmt.Sprintf(sessionInMemKey, peerId, peerId, sessionId)
	var s *Session
	if tmp, ok := sessionsInMem.Get(k); ok {
		fmt.Println("get session from cache")
		s = tmp.(*Session)
	} else {
		fmt.Println("new session")
		ctx, _ := context.WithTimeout(context.Background(), 30*time.Minute)
		s = &Session{
			Id:     sessionId,
			ctx:    ctx,
			cfg:    cfg,
			ds:     ds,
			peerId: peerId,
			n:      n,
			api:    api,
		}
		s.fsm = fsm.NewFSM("", fsmEvents, fsm.Callbacks{
			"enter_state": s.enterState,
		})
	}
	status, err := s.GetStatus()
	if err != nil {
		return nil, err
	}
	s.fsm.SetState(status.Status)
	return s, nil
}

func (f *Session) enterState(e *fsm.Event) {
	fmt.Println("session enter state:", e.Dst)
	var err error
	switch e.Dst {
	case "init":
		err = f.onInit(e.Args[0].(string), e.Args[1].(string), e.Args[2].([]string))
	case "submit":
		err = f.onSubmit()
	case "pay":
		fmt.Println("e.Args[0]", e.Args[0])
		err = f.onPay(e.Args[0].(*escrowpb.SignedSubmitContractResult))
	case "guard":
		err = f.onGuard(e.Args[0].(*escrowpb.SignedPayinResult), e.Args[1].(ic.PrivKey))
	case "wait-upload":
		err = f.onWaitUpload(e.Args[0].(ic.PrivKey))
	case "complete":
		err = f.onComplete()
	case "error":
		f.onError(e.Args[0].(error))
	}
	if err != nil {
		f.ToError(err)
	}
}

func (f *Session) ToInit(renterId string, fileHash string, hashes []string) error {
	return f.fsm.Event("toInit", renterId, fileHash, hashes)
}

func (f *Session) ToSubmit() error {
	return f.fsm.Event("toSubmit")
}

func (f *Session) ToPay(response *escrowpb.SignedSubmitContractResult) error {
	return f.fsm.Event("toPay", response)
}

func (f *Session) ToGuard(res *escrowpb.SignedPayinResult, payerPriKey ic.PrivKey) error {
	return f.fsm.Event("toGuard", res, payerPriKey)
}

func (f *Session) ToWaitUpload(payerPriKey ic.PrivKey) error {
	return f.fsm.Event("toWaitUpload", payerPriKey)
}

func (f *Session) ToComplete() error {
	return f.fsm.Event("toComplete")
}

func (f *Session) ToError(err error) error {
	return f.fsm.Event("toError", err)
}

func (f *Session) Timeout() error {
	return f.fsm.Event("toError", errors.New("timeout"))
}

func (f *Session) onInit(renterId string, fileHash string, hashes []string) error {
	status := &sessionpb.Status{
		Status:  "init",
		Message: "",
	}
	metadata := &sessionpb.Metadata{
		TimeCreate:  time.Now().UTC(),
		RenterId:    renterId,
		FileHash:    fileHash,
		ShardHashes: hashes,
	}
	ks := []string{fmt.Sprintf(session_status_key, f.peerId, f.Id),
		fmt.Sprintf(sessionMetadataKey, f.peerId, f.Id),
	}
	vs := []proto.Message{
		status,
		metadata,
	}
	err := ds.Batch(f.ds, ks, vs)
	if err != nil {
		return err
	}
	go func(ctx context.Context, numShards int) {
		tick := time.Tick(5 * time.Second)
		for true {
			select {
			case <-tick:
				completeNum, errorNum, err := f.GetCompleteShardsNum()
				if err != nil {
					continue
				}
				if completeNum == numShards {
					err := f.ToSubmit()
					if err == nil {
						return
					}
				} else if errorNum > 0 {
					err := f.ToError(errors.New("there are error shards"))
					if err == nil {
						return
					}
				}
				//case <-ctx.Done():
				//	fmt.Println("<-ctx.Done()")
				//	return
			}
		}
	}(f.ctx, len(hashes))
	return err
}

func (f *Session) onSubmit() error {
	status := &sessionpb.Status{
		Status:  "submit",
		Message: "",
	}
	err := ds.Save(f.ds, fmt.Sprintf(session_status_key, f.peerId, f.Id), status)
	if err != nil {
		return err
	}
	fmt.Println(2)
	bs, t, err := f.PrepareContractFromShard()
	if err != nil {
		return err
	}
	fmt.Println(3)
	// check account balance, if not enough for the totalPrice do not submit to escrow
	balance, err := escrow.Balance(f.ctx, f.cfg)
	if err != nil {
		fmt.Println("err", err)
		return err
	}
	fmt.Println(4)
	if balance < t {
		fmt.Println("balance", balance, "t", t)
		err = fmt.Errorf("not enough balance to submit contract, current balance is [%v]", balance)
		return err
	}
	fmt.Println(5)
	req, err := escrow.NewContractRequest(f.cfg, bs, t)
	if err != nil {
		return err
	}
	fmt.Println(6)
	var amount int64 = 0
	for _, c := range req.Contract {
		amount += c.Contract.Amount
	}
	fmt.Println("amount:", amount, "channel amount:", req.BuyerChannel.Channel.Amount)
	fmt.Println("req.buyerChannel", req.BuyerChannel)
	submitContractRes, err := escrow.SubmitContractToEscrow(f.ctx, f.cfg, req)
	if err != nil {
		fmt.Println("escrow submit err", err)
		return fmt.Errorf("failed to submit contracts to escrow: [%v]", err)
	}
	fmt.Println(7)
	go func() {
		f.ToPay(submitContractRes)
	}()
	return nil
}

func (f *Session) onPay(response *escrowpb.SignedSubmitContractResult) error {
	fmt.Println("on pay...")
	status := &sessionpb.Status{
		Status:  "pay",
		Message: "",
	}
	err := ds.Save(f.ds, fmt.Sprintf(session_status_key, f.peerId, f.Id), status)
	if err != nil {
		return err
	}
	privKeyStr := f.cfg.Identity.PrivKey
	payerPrivKey, err := crypto.ToPrivKey(privKeyStr)
	if err != nil {
		return err
	}
	payerPubKey := payerPrivKey.GetPublic()
	payinRequest, err := escrow.NewPayinRequest(response, payerPubKey, payerPrivKey)
	if err != nil {
		return err
	}
	payinRes, err := escrow.PayInToEscrow(f.ctx, f.cfg, payinRequest)
	if err != nil {
		return fmt.Errorf("failed to pay in to escrow: [%v]", err)
	}
	go func() {
		f.ToGuard(payinRes, payerPrivKey)
	}()
	return nil
}

func (f *Session) onGuard(payinRes *escrowpb.SignedPayinResult, payerPriKey ic.PrivKey) error {
	fmt.Println("on guard...")
	status := &sessionpb.Status{
		Status:  "guard",
		Message: "",
	}
	err := ds.Save(f.ds, fmt.Sprintf(session_status_key, f.peerId, f.Id), status)
	fmt.Println("on guard err", err)
	if err != nil {
		return err
	}
	md, err := f.GetMetadata()
	fmt.Println("on guard get md err", err)
	if err != nil {
		return err
	}
	fmt.Println("md", md)
	fsStatus, err := guard.PrepAndUploadFileMeta2(f.ctx, ss, payinRes, payerPriKey, f.cfg)
	if err != nil {
		return fmt.Errorf("failed to send file meta to guard: [%v]", err)
	}

	qs, err := guard.PrepFileChallengeQuestions(ctx, n, api, ss, fsStatus)
	if err != nil {
		return err
	}

	err = guard.SendChallengeQuestions(ctx, cfg, ss.FileHash, qs)
	if err != nil {
		return fmt.Errorf("failed to send challenge questions to guard: [%v]", err)
	}
	go func() {
		f.ToWaitUpload(payerPriKey)
	}()
	return nil
}

func (f *Session) onWaitUpload(payerPriKey ic.PrivKey) error {
	status := &sessionpb.Status{
		Status:  "wait-upload",
		Message: "",
	}
	err := ds.Save(f.ds, fmt.Sprintf(session_status_key, f.peerId, f.Id), status)
	if err != nil {
		return err
	}
	md, err := f.GetMetadata()
	if err != nil {
		return err
	}
	go func() {
		err := backoff.Retry(func() error {
			err := grpc.GuardClient(f.cfg.Services.GuardDomain).WithContext(f.ctx,
				func(ctx context.Context, client guardpb.GuardServiceClient) error {
					req := &guardpb.CheckFileStoreMetaRequest{
						FileHash:     md.FileHash,
						RenterPid:    md.RenterId,
						RequesterPid: f.n.Identity.Pretty(),
						RequestTime:  time.Now().UTC(),
					}
					sign, err := crypto.Sign(payerPriKey, req)
					if err != nil {
						return err
					}
					req.Signature = sign
					meta, err := client.CheckFileStoreMeta(ctx, req)
					if err != nil {
						return err
					}
					if meta.State == guardpb.FileStoreStatus_RUNNING {
						return nil
					}
					return errors.New("uploading")
				})
			return err
		}, bo)
		if err != nil {
			log.Error("Check file upload failed", err)
		}
	}()
	return nil
}

func (f *Session) onComplete() error {
	status := &sessionpb.Status{
		Status:  "complete",
		Message: "",
	}
	err := ds.Save(f.ds, fmt.Sprintf(session_status_key, f.peerId, f.Id), status)
	if err != nil {
		return err
	}
	return nil
}

func (f *Session) onError(err error) error {
	status := &sessionpb.Status{
		Status:  "error",
		Message: err.Error(),
	}
	return ds.Save(f.ds, fmt.Sprintf(session_status_key, f.peerId, f.Id), status)
}

func (f *Session) GetMetadata() (*sessionpb.Metadata, error) {
	mk := fmt.Sprintf(sessionMetadataKey, f.peerId, f.Id)
	md := &sessionpb.Metadata{}
	err := ds.Get(f.ds, mk, md)
	if err == datastore.ErrNotFound {
		return md, nil
	}
	return md, err
}

func (f *Session) GetStatus() (*sessionpb.Status, error) {
	sk := fmt.Sprintf(session_status_key, f.peerId, f.Id)
	st := &sessionpb.Status{}
	err := ds.Get(f.ds, sk, st)
	if err == datastore.ErrNotFound {
		return st, nil
	}
	return st, err
}

func (f *Session) GetCompleteShardsNum() (int, int, error) {
	md, err := f.GetMetadata()
	var completeNum, errorNum int
	if err != nil {
		return 0, 0, err
	}
	for _, h := range md.ShardHashes {
		shard, err := GetShard(f.ctx, f.ds, f.peerId, f.Id, h)
		if err != nil {
			continue
		}
		status, err := shard.Status()
		if err != nil {
			continue
		}
		if status.Status == "complete" {
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
		shard, err := GetShard(s.ctx, s.ds, s.peerId, s.Id, hash)
		if err != nil {
			return nil, 0, err
		}
		scs, err := shard.SignedCongtracts()
		if err != nil {
			return nil, 0, err
		}
		smd, err := shard.Metadata()
		if err != nil {
			return nil, 0, err
		}
		sc, err := escrow.UnmarshalEscrowContract(scs.SignedEscrowContract)
		if err != nil {
			return nil, 0, err
		}
		signedContracts = append(signedContracts, sc)
		totalPrice += smd.TotalPay
	}
	return signedContracts, totalPrice, nil
}