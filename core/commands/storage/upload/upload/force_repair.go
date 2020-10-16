package upload

import (
	"context"
	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"go.uber.org/zap"
	"time"
)

const (
	//fileHash    = "QmYruB8KjxHmKg27eeHL2oQWX6R7T3pkoZe23YRt9UvdDD"
	authPid     = "16Uiu2HAm1yEfFmzC1enfBcfbwf51YA15e4tRd9VRT65TELA1ykAD" //GuardPidPretty
	guardPrivKey = "CAISIJJTFKM777Y0S+pjlSSyKQtZTc7vEQDdAJReLgUMoGpz"
)

var StorageForceRepair = &cmds.Command{
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to upload."),
		//cmds.StringArg("upload-peer-id", false, false, "Peer id when upload upload."),
		//cmds.StringArg("upload-nonce-ts", false, false, "Nounce timestamp when upload."),
		//cmds.StringArg("upload-signature", false, false, "Session signature when upload upload."),
	},
	RunTimeout: 15 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		//fmt.Println("Test Debugging")
		//time.Sleep(time.Second * 20)
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		var renterPid = n.Identity.String()
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}

		fileHash := req.Arguments[0]
		forceRequest := &guardpb.ForceRepairRequest{
			RenterPid: renterPid,
			FileHash:  fileHash,
			AuthPid:   authPid,
			Signature: nil,
		}
		privKey, err := crypto.ToPrivKey(guardPrivKey)
		if err != nil {
			return err
		}
		sign, err := crypto.Sign(privKey, forceRequest)
		forceRequest.Signature = sign

		var response *guardpb.Result
		err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(req.Context, func(ctx context.Context,
			client guardpb.GuardServiceClient) error {
			response, err = client.ForceRepair(ctx, forceRequest)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			log.Panic("send force repair build request error", zap.Error(err))
		}
		log.Info("send force repair build request ", zap.String("response", response.Code.String()))

		ret := &Return{
			RenterPid: renterPid,
			FileHash:  fileHash,
		}
		return res.Emit(ret)
	},
	Type: Return{},
}

type Return struct {
	RenterPid string
	FileHash  string
}

/*func forceRebuild(c guard.GuardServiceClient, status *guard.FileStoreStatus) {
	forceRequest := guard.ForceRepairRequest{
		RenterPid: status.RenterPid,
		FileHash:  status.FileHash,
		AuthPid:   config.GuardPidPretty,
		Signature: nil,
	}

	signature, err := crypto.Sign(config.GuardPrivateKey, &forceRequest)

	if err != nil {
		log.Panic("got error to sign the ready for challenge request", zap.Error(err))
	}
	forceRequest.Signature = signature

	response, err := c.ForceRepair(context.Background(), &forceRequest)
	if err != nil {
		log.Panic("send force repair build request error", zap.Error(err))
	}

	log.Info("send force repair build request ", zap.String("response", response.Code.String()))
}*/
