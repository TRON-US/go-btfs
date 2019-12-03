package corehttp

const WebUIPath = "/btfs/QmPCfG86C3e4UgxFm8hWq8ccQVJDwntLfvCrs3yT3kdKsy"

// this is a list of all past webUI paths.
var WebUIPaths = []string{
	WebUIPath,
	//"/btfs/QmSgYypAeKmPi96PQCde2TL4vdr1ny2FugEXtuS8Mue3Ev",
}

var WebUIOption = RedirectOption("webui", WebUIPath)
