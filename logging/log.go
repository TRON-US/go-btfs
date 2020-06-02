package logging

import (
	"github.com/TRON-US/go-btfs-collect-client/logclient"
	u "github.com/ipfs/go-ipfs-util"
	gologging "github.com/ipfs/go-log"
)

func Logger(system string) *gologging.ZapEventLogger {
	logCollect := u.GetenvBool("LOG_COLLECT") // TODO: add to .btfs/config file
	if logCollect {
		return gologging.LoggerWithOutChannel(system, logclient.LogOutputChan)
	} else {
		return gologging.Logger(system)
	}
}
