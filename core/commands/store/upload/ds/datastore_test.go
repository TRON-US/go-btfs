package ds

import (
	sessionpb "github.com/TRON-US/go-btfs/core/commands/store/upload/pb/session"
	coremock "github.com/TRON-US/go-btfs/core/mock"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestSaveGet(t *testing.T) {
	node, err := coremock.NewMockNode()
	if err != nil {
		t.Fatal(err)
	}

	shardHashes := make([]string, 0)
	shardHashes = append(shardHashes, "Qm1")
	shardHashes = append(shardHashes, "Qm2")
	shardHashes = append(shardHashes, "Qm3")
	current := time.Now().UTC()
	md := &sessionpb.Metadata{
		TimeCreate:  current,
		RenterId:    node.Identity.String(),
		FileHash:    "Qm123",
		ShardHashes: shardHashes,
	}
	err = Save(node.Repo.Datastore(), "ds.ds.test", md)
	if err != nil {
		t.Fatal(err)
	}
	newMd := &sessionpb.Metadata{}
	err = Get(node.Repo.Datastore(), "ds.ds.test", newMd)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, md, newMd)
}
