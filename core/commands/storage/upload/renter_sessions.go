package upload

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"

	"github.com/ipfs/go-datastore"
	"github.com/looplab/fsm"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	rssInitStatus                            = "init"
	rssSubmitStatus                          = "submit"
	rssSubmitBalanceReqSignedStatus          = "submit:check-balance-req-singed"
	rssSubmitLedgerChannelCommitSignedStatus = "submit:ledger-channel-commit-signed"
	rssPayStatus                             = "pay"
	rssPayPayinRequestSignedStatus           = "pay:payin-req-signed"
	rssGuardStatus                           = "guard"
	rssGuardFileMetaSignedStatus             = "guard:file-meta-signed"
	rssWaitUploadStatus                      = "wait-upload"
	rssWaitUploadReqSignedStatus             = "wait-upload:req-signed"
	rssCompleteStatus                        = "complete"
	rssErrorStatus                           = "error"

	rssToSubmitEvent                          = "to-submit-event"
	rssToSubmitBalanceReqSignedEvent          = "to-submit:balance-req-signed-event"
	rssToSubmitLedgerChannelCommitSignedEvent = "to-submit:ledger-channel-commit-signed-event"
	rssToPayEvent                             = "to-pay-event"
	rssToPayPayinRequestSignedEvent           = "to-pay:payin-req-signed-event"
	rssToGuardEvent                           = "to-guard-event"
	rssToGuardFileMetaSignedEvent             = "to-guard:file-meta-signed-event"
	rssToWaitUploadEvent                      = "to-wait-upload-event"
	rssToWaitUploadReqSignedEvent             = "to-wait-upload-signed-event"
	rssToCompleteEvent                        = "to-complete-event"
	rssToErrorEvent                           = "to-error-event"

	renterSessionKey               = "/btfs/%s/renter/sessions/%s/"
	renterSessionInMemKey          = renterSessionKey
	renterSessionStatusKey         = renterSessionKey + "status"
	renterSessionOfflineMetaKey    = renterSessionKey + "offline-meta"
	renterSessionOfflineSigningKey = renterSessionKey + "offline-signing"
)

var (
	renterSessionsInMem = cmap.New()
	rssFsmEvents        = fsm.Events{
		{Name: rssToSubmitEvent, Src: []string{rssInitStatus}, Dst: rssSubmitStatus},
		{Name: rssToSubmitBalanceReqSignedEvent, Src: []string{rssSubmitStatus}, Dst: rssSubmitBalanceReqSignedStatus},
		{Name: rssToSubmitLedgerChannelCommitSignedEvent, Src: []string{rssSubmitBalanceReqSignedStatus}, Dst: rssSubmitLedgerChannelCommitSignedStatus},
		{Name: rssToPayEvent, Src: []string{rssSubmitLedgerChannelCommitSignedStatus}, Dst: rssPayStatus},
		{Name: rssToPayPayinRequestSignedEvent, Src: []string{rssPayStatus}, Dst: rssPayPayinRequestSignedStatus},
		{Name: rssToGuardEvent, Src: []string{rssPayPayinRequestSignedStatus}, Dst: rssGuardStatus},
		{Name: rssToGuardFileMetaSignedEvent, Src: []string{rssGuardStatus}, Dst: rssGuardFileMetaSignedStatus},
		//{Name: rssToGuardQuestionsSignedEvent, Src: []string{rssGuardFileMetaSignedStatus}, Dst: rssGuardQuestionsSignedStatus},
		{Name: rssToWaitUploadEvent, Src: []string{rssGuardFileMetaSignedStatus}, Dst: rssWaitUploadStatus},
		{Name: rssToWaitUploadReqSignedEvent, Src: []string{rssWaitUploadStatus}, Dst: rssWaitUploadReqSignedStatus},
		{Name: rssToCompleteEvent, Src: []string{rssWaitUploadReqSignedStatus}, Dst: rssCompleteStatus},
	}
)

func init() {
	src := make([]string, 0)
	for _, s := range rssFsmEvents {
		src = append(src, s.Src...)
	}
	rssFsmEvents = append(rssFsmEvents, fsm.EventDesc{
		Name: rssToErrorEvent, Src: src, Dst: rssErrorStatus,
	})
}

type RenterSession struct {
	peerId      string
	ssId        string
	hash        string
	shardHashes []string
	fsm         *fsm.FSM
	ctxParams   *ContextParams
	ctx         context.Context
	cancel      context.CancelFunc
}

func GetRenterSession(ctxParams *ContextParams, ssId string, hash string, shardHashes []string) (*RenterSession,
	error) {
	k := fmt.Sprintf(renterSessionInMemKey, ctxParams.n.Identity.Pretty(), ssId)
	var rs *RenterSession
	if tmp, ok := renterSessionsInMem.Get(k); ok {
		log.Debugf("get renter_session:%s from cache.", k)
		rs = tmp.(*RenterSession)
	} else {
		log.Debugf("new renter_session:%s.", k)
		ctx, cancel := helper.NewGoContext(ctxParams.ctx)
		rs = &RenterSession{
			peerId:      ctxParams.n.Identity.Pretty(),
			ssId:        ssId,
			hash:        hash,
			shardHashes: shardHashes,
			ctx:         ctx,
			cancel:      cancel,
			ctxParams:   ctxParams,
		}
		status, err := rs.status()
		if err != nil {
			return nil, err
		}
		if status.Status != rssCompleteStatus {
			rs.fsm = fsm.NewFSM(status.Status, rssFsmEvents, fsm.Callbacks{
				"enter_state": rs.enterState,
			})
		}
		renterSessionsInMem.Set(k, rs)
	}
	return rs, nil
}

var helperText = map[string]string{
	rssInitStatus:       "Searching for recommended hosts…",
	rssSubmitStatus:     "Hosts found! Checking wallet balance and submitting contracts to escrow.",
	rssPayStatus:        "Contracts submitted! Confirming the escrow payment.",
	rssGuardStatus:      "Payment successful! Preparing meta-data and challenge questions.",
	rssWaitUploadStatus: "Confirming successful file shard storage by hosts.",
	rssCompleteStatus:   "File storage successful!",
}

func (rs *RenterSession) enterState(e *fsm.Event) {
	var msg string
	if text, ok := helperText[strings.Split(e.Dst, ":")[0]]; ok {
		msg = text
	} else {
		msg = ""
	}
	fmt.Printf("session: %s enter status: %s\n", rs.ssId, e.Dst)
	switch e.Dst {
	case rssErrorStatus:
		msg = e.Args[0].(error).Error()
		rs.cancel()
	case rssCompleteStatus:
		rs.cancel()
	}
	err := Save(rs.ctxParams.n.Repo.Datastore(), fmt.Sprintf(renterSessionStatusKey, rs.peerId, rs.ssId),
		&renterpb.RenterSessionStatus{
			Status:      e.Dst,
			Message:     msg,
			Hash:        rs.hash,
			ShardHashes: rs.shardHashes,
			LastUpdated: time.Now().UTC(),
		})
	go func() {
		_ = rs.to(rssErrorStatus, err)
	}()
}

func (rs *RenterSession) status() (*renterpb.RenterSessionStatus, error) {
	status := &renterpb.RenterSessionStatus{}
	err := Get(rs.ctxParams.n.Repo.Datastore(), fmt.Sprintf(renterSessionStatusKey, rs.peerId, rs.ssId), status)
	if err == datastore.ErrNotFound {
		return &renterpb.RenterSessionStatus{
			Status:      rssInitStatus,
			Message:     helperText[rssInitStatus],
			ShardHashes: rs.shardHashes,
		}, nil
	}
	return status, err
}

func (rs *RenterSession) GetCompleteShardsNum() (int, int, error) {
	var completeNum, errorNum int
	status, err := rs.status()
	if err != nil {
		return 0, 0, err
	}
	for i, h := range status.ShardHashes {
		shard, err := GetRenterShard(rs.ctxParams, rs.ssId, h, i)
		if err != nil {
			log.Errorf("get renter shard error:", err.Error())
			continue
		}
		s, err := shard.status()
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

func (rs *RenterSession) to(status string, args ...interface{}) error {
	return rs.fsm.Event(status, args...)
}

func (rs *RenterSession) saveOfflineMeta(meta *renterpb.OfflineMeta) error {
	return Save(rs.ctxParams.n.Repo.Datastore(), fmt.Sprintf(renterSessionOfflineMetaKey, rs.peerId, rs.ssId), meta)
}

func (rs *RenterSession) offlineMeta() (*renterpb.OfflineMeta, error) {
	meta := new(renterpb.OfflineMeta)
	err := Get(rs.ctxParams.n.Repo.Datastore(), fmt.Sprintf(renterSessionOfflineMetaKey, rs.peerId, rs.ssId), meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func (rs *RenterSession) saveOfflineSigning(signingData *renterpb.OfflineSigning) error {
	return Save(rs.ctxParams.n.Repo.Datastore(), fmt.Sprintf(renterSessionOfflineSigningKey, rs.peerId, rs.ssId), signingData)
}

func (rs *RenterSession) offlineSigning() (*renterpb.OfflineSigning, error) {
	signingData := new(renterpb.OfflineSigning)
	err := Get(rs.ctxParams.n.Repo.Datastore(), fmt.Sprintf(renterSessionOfflineSigningKey, rs.peerId, rs.ssId),
		signingData)
	if err != nil {
		return nil, err
	}
	return signingData, nil
}
