package files

import (
	"context"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"sort"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"

	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
)

const (
	renterFilePageSize = 100
	guardTimeout       = 360 * time.Second
)

type ByTime []*guardpb.FileStoreMeta

func (a ByTime) Len() int           { return len(a) }
func (a ByTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTime) Less(i, j int) bool { return a[i].RentEnd.UnixNano() < a[j].RentEnd.UnixNano() }

var FileListCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "List file store information for renter.",
		ShortDescription: `
The command helps renter to retrieve the file information for storage, includes file hash, 
file size, rent start and rent end. Especially for renewing the contract for a new extended 
duration, renter can check files with the command and find the desired file to renew. 
`,
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		files, latest, err := sessions.ListRenterFiles(n.Repo.Datastore(), n.Identity.Pretty())
		if err != nil {
			return err
		}

		renterId := n.Identity.String()
		var updatedFiles []*guardpb.FileStoreMeta
		for i := 0; ; i++ {
			metaReq := &guardpb.ListRenterFileInfoRequest{
				RenterPid:        renterId,
				RequesterPid:     renterId,
				LastModifyTime:   &latest,
				RequestPageSize:  renterFilePageSize,
				RequestPageIndex: int32(i),
			}

			signedReq, err := crypto.Sign(n.PrivateKey, metaReq)
			if err != nil {
				return err
			}
			metaReq.Signature = signedReq
			cfg, err := n.Repo.Config()
			if err != nil {
				return err
			}

			fs, last, err := GetUpdatedRenterFiles(req.Context, cfg, metaReq)
			if err != nil {
				return err
			}

			updatedFiles = append(updatedFiles, fs...)
			if last {
				break
			}
		}

		allFiles, err := sessions.SaveFilesMeta(n.Repo.Datastore(), n.Identity.Pretty(), files, updatedFiles)
		if err != nil {
			return err
		}
		sort.Sort(ByTime(allFiles))
		fileList := make([]FileInfo, len(allFiles))
		for index, file := range allFiles {
			fileList[index].FileHash = file.FileHash
			fileList[index].FileSize = file.FileSize
			fileList[index].RentStart = file.RentStart
			fileList[index].RentEnd = file.RentEnd
		}
		return cmds.EmitOnce(res, fileList)
	},
	Type: []FileInfo{},
}

type FileInfo struct {
	FileHash  string
	FileSize  int64
	RentStart time.Time
	RentEnd   time.Time
}

func GetUpdatedRenterFiles(ctx context.Context, cfg *config.Config,
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
