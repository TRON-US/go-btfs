package commands

import (
	"fmt"
	"strings"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/guard"
	cconfig "github.com/tron-us/go-btfs-common/config"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	"github.com/ipfs/go-cid"
)

const (
	guardUrlOptionName                   = "url"
	guardQuestionCountPerShardOptionName = "questions-per-shard"
	guardHostsOptionName                 = "hosts"
)

var GuardCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with guard services from BTFS client.",
		ShortDescription: `
Connect with guard functions directly through this command.
The subcommands here are mostly for debugging and testing purposes.`,
	},
	Subcommands: map[string]*cmds.Command{
		"test": guardTestCmd,
	},
}

var guardTestCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Send tests to guard service endpoints from BTFS client.",
		ShortDescription: `
This command contains subcommands that are typically for development purposes
by letting the BTFS client test individual guard endpoints.`,
	},
	Subcommands: map[string]*cmds.Command{
		"send-challenges": guardSendChallengesCmd,
	},
}

type questionRes struct {
	qs  *guardpb.ShardChallengeQuestions
	err error
	i   int
}

var guardSendChallengesCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Send shard challenge questions from BTFS client.",
		ShortDescription: `
Sends all shard challenge questions under a reed-solomon encoded file
to the guard service.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "File hash to generate the questions from.").EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.StringOption(guardUrlOptionName, "u", "Guard service url including protocol and port. Default: reads from BTFS config."),
		cmds.Int64Option(guardQuestionCountPerShardOptionName, "q", "Number of challenge questions per shard to generate").WithDefault(cconfig.ConstMinQuestionsCountPerChallenge),
		cmds.StringOption(guardHostsOptionName, "sh", "List of hosts for each shard, ordered sequentially. Default: reads from BTFS datastore."),
	},
	RunTimeout: 30 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// get config settings
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
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
		rootHash, err := cid.Parse(req.Arguments[0])
		if err != nil {
			return err
		}
		shardHashes, err := storage.CheckAndGetReedSolomonShardHashes(req.Context, n, api, rootHash)
		if err != nil {
			return err
		}
		// check if we have enough hosts
		var hostIDs []string
		if hl, found := req.Options[guardHostsOptionName].(string); found {
			hostIDs = strings.Split(hl, ",")
		} else {
			hosts, err := storage.GetHostsFromDatastore(req.Context, n, storage.HostModeAll, len(shardHashes))
			if err != nil {
				return err
			}
			for _, ni := range hosts {
				hostIDs = append(hostIDs, ni.NodeID)
			}
		}
		if len(hostIDs) < len(shardHashes) {
			return fmt.Errorf("hosts list must be at least %d", len(shardHashes))
		}

		qCount, _ := req.Options[guardQuestionCountPerShardOptionName].(int64)
		// generate each shard's questions individually, then combine
		questions := make([]*guardpb.ShardChallengeQuestions, len(shardHashes))
		qc := make(chan questionRes)
		for i, sh := range shardHashes {
			go func(shardIndex int, shardHash cid.Cid) {
				qs, err := guard.PrepShardChallengeQuestions(req.Context, n, api,
					rootHash, shardHash, "", int(qCount))
				qc <- questionRes{
					qs:  qs,
					err: err,
					i:   shardIndex,
				}
			}(i, sh)
		}
		for i := 0; i < len(questions); i++ {
			res := <-qc
			if res.err != nil {
				return res.err
			}
			questions[res.i] = res.qs
		}
		// check if we need to update config for a different guard url
		if gu, found := req.Options[guardUrlOptionName].(string); found {
			cfg, err = cfg.Clone()
			if err != nil {
				return err
			}
			cfg.Services.GuardDomain = gu
		}
		// send to guard
		return guard.SendChallengeQuestions(req.Context, cfg, rootHash, questions)
	},
}
