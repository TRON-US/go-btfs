package storage

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"testing"

	"github.com/TRON-US/go-btfs/core/hub"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"

	unixtest "github.com/TRON-US/go-btfs/core/coreunix/test"
)

func TestHostsSaveGet(t *testing.T) {
	node := unixtest.HelpTestMockRepo(t)

	// test all possible modes
	for _, mode := range []string{hub.HubModeAll, hub.HubModeScore,
		hub.HubModeGeo, hub.HubModeRep, hub.HubModePrice, hub.HubModeSpeed} {
		var nodes []*hubpb.Host
		for i := 0; i < 100; i++ {
			ni := &hubpb.Host{NodeId: fmt.Sprintf("%s:node:%d", mode, i)}
			nodes = append(nodes, ni)
		}
		err := SaveHostsIntoDatastore(context.Background(), node, mode, nodes)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := GetHostsFromDatastore(context.Background(), node, mode, 100)
		if err != nil {
			t.Fatal(err)
		}
		for i, sn := range stored {
			bs1, _ := proto.Marshal(sn)
			bs2, _ := proto.Marshal(nodes[i])
			if hex.EncodeToString(bs1) != hex.EncodeToString(bs2) {
				t.Fatal("stored nodes do not match saved nodes")
			}
		}
	}
}
