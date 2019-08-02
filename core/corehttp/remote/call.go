package remote

type Call interface {
	CallGet(string, []string) (map[string]interface{}, error)
	CallPost() // TODO
}

// Must exist here to avoid circular dependency from
// core/commands -> core/corehttp/remote -> core/commands
const apiPrefix = "/api/v0"
