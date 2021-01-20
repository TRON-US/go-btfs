package challenge

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/go-common/v2/json"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"

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
	challengeWorkerCount = 10
	hostChallengeTimeout = 10 * time.Minute
)

var (
	wg               = sync.WaitGroup{}
	isReadyChallenge = true
	log              = logging.Logger("dc_challenge")
	challengeHostBo  = func() *backoff.ExponentialBackOff {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 10 * time.Second
		bo.MaxElapsedTime = hostChallengeTimeout
		bo.Multiplier = 1
		bo.MaxInterval = 10 * time.Second
		return bo
	}()
)

func RequestChallenge(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, cfg *config.Config) error {
	if !isReadyChallenge {
		return nil
	}
	isReadyChallenge = false
	result, err := retrieveQuestionAndChallenge(req, res, env, cfg)
	if result != nil || err != nil {
		isReadyChallenge = true
	}
	return err
}

func retrieveQuestionAndChallenge(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, cfg *config.Config) (*guardpb.Result, error) {
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

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var challengeResp *guardpb.ChallengeJobResponse
	err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(ctx, func(ctx context.Context,
		client guardpb.GuardServiceClient) error {
		challengeResp, err = client.RequestForChallengeJob(ctx, challengeReq)
		if err != nil {
			fmt.Printf("unable to request challenge job for peer id {%s}: %v\n", peerId, err)
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Printf("grpc error of request challenge job for peer id {%s}: %v\n", peerId, err)
		return nil, err
	}
	if challengeResp == nil || challengeResp.PackageUrl == "" || challengeResp.PackageQuestionsCount == 0 {
		fmt.Printf("challenge response for peer id {%s} is not available\n", peerId)
		return nil, fmt.Errorf("challenge response for peer id {%s} is not available", peerId)
	}

	fmt.Println("question url received:", zap.String("url", challengeResp.PackageUrl))
	questions, err := requestQuestions(challengeResp.PackageUrl)
	if err != nil {
		fmt.Printf("request questions error for url: {%s}\n", challengeResp.PackageUrl)
		return nil, err
	}

	requestChan := make(chan struct{}, challengeWorkerCount)
	resultChan := make(chan *guardpb.ShardChallengeResult, len(questions))
	for _, question := range questions {
		requestChan <- struct{}{}
		go doChallenge(req, res, env, question, requestChan, resultChan)
	}
	wg.Wait()

	challengeResults := checkChallengeResults(len(questions), resultChan)
	challengeJobResult := &guardpb.ChallengeJobResult{
		NodePid:    peerId,
		JobId:      challengeResp.JobId,
		Result:     challengeResults,
		SubmitTime: time.Now().UTC(),
	}
	sig, err = crypto.Sign(n.PrivateKey, challengeJobResult)
	if err != nil {
		fmt.Printf("sign challenge job result error for peer id {%s}: %v\n", peerId, err)
		return nil, err
	}
	challengeJobResult.Signature = sig

	var result *guardpb.Result
	err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(ctx, func(ctx context.Context,
		client guardpb.GuardServiceClient) error {
		result, err = client.SubmitChallengeJobResult(ctx, challengeJobResult)
		if err != nil {
			fmt.Printf("unable to submit challenge job result for peer id {%s}: %v\n", peerId, err)
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Printf("grpc error of submit challenge job result for peer id {%s} : %v\n", peerId, err)
		return nil, err
	}
	if result.Code != guardpb.ResponseCode_SUCCESS {
		fmt.Printf("submit challenge job response error for peer id {%s}, response code {%d}\n", peerId, result.Code)
		return nil, fmt.Errorf("submit challenge job response error for peer id {%s}, response code {%d}", peerId, result.Code)
	}
	return result, nil
}

func doChallenge(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, question *Question, requestChan <-chan struct{}, resultChan chan<- *guardpb.ShardChallengeResult) {
	defer wg.Done()
	wg.Add(1)
	errChan := make(chan error)
	challengeResult := &guardpb.ShardChallengeResult{
		HostPid:   question.HostPid,
		FileHash:  question.FileHash,
		ShardHash: question.ShardHash,
		Nonce:     question.Nonce,
	}
	go func() {
		err := backoff.Retry(func() error {
			chunkIndex := strconv.Itoa(int(question.ChunkIndex))
			storageChallengeRes, err := ReqChallengeStorage(req, res, env, question.HostPid, question.FileHash, question.ShardHash, chunkIndex, question.Nonce)
			if err != nil {
				return err
			}
			challengeResult.IsTimeout = false
			challengeResult.Result = storageChallengeRes.Answer
			return nil
		}, challengeHostBo)
		errChan <- err
	}()
	select {
	case err := <-errChan:
		if err != nil {
			fmt.Println("failed to challenge shard", zap.String("shard hash", question.ShardHash), zap.String("host pid", question.HostPid), zap.Error(err))
		}
		resultChan <- challengeResult
		<-requestChan
		return
	case <-time.After(hostChallengeTimeout):
		challengeResult.IsTimeout = true
		resultChan <- challengeResult
		<-requestChan
		fmt.Println("challenge shard timeout", zap.String("shard hash", question.ShardHash), zap.String("host pid", question.HostPid))
		return
	}
}

func checkChallengeResults(questionNum int, resultChan <-chan *guardpb.ShardChallengeResult) []*guardpb.ShardChallengeResult {
	challengeResults := make([]*guardpb.ShardChallengeResult, 0)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if questionNum == len(resultChan) {
			for result := range resultChan {
				challengeResults = append(challengeResults, result)
			}
			break
		}
	}
	return challengeResults
}

func requestQuestions(questionUrl string) ([]*Question, error) {
	resp, err := http.Get(questionUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rawData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("receive wrong status code %d response: %s", resp.StatusCode, string(rawData))
	}

	var questions []*Question
	if err := json.Unmarshal(rawData, &questions); err != nil {
		return nil, err
	}
	return questions, nil
}
