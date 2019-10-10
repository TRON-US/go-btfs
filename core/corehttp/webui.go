package corehttp

const WebUIPath = "/btfs/QmWHXwomLwoGo4DDAjTNyjno11k2Lzmd11RRFdukrJ51QX"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
	//"/btfs/QmSgYypAeKmPi96PQCde2TL4vdr1ny2FugEXtuS8Mue3Ev",
}

var WebUIOption = RedirectOption("webui", WebUIPath)
