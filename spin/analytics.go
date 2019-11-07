package spin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands"
	"github.com/ipfs/go-bitswap"
	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	ic "github.com/libp2p/go-libp2p-crypto"
	"github.com/tron-us/go-btfs-common/info"

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

type dataBag struct {
	PublicKey []byte `json:"public_key"`
	Signature []byte `json:"signature"`
	Payload   []byte `json:"payload"`
}

type healthData struct {
	NodeId       string `json:"node_id"`
	BTFSVersion  string `json:"btfs_version"`
	FailurePoint string `json:"failure_point"`
}

//Server URL for data collection
var (
	log                = logging.Logger("spin")
	statusServerDomain string
)

const (
	routeMetrics = "/metrics"
	routeHealth  = "/health"
)

// other constants
const (
	kilobyte = 1024

	//HeartBeat is how often we send data to server, at the moment set to 15 Minutes
	heartBeat = 15 * time.Minute
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

	statusServerDomain = configuration.StatusServerDomain

	dc := new(dataCollection)
	dc.node = node

	if configuration.Experimental.Analytics {
		infoStats, err := cpu.Info()
		if err == nil {
			dc.CPUInfo = infoStats[0].ModelName
		} else {
			log.Warning(err.Error())
		}

		dc.startTime = time.Now()
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

func (dc *dataCollection) sendData(node *core.IpfsNode) {
	dc.update(node)
	dcMarshal, err := json.Marshal(dc)
	if err != nil {
		dc.reportHealthAlert(fmt.Sprintf("failed to marshal dataCollection object to a byte array: %s", err.Error()))
		return
	}
	if dc.node.PrivateKey == nil {
		dc.reportHealthAlert("node's private key is null")
		return
	}
	signature, err := dc.node.PrivateKey.Sign(dcMarshal)
	if err != nil {
		dc.reportHealthAlert(fmt.Sprintf("failed to sign raw data with node private key: %s", err.Error()))
		return
	}
	publicKey, err := ic.MarshalPublicKey(dc.node.PrivateKey.GetPublic())
	if err != nil {
		dc.reportHealthAlert(fmt.Sprintf("failed to marshal node public key: %s", err.Error()))
		return
	}
	dataBagInstance := new(dataBag)
	dataBagInstance.PublicKey = publicKey
	dataBagInstance.Signature = signature
	dataBagInstance.Payload = dcMarshal
	dataBagMarshaled, err := json.Marshal(dataBagInstance)
	if err != nil {
		dc.reportHealthAlert(fmt.Sprintf("failed to marshal databag: %s", err.Error()))
		return
	}

	// btfs node reports to status server by making HTTP request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", statusServerDomain, routeMetrics), bytes.NewReader(dataBagMarshaled))
	if err != nil {
		dc.reportHealthAlert(fmt.Sprintf("failed to make new http request: %s", err.Error()))
		return
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		dc.reportHealthAlert(fmt.Sprintf("failed to perform http.DefaultClient.Do(): %s", err.Error()))
		return
	}
	defer res.Body.Close()
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
	hd := new(healthData)
	hd.NodeId = dc.NodeID
	hd.BTFSVersion = dc.BTFSVersion
	hd.FailurePoint = failurePoint
	hdMarshaled, err := json.Marshal(hd)
	if err != nil {
		log.Warning(err.Error())
		return
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", statusServerDomain, routeHealth), bytes.NewReader(hdMarshaled))
	if err != nil {
		log.Warning(err.Error())
		return
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warning(err.Error())
		return
	}
	defer res.Body.Close()
}
