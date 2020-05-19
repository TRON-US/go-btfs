package logging

import (
	"github.com/TRON-US/go-btfs-collect-client/logclient"
	u "github.com/ipfs/go-ipfs-util"
	gologging "github.com/ipfs/go-log"
)

var logCollectEnabled bool

func init() {
	logCollectEnabled = u.GetenvBool("LOG_COLLECT") // TODO: add to .btfs/config file
}

func Logger(system string) *gologging.ZapEventLogger {
	if logCollectEnabled {
		return gologging.LoggerWithOutChannel(system, logclient.LogOutputChan)
	} else {
		return gologging.Logger(system)
	}
}

func LogCollectEnabled() bool {
	return logCollectEnabled
}
