package challenge

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/go-common/v2/json"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/repo"

	"github.com/cenkalti/backoff/v4"
	logging "github.com/ipfs/go-log"
)

type Question struct {
	FileHash     string
	ShardHash    string
	HostPid      string
	ChunkIndex   int32
	Nonce        string
	ExpectAnswer string
}

const (
	hostReqChallengePeriod = 2 * time.Second
	hostChallengeTimeout   = 10 * time.Minute
)

var challengeHostBo = func() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 2 * time.Second
	bo.MaxElapsedTime = hostChallengeTimeout
	bo.Multiplier = 1
	bo.MaxInterval = 2 * time.Second
	return bo
}()

var log = logging.Logger("dc_challenge")

func RequestChallenge(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, cfg *config.Config) {
	timer := time.NewTimer(hostReqChallengePeriod)
	defer timer.Stop()
	for ; true; <-timer.C {
		result, err := retrieveChallengeQuestion(req, res, env, cfg)
		if err != nil || result != nil {
			timer.Reset(hostReqChallengePeriod)
		}
	}
}

func retrieveChallengeQuestion(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, cfg *config.Config) (*guardpb.Result, error) {
	n, err := cmdenv.GetNode(env)
	peerId := n.Identity.Pretty()

	challengeReq := &guardpb.ChallengeJobRequest{
		NodePid:     peerId,
		RequestTime: time.Now().UTC(),
	}
	sig, err := crypto.Sign(n.PrivateKey, challengeReq)
	if err != nil {
		fmt.Println("crypto.Sign err", err)
		return nil, err
	}
	challengeReq.Signature = sig

	ctx, _ := helper.NewGoContext(req.Context)
	var challengeResp *guardpb.ChallengeJobResponse
	err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(ctx, func(ctx context.Context,
		client guardpb.GuardServiceClient) error {
		challengeResp, err = client.RequestForChallengeJob(ctx, challengeReq)
		if err != nil {
			fmt.Printf("client.SubmitRepairContract err for peer id {%s}: %v\n", challengeReq.NodePid, err)
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Println("grpc.GuardClient err", err)
		return nil, err
	}
	if challengeResp == nil || challengeResp.PackageQuestionsCount == 0 {
		fmt.Printf("failed to reuqest challenge job for peer id: {%s}\n", n.Identity.Pretty())
		return nil, fmt.Errorf("failed to reuqest challenge job for peer id: {%s}", n.Identity.Pretty())
	}

	rawData, err := repo.DefaultS3Connection.RetrieveData(challengeResp.PackageUrl)
	if err != nil {
		fmt.Printf("can not get challenge questions from S3 for peer id {%s}: %v\n", challengeReq.NodePid, err)
		return nil, err
	}
	if len(rawData) == 0 {
		fmt.Printf("failed to retrieve challenge question for peer id: %s\n", n.Identity.Pretty())
		return nil, fmt.Errorf("failed to retrieve challenge question for peer id: %s", n.Identity.Pretty())
	}

	var data []Question
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, err
	}
	challengeResults := make([]*guardpb.ChallengeResult, 0, len(data))
	for _, question := range data {
		errChan := make(chan error)
		var challengeResult *guardpb.ChallengeResult
		go func() {
			err := backoff.Retry(func() error {
				go func() {
					chunkIndex := strconv.Itoa(int(question.ChunkIndex))
					storageChallengeRes, err := ReqChallengeStorage(req, res, env, question.HostPid, question.FileHash, question.ShardHash, chunkIndex, question.Nonce)
					if err != nil {
						errChan <- err
					}
					challengeResult = &guardpb.ChallengeResult{
						HostPid:   question.HostPid,
						ShardHash: question.FileHash,
						Nonce:     question.Nonce,
						Result:    storageChallengeRes.Answer,
					}
				}()
				select {
				case err := <-errChan:
					return err
				case <-time.After(hostChallengeTimeout):
					challengeResult.IsTimeout = true
					return nil
					//fmt.Errorf("challenge shard {%s} timeout for host id {%s}", question.ShardHash, question.HostPid)
				}
			}, challengeHostBo)
			if err != nil {
				fmt.Printf("failed to challenge shard {%s} for host id {%s}: %v\n", question.ShardHash, question.HostPid, err)
				log.Errorf("failed to challenge shard {%s} for host id {%s}: %v", question.ShardHash, question.HostPid, err)
			} else {
				challengeResults = append(challengeResults, challengeResult)
			}
		}()
	}

	challengeJobResult := &guardpb.ChallengeJobResult{
		NodePid:    peerId,
		JobId:      challengeResp.JobId,
		Result:     challengeResults,
		SubmitTime: time.Now().UTC(),
	}
	sig, err = crypto.Sign(n.PrivateKey, challengeJobResult)
	if err != nil {
		fmt.Println("crypto.Sign err", err)
		return nil, err
	}
	challengeJobResult.Signature = sig

	var result *guardpb.Result
	err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(ctx, func(ctx context.Context,
		client guardpb.GuardServiceClient) error {
		result, err = client.SubmitChallengeJobResult(ctx, challengeJobResult)
		if err != nil {
			fmt.Printf("unable to submit challenge job result for peer id {%s}: %v\n", challengeJobResult.NodePid, err)
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Printf("grpc err for peer id {%s} : %v\n", challengeJobResult.NodePid, err)
		return nil, err
	}
	if result.Code != guardpb.ResponseCode_SUCCESS {
		fmt.Printf("submit challenge job response err for peer id {%s}, response code {%d}\n", challengeJobResult.NodePid, result.Code)
		return nil, fmt.Errorf("submit challenge job response err for peer id {%s}, response code {%d}", challengeJobResult.NodePid, result.Code)
	}
	return result, nil
}
