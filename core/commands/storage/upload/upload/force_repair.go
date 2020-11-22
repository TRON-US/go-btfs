package upload

import (
	"context"
	"fmt"
	cmds "github.com/TRON-US/go-btfs-cmds"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"go.uber.org/zap"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	//fileHash    = "QmYruB8KjxHmKg27eeHL2oQWX6R7T3pkoZe23YRt9UvdDD"
	authPid      = "16Uiu2HAm1yEfFmzC1enfBcfbwf51YA15e4tRd9VRT65TELA1ykAD" //GuardPidPretty
	guardPrivKey = "CAISIJJTFKM777Y0S+pjlSSyKQtZTc7vEQDdAJReLgUMoGpz"
)

var StorageForceRepairCmd = &cmds.Command{
	//Arguments: []cmds.Argument{
	//	cmds.StringArg("file-hash", true, false, "Hash of file to upload."),
	//},
	RunTimeout: 15 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		    //DeCentralBroadcastUrl = "http://%s/api/v1/storage/dcrepair/request?arg=%s&arg=%s&arg=%s&arg=%d&arg=%d&arg=%d"

			//challengeUrl := fmt.Sprintf(DeCentralBroadcastUrl, router, strings.Join(nodes, ","), cmd.FileHash, strings.Join(cmd.LostShardHash, ","), cmd.FileSize, cmd.DownloadReward, cmd.RepairReward)
			testUrl := "http://16Uiu2HAmRZYwkgRy1WUaDnggtUuP5gAV81DDPAP1PLeK43vXkLjj/api/v1/storage/upload/init?arg=639c1da5-e190-4fe8-a955-231abbaa986e&arg=QmdPNysegcLpXWyNwB6mFyYkAgQCG5LL9begkXNhVJtMvP&arg=QmRzi1gAVzTHMWn5Xwv7WPNXWjAp3UHzyzFUckncnvdgPV&arg=250000&arg=&arg=%0A%8D%03%0A%24673fc829-cdf9-4794-a1bd-641a5dc086f6%12516Uiu2HAmCF5CC4qVsBJ2brLg7dtwHk47jQFpc1FA7BHReiTh3oEx%1A516Uiu2HAmCpE9UpKTcgufkBXCCsWq3GtkpbZ3kNQKd8jtRAMG7QaX%22.QmRzi1gAVzTHMWn5Xwv7WPNXWjAp3UHzyzFUckncnvdgPV0%BA%93%80%05%3A.QmdPNysegcLpXWyNwB6mFyYkAgQCG5LL9begkXNhVJtMvPB%0C%08%DA%CF%DA%FD%05%10%9A%EF%B5%93%02J%0B%08%B7%DB%F8%FE%05%10%88%AA%DD4R516Uiu2HAm1yEfFmzC1enfBcfbwf51YA15e4tRd9VRT65TELA1ykADZ516Uiu2HAkzhsnMffxirhJ39ZMdPcXdrcbfrZsvcQdUeVwSBzBqBK7%60%90%A1%0Fh%F7%0F%88%01%97%03%10%06%2A%0C%08%BF%C1%DA%FD%05%10%D8%BD%F2%EF%012F0D%02+j%03%9E%0E%E0A%1E%AE%28%E4%D4%BB%E1z%0C%118%22%7Fq%EBT%5D%82D%E4%B5m%CE%0A%D2j%02+c%88%FB%2B%93%CD%DC%7B+%0A%82%E3%7B%1Fh%B9%B8%8E%05%09%13%9CE%C2q%FD%E2%21Pu%0C5B%0C%08%DA%CF%DA%FD%05%10%84%81%B6%93%02R516Uiu2HAmRJYNshY9DJvWmHAQLjDJz1PG68wktSEBSNErhM1fYy4hZG0E%02%21%00%94%08%9E%A2%AC%40%B6%0D%E3%D7%1B8uq%7B%B4X%7C%06c%3B%AA%15Ny_+%C8%00%3F%2FX%02+B%CA%84%BE%BF%83%27%16O%96%C6%CA%1En%09%81%E3%DF0_%80%22%22n%9F%AD%1A%23%FB%99fRb%0B%08%80%92%B8%C3%98%FE%FF%FF%FF%01j%0B%08%80%92%B8%C3%98%FE%FF%FF%FF%01%8A%01%0B%08%80%92%B8%C3%98%FE%FF%FF%FF%01&arg=-1&arg=10488250&arg=0&arg=16Uiu2HAmRJYNshY9DJvWmHAQLjDJz1PG68wktSEBSNErhM1fYy4h"
			log.Debug("call broadcast repair url:", zap.String("url", testUrl))
			resp, err := http.Post(testUrl, "", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			s, err := ioutil.ReadAll(io.Reader(resp.Body))
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 {
				return fmt.Errorf("got wrong status code %d response: %s", resp.StatusCode, string(s))
			}

			//return nil

		/*	ctxParams, err := uh.ExtractContextParams(req, env)
			if err != nil {
				return nil
			}
			hp := uh.GetHostsProvider(ctxParams, make([]string, 0))

			//newContracts := make([]*guardpb.Contract, 0)
			newCtx, _ := context.WithTimeout(context.Background(), 10*time.Second)

			getHostId (hp, newCtx)*/

/*		host, err := hp.NextValidHost(250000)
		if err != nil {
			fmt.Println("hp.NextValidHost err")
			return err
		}
		fmt.Println("host is", host)*/

		//n, err := cmdenv.GetNode(env)
		//if err != nil {
		//	return err
		//}
		//var renterPid = n.Identity.String()
		//cfg, err := n.Repo.Config()
		//if err != nil {
		//	return err
		//}
		//
		//fileHash := req.Arguments[0]
		//forceRequest := &guardpb.ForceRepairRequest{
		//	RenterPid: renterPid,
		//	FileHash:  fileHash,
		//	AuthPid:   authPid,
		//	Signature: nil,
		//}
		//privKey, err := crypto.ToPrivKey(guardPrivKey)
		//if err != nil {
		//	return err
		//}
		//sign, err := crypto.Sign(privKey, forceRequest)
		//forceRequest.Signature = sign
		//
		//var response *guardpb.Result
		//err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(req.Context, func(ctx context.Context,
		//	client guardpb.GuardServiceClient) error {
		//	response, err = client.ForceRepair(ctx, forceRequest)
		//	if err != nil {
		//		return err
		//	}
		//	return nil
		//})
		//if err != nil {
		//	log.Panic("send force repair build request error", zap.Error(err))
		//}
		//log.Info("send force repair build request ", zap.String("response", response.Code.String()))
		//ebf84226-bb4b-40f6-a1cf-138b7da77854
		//ret := &Return{
		//	RenterPid: renterPid,
		//	FileHash:  fileHash,
		//}
		//return res.Emit(ret)
		/*testUrl := "“http://guard-repairer-app-0.guard-repairer-service:5001/api/v1/storage/dcrepair/request?arg=16Uiu2HAmQzTgUkb27GQ8FbU9fuQVPSN6y8Vbgemfx7Mn3pVxDcsg,16Uiu2HAmRJYNshY9DJvWmHAQLjDJz1PG68wktSEBSNErhM1fYy4h&arg=QmTD6rYspvPqyfB2mpWRVrVkPe5iZmparQ6e1DK8k3bruC&arg=Qmb8wtJkohbQCQTHX83fYZGSWSc7oq9ohVNBPR5etehBnE,QmYtUj813h2X9ctLfds4VuLfuGChedRZvxyF3TwBZnkqJa,QmTYUVVTAGNmmYPCVEgbBy6cP2aMy2pttbJVA57FvPJuJq,QmWWF4XsvGHR8QbqssMJrq2S9GwrA3uMikf6t73sdSoTxR,QmNpiWyEDzQCZXAcpXKRF9AiDiJMDEWJZY6WckUaLaF5qo,QmfB5zVFsfnRAvYHZtTkz6owpTtooRbmJH47un2imMDoMS,Qmf2UgYn6p5Ca9WR5XhBQ8feYaojkn4bag6EdGW1rXj7WA,QmPUdUNDnZMVDVhj3jWhKv2FcJQvB49SuH357sNv7Qmrgw,QmeAghSGxr69bH6EpQBuiowKHnpEAUX7a2T53XrQp6yFdg,QmRuPa4qHXevh7647XL2THTD7Pm2AdohTXhXT1w8wRa7k5,QmbVNqcXN4auJsbbMSEUu9ppR446KRAJrGBv65cXNgxi8v,QmRdbjiZXmauUmHL6TnUDKJrsVVvvBKJ3ZUe9ZN2Fvmgq4,Qmd6BQbh9TpQzSUSvumfjp5tuLt5piU9CUXJUzhjEv1Mzd,Qmcxb35b6E3U2GsRAikz9Re7t9xTm9iCcZfPUtkuPpRsK1,QmVPiR8psR4sXq4gwk5ssJx5SyvQRBRQuUehn8fJPEEU4S,QmSS2p8G4yc9x89z6zzejP5BZanPLQU6FwNgBbLoT9Wu9L,QmR5ciEcbETp7WoWQp4Q39HzuXnnUwyZqAqQFgpSRrVorg&arg=104857600&arg=786432000000000&arg=78643200000000"
		resp, err := http.Post(testUrl, "", nil)

		defer resp.Body.Close()
		_, err = ioutil.ReadAll(io.Reader(resp.Body))
		if err != nil {
			fmt.Println("err with http call", err)
			//return err
		}
		if resp.StatusCode != 200 {
			fmt.Println("resp.StatusCode", resp.StatusCode)
		}*/
		return nil
	},
	//Type: Return{},
}

func getHostId(hp uh.IHostsProvider, ctx context.Context) error {
	go func() {
		select {
		case <-ctx.Done(): //ctx.Done():
			timeStr := time.Now().Format("2006-01-02 15:04:05")
			fmt.Println("end time of downloadAndSignContracts", timeStr)
			fmt.Println("ctx.Done() for downloadAndSignContracts")
		default:
			break
		}
		host, err := hp.NextValidHost(250000)
		if err != nil {
			fmt.Println("hp.NextValidHost", err)
		}
		fmt.Println("host is", host)
	}()
	return nil
}


type Return struct {
	RenterPid string
	FileHash  string
}
