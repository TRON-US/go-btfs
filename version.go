package btfs

// CurrentCommit is the current git commit, this is set as a ldflag in the Makefile
var CurrentCommit string

// CurrentVersionNumber is the current application's version literal
const CurrentVersionNumber = "1.2.0"

const ApiVersion = "/go-btfs/" + CurrentVersionNumber + "/"
