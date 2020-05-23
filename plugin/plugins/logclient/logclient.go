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

type InitCollectParams struct {
	Cid       string
	HValue    string
	InputChan chan []logclient.Entry
}

// Initalize a log collecter and set it as the global tracer in opentracing api
func (lcp *logclientPlugin) InitCollect(params ...interface{}) (*logclient.LogClient, error) {
	p := findParams(params)
	if p.Cid == "" || p.HValue == "" || p.InputChan == nil {
		return nil, fmt.Errorf("zero parameter value: cid [%s], input channel [%v]", p.Cid, p.InputChan)
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
	labels["instance"] = model.LabelValue(p.Cid)
	labels["hValue"] = model.LabelValue(p.HValue)
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

	logc, err := logclient.NewLogClient(conf, p.InputChan)
	if err != nil {
		return nil, errors.Errorf("error: failed to create LogClient: %s\n", err)
	}

	return logc, nil
}

func findParams(val interface{}) *InitCollectParams {
	switch val := val.(type) {
	case *InitCollectParams:
		return val
	case []interface{}:
		return findParams(val[0])
	}
	return nil
}
