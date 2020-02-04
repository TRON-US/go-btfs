package corehttp

const WebUIPath = "/btfs/QmbsPYrDJRT1sjenSpRADNM4ZHdwjgsiZnGp7zoaFpNWWo"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
}

var WebUIOption = RedirectOption("webui", WebUIPath)
