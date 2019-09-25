package remote

import logging "github.com/ipfs/go-log"

type Call interface {
	CallGet(string, []string) ([]byte, error)
	CallPost()
}

var log = logging.Logger("corehttp/remote")

// Must exist here to avoid circular dependency from
// core/commands -> core/corehttp/remote -> core/commands
const apiPrefix = "/api/v0"
