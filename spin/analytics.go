package spin

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"

	"github.com/TRON-US/go-btfs-config"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	pb "github.com/tron-us/go-btfs-common/protos/status"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/cenkalti/backoff"
	"github.com/dustin/go-humanize"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-bitswap"
	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-crypto"
	"github.com/shirou/gopsutil/cpu"
)

type programInfo struct {
	node        *core.IpfsNode
	startTime   time.Time //Time at which the Daemon was ready and analytics started
	NodeID      string    `json:"node_id"`
	HVal        string    `json:"h_val"`
	CPUInfo     string    `json:"cpu_info"`
	BTFSVersion string    `json:"btfs_version"`
	OSType      string    `json:"os_type"`
	ArchType    string    `json:"arch_type"`
}

type dataCollection struct {
	nodepb.Node_Settings
	programInfo
	UpTime      uint64  `json:"up_time"`         //Seconds
	StorageUsed uint64  `json:"storage_used"`    //Stored in Kilobytes
	MemUsed     uint64  `json:"memory_used"`     //Stored in Kilobytes
	CPUUsed     float64 `json:"cpu_used"`        //Overall CPU used
	Upload      uint64  `json:"upload"`          //Upload over last epoch, stored in Kilobytes
	Download    uint64  `json:"download"`        //Download over last epoch, stored in Kilobytes
	TotalUp     uint64  `json:"total_upload"`    //Total data up, Stored in Kilobytes
	TotalDown   uint64  `json:"total_download"`  //Total data down, Stored in Kilobytes
	BlocksUp    uint64  `json:"blocks_up"`       //Total num of blocks uploaded
	BlocksDown  uint64  `json:"blocks_down"`     //Total num of blocks downloaded
	NumPeers    uint64  `json:"peers_connected"` //Number of peers
}

//Server URL for data collection
var (
	log = logging.Logger("spin")
)

// other constants
const (
	kilobyte = 1024

	// HeartBeat is how often we send data to server, at the moment set to 15 Minutes
	heartBeat = 15 * time.Minute

	// Expotentially delayed retries will be capped at this total time
	maxRetryTotal = 10 * time.Minute
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

//Analytics starts the process to collect data and starts the GoRoutine for constant collection
func Analytics(node *core.IpfsNode, BTFSVersion, hValue string) {
	if node == nil {
		return
	}
	configuration, err := node.Repo.Config()
	if err != nil {
		return
	}

	dc := new(dataCollection)
	dc.node = node

	if configuration.Experimental.Analytics {
		infoStats, err := cpu.Info()
		if err == nil {
			dc.CPUInfo = infoStats[0].ModelName
		} else {
			log.Warning(err.Error())
		}

		dc.startTime = time.Now().UTC()
		if node.Identity == "" {
			return
		}
		dc.NodeID = node.Identity.Pretty()
		dc.HVal = hValue
		dc.BTFSVersion = BTFSVersion
		dc.OSType = runtime.GOOS
		dc.ArchType = runtime.GOARCH
	}

	go dc.collectionAgent(node)
}

// update gets the latest analytics and returns a list of errors for reporting if available
func (dc *dataCollection) update(node *core.IpfsNode) []error {
	var res []error

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rds := node.Repo.Datastore()
	b, err := rds.Get(storage.GetHostStorageKey(node.Identity.Pretty()))
	if err != nil && err != ds.ErrNotFound {
		res = append(res, fmt.Errorf("cannot get selfKey: %s", err.Error()))
	}

	var ns nodepb.Node_Settings
	if err == nil {
		err = json.Unmarshal(b, &ns)
		if err != nil {
			res = append(res, fmt.Errorf("cannot parse nodestorage config: %s", err.Error()))
		} else {
			dc.StoragePriceAsk = ns.StoragePriceAsk
			dc.BandwidthPriceAsk = ns.BandwidthPriceAsk
			dc.StorageTimeMin = ns.StorageTimeMin
			dc.BandwidthLimit = ns.BandwidthLimit
			dc.CollateralStake = ns.CollateralStake
		}
	}

	dc.UpTime = durationToSeconds(time.Since(dc.startTime))
	if cpus, err := cpu.Percent(0, false); err != nil {
		res = append(res, fmt.Errorf("failed to get uptime: %s", err.Error()))
	} else {
		dc.CPUUsed = cpus[0]
	}
	dc.MemUsed = m.HeapAlloc / kilobyte
	if storage, err := dc.node.Repo.GetStorageUsage(); err != nil {
		res = append(res, fmt.Errorf("failed to get storage usage: %s", err.Error()))
	} else {
		dc.StorageUsed = storage / kilobyte
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
		dc.Upload = valOrZero(st.DataSent-dc.TotalUp) / kilobyte
		dc.Download = valOrZero(st.DataReceived-dc.TotalDown) / kilobyte
		dc.TotalUp = st.DataSent / kilobyte
		dc.TotalDown = st.DataReceived / kilobyte
		dc.BlocksUp = st.BlocksSent
		dc.BlocksDown = st.BlocksReceived
		dc.NumPeers = uint64(len(st.Peers))
	}

	return res
}

func (dc *dataCollection) sendData(btfsNode *core.IpfsNode, config *config.Config) {
	sm, errs, err := dc.doPrepData(btfsNode)
	if errs != nil || err != nil {
		var sb strings.Builder
		errs := append(errs, err)
		for _, err := range errs {
			sb.WriteString(err.Error())
			sb.WriteRune('\n')
		}
		dc.reportHealthAlert(config, sb.String())
		// If complete prep failure we return
		if err != nil {
			return
		}
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = maxRetryTotal
	backoff.Retry(func() error {
		err := dc.doSendData(config, sm)
		if err != nil {
			log.Error("failed to send data to status server: ", err)
		} else {
			log.Debug("sent analytics to status server")
		}
		return err
	}, bo)
}

// doPrepData gathers the latest analytics and returns (signed object, list of reporting errors, failure)
func (dc *dataCollection) doPrepData(btfsNode *core.IpfsNode) (*pb.SignedMetrics, []error, error) {
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

func (dc *dataCollection) doSendData(config *config.Config, sm *pb.SignedMetrics) error {
	cb := cgrpc.StatusClient(config.Services.StatusServerDomain)
	return cb.WithContext(context.Background(), func(ctx context.Context, client pb.StatusServiceClient) error {
		_, err := client.UpdateMetrics(ctx, sm)
		return err
	})
}

func (dc *dataCollection) getPayload(btfsNode *core.IpfsNode) ([]byte, error) {
	nd := new(nodepb.Node)
	now := time.Now().UTC()
	nd.TimeCreated = now
	nd.NodeId = dc.NodeID
	nd.BtfsVersion = dc.BTFSVersion
	nd.ArchType = dc.ArchType
	nd.BlocksDown = dc.BlocksDown
	nd.BlocksUp = dc.BlocksUp
	nd.CpuInfo = dc.CPUInfo
	nd.CpuUsed = dc.CPUUsed
	nd.Download = dc.Download
	nd.MemoryUsed = dc.MemUsed
	nd.OsType = dc.OSType
	nd.PeersConnected = dc.NumPeers
	nd.StorageUsed = dc.StorageUsed
	nd.UpTime = dc.UpTime
	nd.Upload = dc.Upload
	nd.TotalUpload = dc.TotalUp
	nd.TotalDownload = dc.TotalDown
	if config, err := dc.node.Repo.Config(); err == nil {
		if storageMax, err := humanize.ParseBytes(config.Datastore.StorageMax); err == nil {
			nd.StorageVolumeCap = storageMax
		}
	}

	nd.StoragePriceAsk = dc.StoragePriceAsk
	nd.BandwidthPriceAsk = dc.BandwidthPriceAsk
	nd.StorageTimeMin = dc.StorageTimeMin
	nd.BandwidthLimit = dc.BandwidthLimit
	nd.CollateralStake = dc.CollateralStake
	bytes, err := proto.Marshal(nd)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (dc *dataCollection) collectionAgent(node *core.IpfsNode) {
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
		if config.Experimental.Analytics {
			dc.sendData(node, config)
		}
	}
}

func (dc *dataCollection) reportHealthAlert(config *config.Config, failurePoint string) {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = maxRetryTotal
	backoff.Retry(func() error {
		err := dc.doReportHealthAlert(config, failurePoint)
		if err != nil {
			log.Error("failed to report health alert to status server: ", err)
		}
		return err
	}, bo)
}

func (dc *dataCollection) doReportHealthAlert(config *config.Config, failurePoint string) error {
	n := new(pb.NodeHealth)
	n.BtfsVersion = dc.BTFSVersion
	n.FailurePoint = failurePoint
	n.NodeId = dc.NodeID
	n.TimeCreated = time.Now()

	cb := cgrpc.StatusClient(config.Services.StatusServerDomain)
	return cb.WithContext(context.Background(), func(ctx context.Context, client pb.StatusServiceClient) error {
		_, err := client.CollectHealth(ctx, n)
		return err
	})
}
