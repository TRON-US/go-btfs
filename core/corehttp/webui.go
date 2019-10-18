package corehttp

const WebUIPath = "/btfs/QmTfM7Rz58Ejwq8Yg3jEEhtLc3BBxucpATiq4pRCy6TK4J"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
	//"/btfs/QmSgYypAeKmPi96PQCde2TL4vdr1ny2FugEXtuS8Mue3Ev",
}

var WebUIOption = RedirectOption("webui", WebUIPath)
