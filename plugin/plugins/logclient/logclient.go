package logclient

import (
	"fmt"
	"github.com/prometheus/common/model"
	"os"
	"time"

	logclient "github.com/TRON-US/go-btfs-collect-client/logclient"
	"github.com/TRON-US/go-btfs/plugin"
	"github.com/pkg/errors"
)

// Plugins is exported list of plugins that will be loaded
var Plugins = []plugin.Plugin{
	&logclientPlugin{},
}

type logclientPlugin struct {
	inputChannel chan []logclient.Entry
}

var _ plugin.PluginCollect = (*logclientPlugin)(nil)

func (*logclientPlugin) Name() string {
	return "logclient"
}

func (*logclientPlugin) Version() string {
	return "0.0.1"
}

func (*logclientPlugin) Init() error {
	return nil
}

const (
	DEFAULT_PUSH_URL        = "http://localhost:3100/loki/api/v1/push"
	DEFAULT_COLLECTION_DEST = "loki"
)

// Initalize a log collecter and set it as the global tracer in opentracing api
func (lcp *logclientPlugin) InitCollect(params ...interface{}) (*logclient.LogClient, error) {
	cid, inputChan := findParams(params)
	if cid == "" || inputChan == nil {
		return nil, fmt.Errorf("zero parameter value: cid [%s], input channel [%v]", cid, inputChan)
	}
	// init configuration
	url := os.Getenv("LOG_COLLECTOR_URL") // TODO: add a domain to .btfs/config file
	if url == "" {
		url = DEFAULT_PUSH_URL
	}
	dest := os.Getenv("LOG_COLLECTION_DEST") // TODO: add to .btfs/config file
	if dest == "" {
		dest = DEFAULT_COLLECTION_DEST
	}
	labels := make(model.LabelSet)
	labels["job"] = "btfsnode"
	labels["instance"] = model.LabelValue(cid)
	conf := &logclient.Configuration{
		Labels:             labels.String(),
		URL:                url,
		Destination:        dest,
		BatchWaitDuration:  10 * time.Second,
		BatchCapacity:      5,
		NetworkSendTimeout: 150 * time.Second, // TODO: change back to 15. This is just for DEBUGGING
		NetworkSendRetries: logclient.DEFAULT_NUM_OF_RETRIES,
	}

	// Open operators
	// if a channel is given, pass it to NewLogClient()

	logc, err := logclient.NewLogClient(conf, inputChan)
	if err != nil {
		return nil, errors.Errorf("error: failed to create LogClient: %s\n", err)
	}

	return logc, nil
}

func findParams(val interface{}) (string, chan []logclient.Entry) {
	switch val := val.(type) {
	case chan []logclient.Entry:
		return "", val
	case string:
		return val, nil
	case []interface{}:
		var cid string
		var channel chan []logclient.Entry
		for i := 0; i < len(val); i++ {
			ci, ch := findParams(val[i])
			if ci != "" && ch != nil {
				return ci, ch
			} else if ci != "" {
				cid = ci
			} else if ch != nil {
				channel = ch
			}
		}
		return cid, channel
	}
	return "", nil
}
