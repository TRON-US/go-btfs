package corehttp

const WebUIPath = "/btfs/QmPCfG86C3e4UgxFm8hWq8ccQVJDwntLfvCrs3yT3kdKsy"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
}

var WebUIOption = RedirectOption("webui", WebUIPath)
