package analytics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/shirou/gopsutil/cpu"
)

type programInfo struct {
	node        *core.IpfsNode
	startTime   time.Time
	NodeID      string `json:"node_id"`
	CPUInfo     string `json:"cpu_info"`
	BTFSVersion string `json:"btfs_version"`
	OSType      string `json:"os_type"`
	ArchType    string `json:"arch_type"`
}

type dataCollection struct {
	programInfo
	UpTime      uint64  `json:"up_time"`      //Seconds
	StorageUsed uint64  `json:"storage_used"` //
	MemUsed     uint64  `json:"memory_used"`  //
	CPUUsed     float64 `json:"cpu_used"`
}

//HeartBeat is how often we send data to server, at the moment set to 15 Minutes
var heartBeat = 15 * time.Minute

//Server URL for data collection
var dataServeURL = "http://18.220.204.165:8080/metrics"

func durationToSeconds(duration time.Duration) uint64 {
	return uint64(duration.Nanoseconds() / int64(time.Second/time.Nanosecond))
}

//Initialize starts the process to collect data and starts the GoRoutine for constant collection
func Initialize(n *core.IpfsNode, BTFSVersion string) {
	dc := new(dataCollection)
	infoStats, _ := cpu.Info()

	dc.node = n
	dc.startTime = time.Now()
	dc.NodeID = n.Identity.Pretty()
	dc.CPUInfo = infoStats[0].ModelName
	dc.BTFSVersion = BTFSVersion
	dc.OSType = runtime.GOOS
	dc.ArchType = runtime.GOARCH

	go dc.collectionAgent()
}

func (dc *dataCollection) update() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	cpus, _ := cpu.Percent(0, false)
	storage, _ := dc.node.Repo.GetStorageUsage()

	dc.UpTime = durationToSeconds(time.Since(dc.startTime))
	dc.CPUUsed = cpus[0]
	dc.MemUsed = m.HeapAlloc / 1024
	dc.StorageUsed = storage / 1024
}

func (dc *dataCollection) printData() {
	temp, _ := json.Marshal(dc)
	fmt.Print(string(temp))
}

func (dc *dataCollection) sendData() {
	temp, _ := json.Marshal(dc)

	req, err := http.NewRequest("POST", dataServeURL, bytes.NewReader(temp))
	req.Header.Add("Content-Type", "application/json")
	if err != nil {
		return
	}

	res, err := http.DefaultClient.Do(req)
	defer res.Body.Close()

	if err != nil {
		return
	}
}

func (dc *dataCollection) collectionAgent() {
	tick := time.NewTicker(heartBeat)
	defer tick.Stop()
	for range tick.C {
		dc.update()
		dc.sendData()
	}
}
