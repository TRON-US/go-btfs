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
			_, err := commands.GetSettings(node.Context(), c.Services.HubDomain, node.Identity.Pretty(),
				node.Repo.Datastore())
			if err != nil {
				log.Error("error occured when getting settings, error: ", err.Error())
			}
		}()
	}

	//TODO: spin
}
