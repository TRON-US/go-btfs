package upload

import (
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/store/upload/ds"

	cmds "github.com/TRON-US/go-btfs-cmds"
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
		fmt.Println("status....")
		status := &StatusRes{}
		// check and get session info from sessionMap
		ssID := req.Arguments[0]
		// get hosts
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}

		// get node
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		// get core api
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}

		ss, err := ds.GetSession(ssID, n.Identity.Pretty(), &ds.SessionInitParams{
			Context:   req.Context,
			Config:    cfg,
			Datastore: n.Repo.Datastore(),
			N:         n,
			Api:       api,
			RenterId:  n.Identity.Pretty(),
		})
		if err != nil {
			return err
		}
		sessionStatus, err := ss.GetStatus()
		if err != nil {
			return err
		}
		status.Status = sessionStatus.Status
		status.Message = sessionStatus.Message

		// check if checking request from host or client
		if !cfg.Experimental.StorageClientEnabled && !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage client/host api not enabled")
		}

		// get shards info from session
		shards := make(map[string]*ShardStatus)
		metadata, err := ss.GetMetadata()
		if err != nil {
			return err
		}
		status.FileHash = metadata.FileHash
		for _, h := range metadata.ShardHashes {
			shard, err := ds.GetShard(ss.PeerId, ss.Id, h, &ds.ShardInitParams{
				Context:   ss.Context,
				Datastore: ss.Datastore,
			})
			if err != nil {
				return err
			}
			st, err := shard.Status()
			if err != nil {
				return err
			}
			contracts, err := shard.SignedCongtracts()
			if err != nil {
				return err
			}
			c := &ShardStatus{
				ContractID: "",
				Price:      0,
				Host:       "",
				Status:     st.Status,
				Message:    st.Message,
			}
			if contracts.GuardContract != nil {
				c.ContractID = contracts.GuardContract.ContractId
				c.Price = contracts.GuardContract.Price
				c.Host = contracts.GuardContract.HostPid
			}
			shards[h] = c
		}
		status.Shards = shards
		return res.Emit(status)
	},
	Type: StatusRes{},
}

type StatusRes struct {
	Status   string
	Message  string
	FileHash string
	Shards   map[string]*ShardStatus
}

type ShardStatus struct {
	ContractID string
	Price      int64
	Host       string
	Status     string
	Message    string
}
