package storage

import (
	"context"
	"fmt"
	"testing"

	"github.com/TRON-US/go-btfs/core/hub"
	"github.com/tron-us/go-btfs-common/info"

	unixtest "github.com/TRON-US/go-btfs/core/coreunix/test"
)

func TestHostsSaveGet(t *testing.T) {
	node := unixtest.HelpTestMockRepo(t)

	// test all possible modes
	for _, mode := range []string{hub.HubModeAll, hub.HubModeScore,
		hub.HubModeGeo, hub.HubModeRep, hub.HubModePrice, hub.HubModeSpeed} {
		var nodes []*info.Node
		for i := 0; i < 100; i++ {
			ni := &info.Node{NodeID: fmt.Sprintf("%s:node:%d", mode, i)}
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
			if *sn != *nodes[i] {
				t.Fatal("stored nodes do not match saved nodes")
			}
		}
	}
}
