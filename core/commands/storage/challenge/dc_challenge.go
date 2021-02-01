package challenge

import (
	"compress/gzip"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/go-common/v2/json"
	"github.com/tron-us/protobuf/proto"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	"github.com/cenkalti/backoff/v4"
	logging "github.com/ipfs/go-log"

	"go.uber.org/zap"
)

const (
	challengeWorkerCount = 10
	hostChallengeTimeout = 10 * time.Minute
)

var (
	log              = logging.Logger("dc_challenge")
	isReadyChallenge = true
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
	fmt.Println("get into RequestChallenge")
	if !isReadyChallenge {
		return nil
	}
	isReadyChallenge = false
	result, err := retrieveQuestionAndChallenge(req, res, env, cfg)
	fmt.Println("result.Code", result.Code)
	if result != nil || err != nil {
		isReadyChallenge = true
	}
	return err
}

func retrieveQuestionAndChallenge(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, cfg *config.Config) (*guardpb.Result, error) {
	fmt.Println("get into retrieveQuestionAndChallenge")
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
		fmt.Println("challengeResp", challengeResp.PackageQuestionsCount, challengeResp.PackageUrl, challengeResp.NodePid)
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
	fmt.Println("questions length", len(questions))
	fmt.Println("question 0 information", len(questions), questions[0].FileHash, questions[0].FileHash, questions[0].ShardHash, questions[0].Nonce, questions[0].ChunkIndex)
	if len(questions) != int(challengeResp.PackageQuestionsCount) {
		fmt.Printf("question amount is not correct, expected {%d} got {%d}\n", challengeResp.PackageQuestionsCount, len(questions))
		return nil, fmt.Errorf("question amount is not correct, expected {%d} got {%d}", challengeResp.PackageQuestionsCount, len(questions))
	}

	requestChan := make(chan *guardpb.DeQuestion, len(questions))
	resultChan := make(chan *guardpb.ShardChallengeResult, len(questions))
	for _, question := range questions {
		requestChan <- question
	}

	var wg sync.WaitGroup
	for count := 0; count < challengeWorkerCount; count++ {
		wg.Add(1)
		go doChallenge(req, res, env, requestChan, resultChan, &wg)
	}
	wg.Wait()

	challengeResults := checkChallengeResults(len(questions), resultChan)
	fmt.Println("challengeResults length", len(challengeResults))
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
		fmt.Println("client.SubmitChallengeJobResult return", result.Code, result.Message)
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
	fmt.Println("challenge jobs done")
	return result, nil
}

func doChallenge(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, requestChan <-chan *guardpb.DeQuestion, resultChan chan<- *guardpb.ShardChallengeResult, wg *sync.WaitGroup) {
	fmt.Println("get into doChallenge")
	defer wg.Done()
	for question := range requestChan {
		challengeResult := &guardpb.ShardChallengeResult{
			HostPid:   question.HostPid,
			FileHash:  question.FileHash,
			ShardHash: question.ShardHash,
			Nonce:     question.Nonce,
		}
		err := backoff.Retry(func() error {
			storageChallengeRes, err := respChallengeResult(req, res, env, question)
			if err != nil {
				//debug here
				fmt.Println("respChallengeResult failed err", err)
				return err
			}
			challengeResult.Result = storageChallengeRes.Answer
			fmt.Println("challengeResult.Result", challengeResult.Result)
			return nil
		}, challengeHostBo)
		if err != nil {
			challengeResult.IsTimeout = true
		}
		resultChan <- challengeResult
	}
	fmt.Println("single go routine returned")
}

func respChallengeResult(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment, question *guardpb.DeQuestion) (*StorageChallengeRes, error) {
	fmt.Println("get into respChallengeResult")
	n, err := cmdenv.GetNode(env)
	if err != nil {
		return nil, err
	}
	var scr *StorageChallengeRes
	chunkIndex := strconv.Itoa(int(question.ChunkIndex))
	if question.HostPid == n.Identity.Pretty() {
		scr, err = respChallengeStorage(req, res, env, question.FileHash, question.ShardHash, chunkIndex, question.Nonce)
		if err != nil {
			return nil, err
		}
	} else {
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return nil, err
		}
		pi, err := remote.FindPeer(req.Context, n, question.HostPid)
		if err != nil {
			return nil, err
		}
		resp, err := remote.P2PCallStrings(req.Context, n, api, pi.ID, "/storage/challenge/response",
			"",
			question.FileHash,
			question.ShardHash,
			chunkIndex,
			question.Nonce,
		)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(resp, &scr)
		if err != nil {
			return nil, err
		}
	}
	return scr, nil
}

func checkChallengeResults(questionNum int, resultChan <-chan *guardpb.ShardChallengeResult) []*guardpb.ShardChallengeResult {
	fmt.Println("get into checkChallengeResults")
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

func requestQuestions(questionUrl string) ([]*guardpb.DeQuestion, error) {
	fmt.Println("get into requestQuestions")
	resp, err := http.Get(questionUrl)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	defer resp.Body.Close()

	rawData, err := ioutil.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("receive wrong status code %d response: %s", resp.StatusCode, string(rawData))
	}

	dcQuestions := new(guardpb.DeCentralQuestions)
	if err := proto.Unmarshal(rawData, dcQuestions); err != nil {
		fmt.Println("decentralized questions unmarshal error", err)
		return nil, err
	}

	return dcQuestions.Qs, nil
}
