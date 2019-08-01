package remote

type Call interface {
	CallGet(string, []string) (map[string]interface{}, error)
	CallPost()
}


