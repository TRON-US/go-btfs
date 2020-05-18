package sessions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	RssInitStatus                            = "init"
	RssSubmitStatus                          = "submit"
	RssSubmitBalanceReqSignedStatus          = "submit:check-balance-req-singed"
	RssSubmitLedgerChannelCommitSignedStatus = "submit:ledger-channel-commit-signed"
	RssPayStatus                             = "pay"
	RssPayPayinRequestSignedStatus           = "pay:payin-req-signed"
	RssGuardStatus                           = "guard"
	RssGuardFileMetaSignedStatus             = "guard:file-meta-signed"
	RssGuardQuestionsSignedStatus            = "guard:questions-signed"
	RssWaitUploadStatus                      = "wait-upload"
	RssWaitUploadReqSignedStatus             = "wait-upload:req-signed"
	RssCompleteStatus                        = "complete"
	RssErrorStatus                           = "error"

	RssToSubmitEvent                          = "to-submit-event"
	RssToSubmitBalanceReqSignedEvent          = "to-submit:balance-req-signed-event"
	RssToSubmitLedgerChannelCommitSignedEvent = "to-submit:ledger-channel-commit-signed-event"
	RssToPayEvent                             = "to-pay-event"
	RssToPayPayinRequestSignedEvent           = "to-pay:payin-req-signed-event"
	RssToGuardEvent                           = "to-guard-event"
	RssToGuardFileMetaSignedEvent             = "to-guard:file-meta-signed-event"
	RssToGuardQuestionsSignedEvent            = "to-guard:questions-signed-event"
	RssToWaitUploadEvent                      = "to-wait-upload-event"
	RssToWaitUploadReqSignedEvent             = "to-wait-upload-signed-event"
	RssToCompleteEvent                        = "to-complete-event"
	RssToErrorEvent                           = "to-error-event"

	RenterSessionKey               = "/btfs/%s/renter/sessions/%s/"
	RenterSessionInMemKey          = RenterSessionKey
	RenterSessionStatusKey         = RenterSessionKey + "status"
	RenterSessionOfflineMetaKey    = RenterSessionKey + "offline-meta"
	RenterSessionOfflineSigningKey = RenterSessionKey + "offline-signing"
)

var (
	renterSessionsInMem = cmap.New()
	rssFsmEvents        = fsm.Events{
		{Name: RssToSubmitEvent, Src: []string{RssInitStatus}, Dst: RssSubmitStatus},
		{Name: RssToSubmitBalanceReqSignedEvent, Src: []string{RssSubmitStatus}, Dst: RssSubmitBalanceReqSignedStatus},
		{Name: RssToSubmitLedgerChannelCommitSignedEvent, Src: []string{RssSubmitBalanceReqSignedStatus}, Dst: RssSubmitLedgerChannelCommitSignedStatus},
		{Name: RssToPayEvent, Src: []string{RssSubmitLedgerChannelCommitSignedStatus}, Dst: RssPayStatus},
		{Name: RssToPayPayinRequestSignedEvent, Src: []string{RssPayStatus}, Dst: RssPayPayinRequestSignedStatus},
		{Name: RssToGuardEvent, Src: []string{RssPayPayinRequestSignedStatus}, Dst: RssGuardStatus},
		{Name: RssToGuardFileMetaSignedEvent, Src: []string{RssGuardStatus}, Dst: RssGuardFileMetaSignedStatus},
		{Name: RssToGuardQuestionsSignedEvent, Src: []string{RssGuardFileMetaSignedStatus}, Dst: RssGuardQuestionsSignedStatus},
		{Name: RssToWaitUploadEvent, Src: []string{RssGuardQuestionsSignedStatus}, Dst: RssWaitUploadStatus},
		{Name: RssToWaitUploadReqSignedEvent, Src: []string{RssWaitUploadStatus}, Dst: RssWaitUploadReqSignedStatus},
		{Name: RssToCompleteEvent, Src: []string{RssWaitUploadReqSignedStatus}, Dst: RssCompleteStatus},
	}
)

func init() {
	src := make([]string, 0)
	for _, s := range rssFsmEvents {
		src = append(src, s.Src...)
	}
	rssFsmEvents = append(rssFsmEvents, fsm.EventDesc{
		Name: RssToErrorEvent, Src: src, Dst: RssErrorStatus,
	})
}

type RenterSession struct {
	PeerId      string
	SsId        string
	Hash        string
	ShardHashes []string
	fsm         *fsm.FSM
	CtxParams   *uh.ContextParams
	Ctx         context.Context
	Cancel      context.CancelFunc
}

func GetRenterSession(ctxParams *uh.ContextParams, ssId string, hash string, shardHashes []string) (*RenterSession,
	error) {
	k := fmt.Sprintf(RenterSessionInMemKey, ctxParams.N.Identity.Pretty(), ssId)
	var rs *RenterSession
	if tmp, ok := renterSessionsInMem.Get(k); ok {
		log.Debugf("get renter_session:%s from cache.", k)
		rs = tmp.(*RenterSession)
	} else {
		log.Debugf("new renter_session:%s.", k)
		ctx, cancel := helper.NewGoContext(ctxParams.Ctx)
		rs = &RenterSession{
			PeerId:      ctxParams.N.Identity.Pretty(),
			SsId:        ssId,
			Hash:        hash,
			ShardHashes: shardHashes,
			Ctx:         ctx,
			Cancel:      cancel,
			CtxParams:   ctxParams,
		}
		status, err := rs.Status()
		if err != nil {
			return nil, err
		}
		if status.Status != RssCompleteStatus {
			rs.fsm = fsm.NewFSM(status.Status, rssFsmEvents, fsm.Callbacks{
				"enter_state": rs.enterState,
			})
		}
		renterSessionsInMem.Set(k, rs)
	}
	return rs, nil
}

var helperText = map[string]string{
	RssInitStatus:       "Searching for recommended hosts…",
	RssSubmitStatus:     "Hosts found! Checking wallet balance and submitting contracts to escrow.",
	RssPayStatus:        "Contracts submitted! Confirming the escrow payment.",
	RssGuardStatus:      "Payment successful! Preparing meta-data and challenge questions.",
	RssWaitUploadStatus: "Confirming successful file shard storage by hosts.",
	RssCompleteStatus:   "File storage successful!",
}

func (rs *RenterSession) enterState(e *fsm.Event) {
	var msg string
	if text, ok := helperText[strings.Split(e.Dst, ":")[0]]; ok {
		msg = text
	} else {
		msg = ""
	}
	switch e.Dst {
	case RssErrorStatus:
		msg = e.Args[0].(error).Error()
		rs.Cancel()
	case RssCompleteStatus:
		rs.Cancel()
	}
	fmt.Printf("[%s] session: %s entered state: %s, msg: %s\n", time.Now().Format(time.RFC3339), rs.SsId, e.Dst, msg)
	err := Save(rs.CtxParams.N.Repo.Datastore(), fmt.Sprintf(RenterSessionStatusKey, rs.PeerId, rs.SsId),
		&renterpb.RenterSessionStatus{
			Status:      e.Dst,
			Message:     msg,
			Hash:        rs.Hash,
			ShardHashes: rs.ShardHashes,
			LastUpdated: time.Now().UTC(),
		})
	go func() {
		_ = rs.To(RssErrorStatus, err)
	}()
}

func (rs *RenterSession) Status() (*renterpb.RenterSessionStatus, error) {
	status := &renterpb.RenterSessionStatus{}
	err := Get(rs.CtxParams.N.Repo.Datastore(), fmt.Sprintf(RenterSessionStatusKey, rs.PeerId, rs.SsId), status)
	if err == datastore.ErrNotFound {
		return &renterpb.RenterSessionStatus{
			Status:      RssInitStatus,
			Message:     helperText[RssInitStatus],
			ShardHashes: rs.ShardHashes,
		}, nil
	}
	return status, err
}

func (rs *RenterSession) GetCompleteShardsNum() (int, int, error) {
	var completeNum, errorNum int
	status, err := rs.Status()
	if err != nil {
		return 0, 0, err
	}
	for i, h := range status.ShardHashes {
		shard, err := GetRenterShard(rs.CtxParams, rs.SsId, h, i)
		if err != nil {
			log.Errorf("get renter shard error:", err.Error())
			continue
		}
		s, err := shard.Status()
		if err != nil {
			return 0, 0, err
		}
		if s.Status == rshContractStatus {
			completeNum++
		} else if status.Status == rshErrorStatus {
			errorNum++
			return completeNum, errorNum, nil
		}
	}
	return completeNum, errorNum, nil
}

func (rs *RenterSession) To(event string, args ...interface{}) error {
	return rs.fsm.Event(event, args...)
}

func (rs *RenterSession) SaveOfflineMeta(meta *renterpb.OfflineMeta) error {
	return Save(rs.CtxParams.N.Repo.Datastore(), fmt.Sprintf(RenterSessionOfflineMetaKey, rs.PeerId, rs.SsId), meta)
}

func (rs *RenterSession) OfflineMeta() (*renterpb.OfflineMeta, error) {
	meta := new(renterpb.OfflineMeta)
	err := Get(rs.CtxParams.N.Repo.Datastore(), fmt.Sprintf(RenterSessionOfflineMetaKey, rs.PeerId, rs.SsId), meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func (rs *RenterSession) SaveOfflineSigning(signingData *renterpb.OfflineSigning) error {
	return Save(rs.CtxParams.N.Repo.Datastore(), fmt.Sprintf(RenterSessionOfflineSigningKey, rs.PeerId, rs.SsId), signingData)
}

func (rs *RenterSession) OfflineSigning() (*renterpb.OfflineSigning, error) {
	signingData := new(renterpb.OfflineSigning)
	err := Get(rs.CtxParams.N.Repo.Datastore(), fmt.Sprintf(RenterSessionOfflineSigningKey, rs.PeerId, rs.SsId),
		signingData)
	if err != nil {
		return nil, err
	}
	return signingData, nil
}
