package spin

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"

	config "github.com/TRON-US/go-btfs-config"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	pb "github.com/tron-us/go-btfs-common/protos/status"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v3"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-bitswap"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-crypto"
	"github.com/shirou/gopsutil/cpu"
)

type dcWrap struct {
	node   *core.IpfsNode
	pn     *nodepb.Node
	config *config.Config
}

//Server URL for data collection
var (
	log = logging.Logger("spin")
)

// other constants
const (
	// HeartBeat is how often we send data to server, at the moment set to 15 Minutes
	heartBeat = 15 * time.Minute

	// Expotentially delayed retries will be capped at this total time
	maxRetryTotal = 10 * time.Minute

	// Timeout to retrieve settings/config
	updateTimeout = 30 * time.Second
)

//Go doesn't have a built in Max function? simple function to not have negatives values
func valOrZero(x uint64) uint64 {
	if x < 0 {
		return 0
	}

	return x
}

func durationToSeconds(duration time.Duration) uint64 {
	return uint64(duration.Nanoseconds() / int64(time.Second/time.Nanosecond))
}

func isAnalyticsEnabled(cfg *config.Config) bool {
	return cfg.Experimental.StorageHostEnabled || cfg.Experimental.Analytics
}

// Analytics starts the process to collect data and starts the GoRoutine for constant collection
func Analytics(cfgRoot string, node *core.IpfsNode, BTFSVersion, hValue string) {
	if node == nil {
		return
	}
	configuration, err := node.Repo.Config()
	if err != nil {
		return
	}

	dc := new(dcWrap)
	dc.node = node
	dc.pn = new(nodepb.Node)
	dc.config = configuration

	if isAnalyticsEnabled(dc.config) {
		if dc.config.Experimental.Analytics != dc.config.Experimental.StorageHostEnabled {
			fmt.Println("Experimental.Analytics is overridden by Experimental.StorageHostEnabled")
		}
		infoStats, err := cpu.Info()
		if err == nil {
			dc.pn.CpuInfo = infoStats[0].ModelName
		} else {
			log.Warning(err.Error())
		}

		dc.pn.TimeCreated = time.Now()
		if node.Identity == "" {
			return
		}
		dc.pn.NodeId = node.Identity.Pretty()
		dc.pn.HVal = hValue
		dc.pn.BtfsVersion = BTFSVersion
		dc.pn.OsType = runtime.GOOS
		dc.pn.ArchType = runtime.GOARCH
		if storageMax, err := helper.CheckAndValidateHostStorageMax(cfgRoot, node.Repo, nil, true); err == nil {
			dc.pn.StorageVolumeCap = storageMax
		} else {
			log.Warning(err.Error())
		}

		dc.pn.Analytics = dc.config.Experimental.Analytics
		dc.pn.FilestoreEnabled = dc.config.Experimental.FilestoreEnabled
		dc.pn.HostsSyncEnabled = dc.config.Experimental.HostsSyncEnabled
		dc.pn.HostsSyncMode = dc.config.Experimental.HostsSyncMode
		dc.pn.Libp2PStreamMounting = dc.config.Experimental.Analytics
		dc.pn.P2PHttpProxy = dc.config.Experimental.P2pHttpProxy
		dc.pn.PreferTls = dc.config.Experimental.PreferTLS
		dc.pn.Quic = dc.config.Experimental.QUIC
		dc.pn.RemoveOnUnpin = dc.config.Experimental.RemoveOnUnpin
		dc.pn.ShardingEnabled = dc.config.Experimental.ShardingEnabled
		dc.pn.StorageClientEnabled = dc.config.Experimental.StorageClientEnabled
		dc.pn.StorageHostEnabled = dc.config.Experimental.StorageHostEnabled
		dc.pn.StrategicProviding = dc.config.Experimental.StrategicProviding
		dc.pn.UrlStoreEnabled = dc.config.Experimental.UrlstoreEnabled
	}

	go dc.collectionAgent(node)
}

// update gets the latest analytics and returns a list of errors for reporting if available
func (dc *dcWrap) update(node *core.IpfsNode) []error {
	var res []error

	var (
		m  runtime.MemStats
		ns *nodepb.Node_Settings
	)
	runtime.ReadMemStats(&m)
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	ns, err := helper.GetHostStorageConfig(ctx, node)
	if err != nil {
		res = append(res, fmt.Errorf("failed to get node storage config: %s", err.Error()))
	} else {
		dc.pn.StoragePriceAsk = ns.StoragePriceAsk
		dc.pn.BandwidthPriceAsk = ns.BandwidthPriceAsk
		dc.pn.StorageTimeMin = ns.StorageTimeMin
		dc.pn.BandwidthLimit = ns.BandwidthLimit
		dc.pn.CollateralStake = ns.CollateralStake
	}

	dc.pn.UpTime = durationToSeconds(time.Since(dc.pn.TimeCreated))
	if cpus, err := cpu.Percent(0, false); err != nil {
		res = append(res, fmt.Errorf("failed to get uptime: %s", err.Error()))
	} else {
		dc.pn.CpuUsed = cpus[0]
	}
	dc.pn.MemoryUsed = m.HeapAlloc / uint64(units.KiB)
	if storage, err := dc.node.Repo.GetStorageUsage(); err != nil {
		res = append(res, fmt.Errorf("failed to get storage usage: %s", err.Error()))
	} else {
		dc.pn.StorageUsed = storage / uint64(units.KiB)
	}

	bs, ok := dc.node.Exchange.(*bitswap.Bitswap)
	if !ok {
		res = append(res, fmt.Errorf("failed to perform dc.node.Exchange.(*bitswap.Bitswap) type assertion"))
		return res
	}

	st, err := bs.Stat()
	if err != nil {
		res = append(res, fmt.Errorf("failed to perform bs.Stat() call: %s", err.Error()))
	} else {
		dc.pn.Upload = valOrZero(st.DataSent-dc.pn.TotalUpload) / uint64(units.KiB)
		dc.pn.Download = valOrZero(st.DataReceived-dc.pn.TotalDownload) / uint64(units.KiB)
		dc.pn.TotalUpload = st.DataSent / uint64(units.KiB)
		dc.pn.TotalDownload = st.DataReceived / uint64(units.KiB)
		dc.pn.BlocksUp = st.BlocksSent
		dc.pn.BlocksDown = st.BlocksReceived
		dc.pn.PeersConnected = uint64(len(st.Peers))
	}

	return res
}

func (dc *dcWrap) sendData(node *core.IpfsNode, config *config.Config) {
	sm, errs, err := dc.doPrepData(node)
	if errs == nil {
		errs = make([]error, 0)
	}
	var sb strings.Builder
	if err != nil {
		errs = append(errs, err)
	}
	for _, err := range errs {
		sb.WriteString(err.Error())
		sb.WriteRune('\n')
	}
	dc.reportHealthAlert(node.Context(), config, sb.String())
	// If complete prep failure we return
	if err != nil {
		return
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = maxRetryTotal
	backoff.Retry(func() error {
		err := dc.doSendData(node.Context(), config, sm)
		if err != nil {
			log.Error("failed to send data to status server: ", err)
		} else {
			log.Debug("sent analytics to status server")
		}
		return err
	}, bo)
}

// doPrepData gathers the latest analytics and returns (signed object, list of reporting errors, failure)
func (dc *dcWrap) doPrepData(btfsNode *core.IpfsNode) (*pb.SignedMetrics, []error, error) {
	errs := dc.update(btfsNode)
	payload, err := dc.getPayload(btfsNode)
	if err != nil {
		return nil, errs, fmt.Errorf("failed to marshal dataCollection object to a byte array: %s", err.Error())
	}
	if dc.node.PrivateKey == nil {
		return nil, errs, fmt.Errorf("node's private key is null")
	}

	signature, err := dc.node.PrivateKey.Sign(payload)
	if err != nil {
		return nil, errs, fmt.Errorf("failed to sign raw data with node private key: %s", err.Error())
	}

	publicKey, err := ic.MarshalPublicKey(dc.node.PrivateKey.GetPublic())
	if err != nil {
		return nil, errs, fmt.Errorf("failed to marshal node public key: %s", err.Error())
	}

	sm := new(pb.SignedMetrics)
	sm.Payload = payload
	sm.Signature = signature
	sm.PublicKey = publicKey
	return sm, errs, nil
}

func (dc *dcWrap) doSendData(ctx context.Context, config *config.Config, sm *pb.SignedMetrics) error {
	cb := cgrpc.StatusClient(config.Services.StatusServerDomain)
	return cb.WithContext(ctx, func(ctx context.Context, client pb.StatusServiceClient) error {
		_, err := client.UpdateMetrics(ctx, sm)
		return err
	})
}

func (dc *dcWrap) getPayload(btfsNode *core.IpfsNode) ([]byte, error) {
	bytes, err := proto.Marshal(dc.pn)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (dc *dcWrap) collectionAgent(node *core.IpfsNode) {
	tick := time.NewTicker(heartBeat)
	defer tick.Stop()
	// Force tick on immediate start
	// make the configuration available in the for loop
	for ; true; <-tick.C {
		config, err := dc.node.Repo.Config()
		if err != nil {
			continue
		}
		// check config for explicit consent to data collect
		// consent can be changed without reinitializing data collection
		if isAnalyticsEnabled(config) {
			dc.sendData(node, config)
		}
	}
}

func (dc *dcWrap) reportHealthAlert(ctx context.Context, config *config.Config, failurePoint string) {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = maxRetryTotal
	backoff.Retry(func() error {
		err := dc.doReportHealthAlert(ctx, config, failurePoint)
		if err != nil {
			log.Error("failed to report health alert to status server: ", err)
		}
		return err
	}, bo)
}

func (dc *dcWrap) doReportHealthAlert(ctx context.Context, config *config.Config, failurePoint string) error {
	n := new(pb.NodeHealth)
	n.BtfsVersion = dc.pn.BtfsVersion
	n.FailurePoint = failurePoint
	n.NodeId = dc.pn.NodeId
	n.TimeCreated = time.Now()

	cb := cgrpc.StatusClient(config.Services.StatusServerDomain)
	return cb.WithContext(ctx, func(ctx context.Context, client pb.StatusServiceClient) error {
		_, err := client.CollectHealth(ctx, n)
		return err
	})
}
