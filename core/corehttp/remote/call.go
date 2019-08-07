package remote

type Call interface {
	CallGet(string, []string) ([]byte, error)
	CallPost()
}


