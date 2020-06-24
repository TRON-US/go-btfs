package sessions

import (
	"fmt"
	"strconv"
	"time"

	"github.com/ipfs/go-datastore"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/protobuf/proto"
)

const (
	renterFilePrefix    = "/btfs/%s/renter/files/" // %s peerID
	renterFileKey       = renterFilePrefix + "%s/" // %s fileHash
	renterFilesInMemKey = renterFileKey + "%s"     // %s latest
)

func ListRenterFiles(d datastore.Datastore, peerId string) ([]*guardpb.FileStoreMeta, time.Time, error) {
	var k = fmt.Sprintf(renterFilePrefix, peerId)

	vs, latest, err := ListFiles(d, k)
	if err != nil {
		return nil, latest, err
	}

	files := make([]*guardpb.FileStoreMeta, 0)
	for _, v := range vs {
		fm := &guardpb.FileStoreMeta{}
		err := proto.Unmarshal(v, fm)
		if err != nil {
			log.Error(err)
			continue
		}
		files = append(files, fm)
	}
	return files, latest, nil
}

func SaveFilesMeta(ds datastore.Datastore, peerId string, files []*guardpb.FileStoreMeta, updatedFiles []*guardpb.FileStoreMeta) ([]*guardpb.FileStoreMeta, error) {
	var ks []string
	var vs []proto.Message

	fmap := map[string]*guardpb.FileStoreMeta{}
	for _, ufile := range updatedFiles {
		fmap[ufile.FileHash] = ufile
	}

	timestamp := time.Now().Unix()
	tsString := strconv.FormatInt(timestamp, 10)

	for _, file := range files {
		if v, ok := fmap[file.FileHash]; ok {
			ks = append(ks, fmt.Sprintf(renterFilesInMemKey, peerId, file.FileHash, tsString))
			file = v
			vs = append(vs, file)
			delete(fmap, file.FileHash)
		}
	}

	for fileHash, file := range fmap {
		ks = append(ks, fmt.Sprintf(renterFilesInMemKey, peerId, fileHash, tsString))
		vs = append(vs, file)
		files = append(files, file)
	}

	if len(ks) > 0 {
		err := Batch(ds, ks, vs)
		if err != nil {
			return nil, err
		}
	}

	return files, nil
}
