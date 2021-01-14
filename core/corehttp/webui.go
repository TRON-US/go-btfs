package corehttp

// TODO: move to BTNS
const WebUIPath = "/btfs/QmWV91WcJvnHLYKUWDm9fWufA76oLGbtd8AaBsB7wYH2ZJ"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
}

var WebUIOption = RedirectOption("webui", WebUIPath)
