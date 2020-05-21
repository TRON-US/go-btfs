package plugin

import (
	"github.com/TRON-US/go-btfs-collect-client/logclient"
)

// PluginCollect is an interface that can be implemented to add a collector
type PluginCollect interface {
	Plugin

	InitCollect(params ...interface{}) (*logclient.LogClient, error)
}
