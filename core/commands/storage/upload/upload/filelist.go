package upload

import (
	"context"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
)

const (
	renterFilePageSize = 100
	guardTimeout       = 360 * time.Second
)

var FilelistCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "List file store information for renter.",
		ShortDescription: `
The command helps renter to retrieve the file information for storage, includes file hash, 
file size, rent start and rent end. Especially for renewing the contract for a new extended 
duration, renter can check files with the command and find the desired file to renew. 
`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to renew."),
		cmds.StringArg("renew-length", true, false, "New File storage period on hosts in days."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ctxParams, err := uh.ExtractContextParams(req, env)
		if err != nil {
			return err
		}
		renterId := ctxParams.N.Identity.String()
		var files []*guardpb.FileStoreMeta
		for i := 0; ; i++ {
			metaReq := &guardpb.ListRenterFileInfoRequest{
				RenterPid:        renterId,
				RequesterPid:     renterId,
				RequestTime:      time.Now().UTC(),
				RequestPageSize:  renterFilePageSize,
				RequestPageIndex: int32(i),
			}

			signedReq, err := crypto.Sign(ctxParams.N.PrivateKey, metaReq)
			if err != nil {
				return err
			}
			metaReq.Signature = signedReq

			cfg, err := ctxParams.N.Repo.Config()
			if err != nil {
				return err
			}

			fs, last, err := ListRenterFiles(ctxParams.Ctx, cfg, metaReq)
			if err != nil {
				return err
			}

			files = append(files, fs...)
			if last {
				break
			}
		}

		fileList := make([]FileInfo, len(files))
		for index, file := range files {
			fileList[index].FileHash = file.FileHash
			fileList[index].FileSize = file.FileSize
			fileList[index].RentStart = file.RentStart
			fileList[index].RentEnd = file.RentEnd
		}
		return res.Emit(fileList)
	},
	Type: FileInfo{},
}

type FileInfo struct {
	FileHash  string
	FileSize  int64
	RentStart time.Time
	RentEnd   time.Time
}

func ListRenterFiles(ctx context.Context, cfg *config.Config,
	listReq *guardpb.ListRenterFileInfoRequest) ([]*guardpb.FileStoreMeta, bool, error) {
	cb := grpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	var files []*guardpb.FileStoreMeta
	var lastPage bool
	err := cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
		res, err := client.RetrieveFileInfo(ctx, listReq)
		if err != nil {
			return err
		}
		files = res.FileStoreMeta
		lastPage = res.Count < listReq.RequestPageSize
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return files, lastPage, nil
}
