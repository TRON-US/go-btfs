package ds

import (
	"fmt"
	"testing"
	"time"

	coremock "github.com/TRON-US/go-btfs/core/mock"
	sessionpb "github.com/TRON-US/go-btfs/protos/session"

	"github.com/stretchr/testify/assert"
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

func TestList(t *testing.T) {
	node, err := coremock.NewMockNode()
	if err != nil {
		t.Fatal(err)
	}
	list, err := List(node.Repo.Datastore(), "/", 10)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range list {
		fmt.Println("i", i, "m", m)
	}
}
