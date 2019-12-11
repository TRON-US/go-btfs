package analytics

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/tron-us/go-btfs-common/protos/node"
	pb "github.com/tron-us/go-btfs-common/protos/status"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/cenkalti/backoff"
	"github.com/dustin/go-humanize"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-bitswap"
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

// other constants
const (
	kilobyte = 1024

	// HeartBeat is how often we send data to server, at the moment set to 15 Minutes
	heartBeat = 15 * time.Minute

	// Expotentially delayed retries will be capped at this total time
	maxRetryTotal = 10 * time.Minute

	dialTimeout = time.Minute

	callTimeout = 5 * time.Second
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

var log = logging.Logger("analytics/analytics")

//Initialize starts the process to collect data and starts the GoRoutine for constant collection
func Initialize(n *core.IpfsNode, BTFSVersion, hValue string) {
	if n == nil {
		return
	}

	configuration, err := n.Repo.Config()
	if err != nil {
		return
	}

	dc := new(dataCollection)
	dc.node = n

	if configuration.Experimental.Analytics {
		infoStats, err := cpu.Info()
		if err == nil {
			dc.CPUInfo = infoStats[0].ModelName
		} else {
			log.Warning(err.Error())
		}

		dc.startTime = time.Now()
		if n.Identity == "" {
			return
		}
		dc.NodeID = n.Identity.Pretty()
		dc.HVal = hValue
		dc.BTFSVersion = BTFSVersion
		dc.OSType = runtime.GOOS
		dc.ArchType = runtime.GOARCH
	}

	go dc.collectionAgent()
}

func (dc *dataCollection) update() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	dc.UpTime = durationToSeconds(time.Since(dc.startTime))
	cpus, e := cpu.Percent(0, false)
	if e == nil {
		dc.CPUUsed = cpus[0]
	}
	dc.MemUsed = m.HeapAlloc / kilobyte
	storage, e := dc.node.Repo.GetStorageUsage()
	if e == nil {
		dc.StorageUsed = storage / kilobyte
	}

	bs, ok := dc.node.Exchange.(*bitswap.Bitswap)
	if !ok {
		dc.reportHealthAlert("failed to perform dc.node.Exchange.(*bitswap.Bitswap) type assertion")
		return
	}

	st, err := bs.Stat()
	if err != nil {
		dc.reportHealthAlert(fmt.Sprintf("failed to perform bs.Stat() call: %s", err.Error()))
		return
	}

	dc.Upload = valOrZero(st.DataSent-dc.TotalUp) / kilobyte
	dc.Download = valOrZero(st.DataReceived-dc.TotalDown) / kilobyte
	dc.TotalUp = st.DataSent / kilobyte
	dc.TotalDown = st.DataReceived / kilobyte
	dc.BlocksUp = st.BlocksSent
	dc.BlocksDown = st.BlocksReceived

	dc.NumPeers = uint64(len(st.Peers))
}

func (dc *dataCollection) sendData() {
	sm, err := dc.doPrepData()
	if err != nil {
		log.Error("failed to prepare data for status server: ", err)
		dc.reportHealthAlert(err.Error())
		return
	}

	// Retry for connections
	retry(func() error {
		err := dc.doSendData(sm)
		if err != nil {
			log.Error("failed to send data to status server: ", err)
		}
		return err
	})
}

func (dc *dataCollection) doPrepData() (*pb.SignedMetrics, error) {
	dc.update()
	payload, err := dc.getPayload()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal dataCollection object to a byte array: %s", err.Error())
	}
	if dc.node.PrivateKey == nil {
		return nil, fmt.Errorf("node's private key is null")
	}

	signature, err := dc.node.PrivateKey.Sign(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to sign raw data with node private key: %s", err.Error())
	}

	publicKey, err := ic.MarshalPublicKey(dc.node.PrivateKey.GetPublic())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal node public key: %s", err.Error())
	}

	sm := new(pb.SignedMetrics)
	sm.Payload = payload
	sm.Signature = signature
	sm.PublicKey = publicKey
	return sm, nil
}

func (dc *dataCollection) doSendData(sm *pb.SignedMetrics) error {
	config, err := dc.node.Repo.Config()
	if err != nil {
		return err
	}
	cb := cgrpc.StatusClient(config.Services.StatusServerDomain)
	return cb.WithContext(context.Background(), func(ctx context.Context, client pb.StatusServiceClient) error {
		_, err := client.UpdateMetrics(ctx, sm)
		return err
	})
}

func (dc *dataCollection) getPayload() ([]byte, error) {
	nd := new(node.Node)
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
	nd.HVal = dc.HVal
	if config, err := dc.node.Repo.Config(); err == nil {
		if storageMax, err := humanize.ParseBytes(config.Datastore.StorageMax); err == nil {
			nd.StorageVolumeCap = storageMax
		}
	}
	bytes, err := proto.Marshal(nd)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func (dc *dataCollection) collectionAgent() {
	tick := time.NewTicker(heartBeat)

	defer tick.Stop()

	config, _ := dc.node.Repo.Config()
	if config.Experimental.Analytics {
		dc.sendData()
	}
	// make the configuration available in the for loop
	for range tick.C {
		config, _ := dc.node.Repo.Config()
		// check config for explicit consent to data collect
		// consent can be changed without reinitializing data collection
		if config.Experimental.Analytics {
			dc.sendData()
		}
	}
}

func retry(f func() error) {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = maxRetryTotal
	backoff.Retry(f, bo)
}

func (dc *dataCollection) reportHealthAlert(failurePoint string) {
	retry(func() error {
		return dc.doReportHealthAlert(failurePoint)
	})
}

func (dc *dataCollection) doReportHealthAlert(failurePoint string) error {
	n := new(pb.NodeHealth)
	n.BtfsVersion = dc.BTFSVersion
	n.FailurePoint = failurePoint
	n.NodeId = dc.NodeID
	now := time.Now().UTC()
	n.TimeCreated = now

	config, err := dc.node.Repo.Config()
	if err != nil {
		return err
	}
	cb := cgrpc.StatusClient(config.Services.StatusServerDomain)
	return cb.WithContext(context.Background(), func(ctx context.Context, client pb.StatusServiceClient) error {
		_, err := client.CollectHealth(ctx, n)
		return err
	})
}
