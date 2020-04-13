package upload

import (
	"fmt"

	cmds "github.com/TRON-US/go-btfs-cmds"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	"github.com/gogo/protobuf/proto"
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

		ctxParams, err := extractContextParams(req, env)
		if err != nil {
			return err
		}

		// check if checking request from host or client
		if !ctxParams.cfg.Experimental.StorageClientEnabled && !ctxParams.cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage client/host api not enabled")
		}

		session, err := GetRenterSession(ctxParams, ssId, "", make([]string, 0))
		if err != nil {
			return err
		}
		sessionStatus, err := session.status()
		if err != nil {
			return err
		}
		status.Status = sessionStatus.Status
		status.Message = sessionStatus.Message

		// get shards info from session
		shards := make(map[string]*ShardStatus)
		status.FileHash = sessionStatus.Hash
		for _, h := range session.shardHashes {
			shard, err := GetRenterShard(ctxParams, ssId, h)
			if err != nil {
				return err
			}
			st, err := shard.status()
			if err != nil {
				return err
			}
			contracts, err := shard.contracts()
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
			if contracts.SignedGuardContract != nil {
				contract := new(guardpb.Contract)
				err := proto.Unmarshal(contracts.SignedGuardContract, contract)
				if err != nil {
					return err
				}
				c.ContractID = contract.ContractId
				c.Price = contract.Price
				c.Host = contract.HostPid
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
