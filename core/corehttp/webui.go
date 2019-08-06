package corehttp

const WebUIPath = "/btfs/QmQNSC916rNr9wfDhFx768atzA6UfT2z5XSiZSU9H71Scc"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
	//"/btfs/QmSgYypAeKmPi96PQCde2TL4vdr1ny2FugEXtuS8Mue3Ev",
}

var WebUIOption = RedirectOption("webui", WebUIPath)
