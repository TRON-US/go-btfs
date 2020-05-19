package collect

import (
	"fmt"
)

const (
	COLLECT_LOG = iota + 1
	COLLECT_METRICS
	COLLECT_TRACES
)

var collectClients map[int]interface{}

func init() {
	collectClients = make(map[int]interface{})
}

func AddCollectClient(collectType string, collectClient interface{}) error {
	switch collectType {
	case "logclient":
		collectClients[COLLECT_LOG] = collectClient
	case "metricsclient":
		collectClients[COLLECT_METRICS] = collectClient
	case "traceclient":
		collectClients[COLLECT_TRACES] = collectClient
	default:
		return fmt.Errorf("unexpected collector type [%s], possible program error", collectType)
	}
	return nil
}

func GetCollectClient(collectType int) (interface{}, error) {
	if collectType < COLLECT_LOG || collectType > COLLECT_TRACES {
		return nil, fmt.Errorf("unexpected collector type [%s], possible program error", collectType)
	}
	return collectClients[collectType], nil
}
