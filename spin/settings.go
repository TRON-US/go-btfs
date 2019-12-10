package spin

import (
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands"
)

func Settings(node *core.IpfsNode) {
	c, err := node.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return
	}

	if c.Experimental.StorageHostEnabled {
		go func() {
			commands.GetSettings(c.Services.HubDomain, node.Identity.Pretty(), node.Repo.Datastore())
		}()
	}

	//TODO: spin
}
