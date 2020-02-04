package corehttp

const WebUIPath = "/btfs/QmaeUyKBdYUSbDqCN6guuCCVrdY5iMcGeKizzSEiFrzEZS"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
}

var WebUIOption = RedirectOption("webui", WebUIPath)
