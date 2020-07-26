package upload

import (
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"

	cmds "github.com/TRON-US/go-btfs-cmds"

	"github.com/ipfs/go-datastore"
)

var StorageUploadStatusCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Check storage upload and payment status (From client's perspective).",
		ShortDescription: `
This command print upload and payment status by the time queried.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		status := &StatusRes{}
		// check and get session info from sessionMap
		ssId := req.Arguments[0]

		ctxParams, err := helper.ExtractContextParams(req, env)
		if err != nil {
			return err
		}

		// check if checking request from host or client
		if !ctxParams.Cfg.Experimental.StorageClientEnabled && !ctxParams.Cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage client/host api not enabled")
		}

		session, err := sessions.GetRenterSession(ctxParams, ssId, "", make([]string, 0))
		if err != nil {
			return err
		}
		sessionStatus, err := session.Status()
		if err != nil {
			return err
		}
		status.Status = sessionStatus.Status
		status.Message = sessionStatus.Message
		info, err := session.GetAdditionalInfo()
		if err == nil {
			status.AdditionalInfo = info.Info
		} else {
			// NOP
		}

		// get shards info from session
		shards := make(map[string]*ShardStatus)
		status.FileHash = session.Hash
		for i, h := range session.ShardHashes {
			shard, err := sessions.GetRenterShard(ctxParams, ssId, h, i)
			if err != nil {
				return err
			}
			st, err := shard.Status()
			if err != nil {
				return err
			}
			contracts, err := shard.Contracts()
			if err != nil {
				return err
			}
			additionalInfo, err := shard.GetAdditionalInfo()
			if err != nil && err != datastore.ErrNotFound {
				return err
			}
			c := &ShardStatus{
				ContractID:     "",
				Price:          0,
				Host:           "",
				Status:         st.Status,
				Message:        st.Message,
				AdditionalInfo: additionalInfo.Info,
			}
			if contracts.SignedGuardContract != nil {
				c.ContractID = contracts.SignedGuardContract.ContractId
				c.Price = contracts.SignedGuardContract.Price
				c.Host = contracts.SignedGuardContract.HostPid
			}
			shards[sessions.GetShardId(ssId, h, i)] = c
		}
		status.Shards = shards
		return res.Emit(status)
	},
	Type: StatusRes{},
}

type StatusRes struct {
	Status         string
	Message        string
	AdditionalInfo string
	FileHash       string
	Shards         map[string]*ShardStatus
}

type ShardStatus struct {
	ContractID     string
	Price          int64
	Host           string
	Status         string
	Message        string
	AdditionalInfo string
}
