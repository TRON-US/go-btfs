package challenge

import (
	"fmt"
	"strconv"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/tron-us/go-common/v2/json"

	cidlib "github.com/ipfs/go-cid"
)

var StorageChallengeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with storage challenge requests and responses.",
		ShortDescription: `
These commands contain both client-side and host-side challenge functions.

btfs storage challenge request <peer-id> <contract-id> <file-hash> <shard-hash> <chunk-index> <nonce>
btfs storage challenge response <contract-id> <file-hash> <shard-hash> <chunk-index> <nonce>`,
	},
	Subcommands: map[string]*cmds.Command{
		"request":  storageChallengeRequestCmd,
		"response": StorageChallengeResponseCmd,
	},
}

var storageChallengeRequestCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Challenge storage hosts with Proof-of-Storage requests.",
		ShortDescription: `
This command challenges storage hosts on behalf of a client to see if hosts
still store a piece of file (usually a shard) as agreed in storage contract.`,
	},
	Arguments: append([]cmds.Argument{
		cmds.StringArg("peer-id", true, false, "Host Peer ID to send challenge requests."),
	}, StorageChallengeResponseCmd.Arguments...), // append pass-through arguments
	RunTimeout: 20 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		tm := TimeEvaluate{}
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}
		tm.RecordTime("GetConfig")

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		tm.RecordTime("GetNode")

		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		tm.RecordTime("GetApi")
		// Check if peer is reachable
		pi, err := remote.FindPeer(req.Context, n, req.Arguments[0])
		if err != nil {
			return err
		}
		tm.RecordTime("FindPeer")
		// Pass arguments through to host response endpoint
		resp, err := remote.P2PCallStrings(req.Context, n, api, pi.ID, "/storage/challenge/response",
			req.Arguments[1:]...)
		if err != nil {
			return err
		}

		tm.RecordTime("P2PCall")
		var scr StorageChallengeRes
		err = json.Unmarshal(resp, &scr)
		if err != nil {
			return err
		}
		tm.RecordTime("Unmarshall")
		scr.timeEvaluate = append(scr.timeEvaluate, tm)
		return cmds.EmitOnce(res, &scr)
	},
	Type: StorageChallengeRes{},
}

type TimeEvaluate struct {
	TmVal   []time.Time
	TmIndex int
	Event   []string
}

func (t *TimeEvaluate) init() {
	t.TmVal = make([]time.Time, 0)
	t.TmIndex = 0
	t.Event = make([]string, 0)
	t.RecordTime("Start")
}

func (t *TimeEvaluate) RecordTime(event string) {
	if t.TmVal == nil {
		t.init()
	}
	t.TmVal = append(t.TmVal, time.Now())
	t.Event = append(t.Event, event)
	t.TmIndex++
}
func (t *TimeEvaluate) Report() string {
	result := "Report(InNanoS):Start"
	for i := 1; i < t.TmIndex; i++ {
		costTime := t.TmVal[i].UnixNano() - t.TmVal[i-1].UnixNano()
		result = fmt.Sprintf("%s-%s:%d", result, t.Event[i], costTime)
	}
	tCo := t.TmVal[t.TmIndex-1].Unix() - t.TmVal[0].Unix()
	result = fmt.Sprintf("%s-Sum:%ds", result, tCo)
	return result
}

type StorageChallengeRes struct {
	Answer       string
	timeEvaluate []TimeEvaluate
}

var StorageChallengeResponseCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Storage host responds to Proof-of-Storage requests.",
		ShortDescription: `
This command (on host) reads the challenge question and returns the answer to
the challenge request back to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("contract-id", true, false, "Contract ID associated with the challenge requests."),
		cmds.StringArg("file-hash", true, false, "File root multihash for the data stored at this host."),
		cmds.StringArg("shard-hash", true, false, "Shard multihash for the data stored at this host."),
		cmds.StringArg("chunk-index", true, false, "Chunk index for this challenge. Chunks available on this host include root + metadata + shard chunks."),
		cmds.StringArg("nonce", true, false, "Nonce for this challenge. A random UUIDv4 string."),
	},
	RunTimeout: 1 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		tm := TimeEvaluate{}
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}
		tm.RecordTime("HGetConfig")

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		tm.RecordTime("HGetNode")
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		tm.RecordTime("HGetApi")
		fileHash, err := cidlib.Parse(req.Arguments[1])
		if err != nil {
			return err
		}
		tm.RecordTime("HParseFileCid")

		sh := req.Arguments[2]
		shardHash, err := cidlib.Parse(sh)
		if err != nil {
			return err
		}
		tm.RecordTime("HParseShardCid")
		chunkIndex, err := strconv.Atoi(req.Arguments[3])
		if err != nil {
			return err
		}
		nonce := req.Arguments[4]
		// Get (cached) challenge response object and solve challenge
		sc, err := NewStorageChallengeResponse(req.Context, n, api, fileHash, shardHash, "", false, 0)
		if err != nil {
			return err
		}
		tm.RecordTime("HNewResponse")

		err = sc.SolveChallenge(chunkIndex, nonce)
		if err != nil {
			return err
		}
		tm.RecordTime("HSolveChallenge")
		return cmds.EmitOnce(res, &StorageChallengeRes{
			Answer:       sc.Hash,
			timeEvaluate: []TimeEvaluate{tm},
		})
	},
	Type: StorageChallengeRes{},
}
