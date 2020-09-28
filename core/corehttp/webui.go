package corehttp

// TODO: move to BTNS
const WebUIPath = "/btfs/QmVc2xjpt1xbBDu98QdDnPNS7nwyAgMvPbNpegJ9YyeLTd"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
}

var WebUIOption = RedirectOption("webui", WebUIPath)
