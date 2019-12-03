package spin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cenkalti/backoff"
	"github.com/dustin/go-humanize"
	"github.com/gogo/protobuf/proto"
	"github.com/tron-us/go-btfs-common/protos/node"
	"google.golang.org/grpc"
	"runtime"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands"
	"github.com/ipfs/go-bitswap"
	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-crypto"
	"github.com/shirou/gopsutil/cpu"
	"github.com/tron-us/go-btfs-common/info"
	pb "github.com/tron-us/go-btfs-common/protos/status"
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
	info.NodeStorage
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

func (dc *dataCollection) update(node *core.IpfsNode) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rds := node.Repo.Datastore()
	b, err := rds.Get(commands.GetHostStorageKey(node.Identity.Pretty()))
	if err != nil && err != ds.ErrNotFound {
		dc.reportHealthAlert(fmt.Sprintf("cannot get selfKey: %s", err.Error()))
	}

	var ns info.NodeStorage
	if err == nil {
		err = json.Unmarshal(b, &ns)
		if err != nil {
			log.Warning(err.Error())
			dc.reportHealthAlert(fmt.Sprintf("cannot parse nodestorage config: %s", err.Error()))
		} else {
			dc.StoragePriceAsk = ns.StoragePriceAsk
			dc.BandwidthPriceAsk = ns.BandwidthPriceAsk
			dc.StorageTimeMin = ns.StorageTimeMin
			dc.BandwidthLimit = ns.BandwidthLimit
			dc.CollateralStake = ns.CollateralStake
		}
	}

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

func (dc *dataCollection) getGrpcConn() (*grpc.ClientConn, context.CancelFunc, error) {
	config, err := dc.node.Repo.Config()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %s", err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	conn, err := grpc.DialContext(ctx, config.Services.StatusServerDomain, grpc.WithInsecure(), grpc.WithDisableRetry())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to status server: %s", err.Error())
	}
	return conn, cancel, nil
}

func (dc *dataCollection) sendData(btfsNode *core.IpfsNode) {
	sm, err := dc.doPrepData(btfsNode)
	if err != nil {
		dc.reportHealthAlert(err.Error())
		return
	}

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = maxRetryTotal
	backoff.Retry(func() error {
		err := dc.doSendData(sm)
		if err != nil {
			log.Error("failed to send data to status server: ", err)
		}
		return err
	}, bo)
}

func (dc *dataCollection) doPrepData(btfsNode *core.IpfsNode) (*pb.SignedMetrics, error) {
	dc.update(btfsNode)
	payload, err := dc.getPayload(btfsNode)
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
	conn, cancel, err := dc.getGrpcConn()
	if err != nil {
		return err
	}
	defer cancel()
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	client := pb.NewStatusClient(conn)
	_, err = client.UpdateMetrics(ctx, sm)
	if err != nil {
		return err
	}
	return nil
}

func (dc *dataCollection) getPayload(btfsNode *core.IpfsNode) ([]byte, error) {
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
		config, _ := dc.node.Repo.Config()
		// check config for explicit consent to data collect
		// consent can be changed without reinitializing data collection
		if config.Experimental.Analytics {
			dc.sendData(node)
		}
	}
}

func (dc *dataCollection) reportHealthAlert(failurePoint string) {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = maxRetryTotal
	backoff.Retry(func() error {
		err := dc.doReportHealthAlert(failurePoint)
		if err != nil {
			log.Error("failed to report health alert to status server: ", err)
		}
		return err
	}, bo)
}

func (dc *dataCollection) doReportHealthAlert(failurePoint string) error {
	conn, cancel, err := dc.getGrpcConn()
	if err != nil {
		return err
	}
	defer cancel()
	defer conn.Close()

	n := new(pb.NodeHealth)
	n.BtfsVersion = dc.BTFSVersion
	n.FailurePoint = failurePoint
	n.NodeId = dc.NodeID
	now := time.Now().UTC()
	n.TimeCreated = now

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	client := pb.NewStatusClient(conn)
	_, err = client.CollectHealth(ctx, n)
	return err
}
