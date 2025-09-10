package metrics

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/hashicorp/memberlist"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"

	"gnomatix/dreamfs/v2/pkg/network"
	"gnomatix/dreamfs/v2/pkg/utils"
)

type PeerMetrics struct {
	Host         string  `json:"host"`
	IP           string  `json:"ip"`
	CPU          float64 `json:"cpu"`
	MemoryGB     float64 `json:"memory"`
	IOReadMB     float64 `json:"io_read"`
	IOWriteMB    float64 `json:"io_write"`
	FilesIndexed int     `json:"files_indexed"`
}

var peerMetrics = make(map[string]PeerMetrics)
var peerMetricsMutex sync.Mutex

func CollectLocalMetrics(filesIndexed int) PeerMetrics {
	cpuPercent, _ := cpu.Percent(0, false)
	memStats, _ := mem.VirtualMemory()
	ioStats, _ := disk.IOCounters()

	var ioRead, ioWrite float64
	for _, io := range ioStats {
		ioRead += float64(io.ReadBytes) / (1024 * 1024)   // MB
		ioWrite += float64(io.WriteBytes) / (1024 * 1024) // MB
	}

	host, _ := os.Hostname()
	ip := utils.GetLocalIP() // Use utils.GetLocalIP()

	return PeerMetrics{
		Host:         host,
		IP:           ip,
		CPU:          cpuPercent[0],
		MemoryGB:     float64(memStats.Used) / (1024 * 1024 * 1024),
		IOReadMB:     ioRead,
		IOWriteMB:    ioWrite,
		FilesIndexed: filesIndexed,
	}
}

// BroadcastPeerMetrics now takes a *network.SwarmDelegate
func BroadcastPeerMetrics(d *network.SwarmDelegate, filesIndexed int) {
	metrics := CollectLocalMetrics(filesIndexed)
	data, _ := json.Marshal(metrics)

	peerMetricsMutex.Lock()
	defer peerMetricsMutex.Unlock()
	peerMetrics[metrics.IP] = metrics

	// Use the broadcasts queue from the SwarmDelegate
	d.Broadcasts.QueueBroadcast(&network.PeerMetaBroadcast{msg: data})
}

type PeerMetaBroadcast struct {
	msg []byte
}

func (p *PeerMetaBroadcast) Message() []byte { return p.msg }
func (p *PeerMetaBroadcast) Finished()       {}

func RenderPeerMetricsUI() {
	peerMetricsMutex.Lock()
	defer peerMetricsMutex.Unlock()

	columns := []table.Column{
		{Title: "Host", Width: 12},
		{Title: "IP", Width: 15},
		{Title: "CPU%", Width: 7},
		{Title: "RAM GB", Width: 8},
		{Title: "I/O RW", Width: 12},
		{Title: "Files Indexed", Width: 15},
	}

	var rows []table.Row
	var totalCPU, totalMemory, totalIORead, totalIOWrite float64
	var totalFiles int

	for _, peer := range peerMetrics {
		rows = append(rows, table.Row{
			peer.Host, peer.IP,
			fmt.Sprintf("%.1f", peer.CPU),
			fmt.Sprintf("%.1f", peer.MemoryGB),
			fmt.Sprintf("%.1fMB/s", peer.IOReadMB+peer.IOWriteMB),
			fmt.Sprintf("%d", peer.FilesIndexed),
		})

		totalCPU += peer.CPU
		totalMemory += peer.MemoryGB
		totalIORead += peer.IOReadMB
		totalIOWrite += peer.IOWriteMB
		totalFiles += peer.FilesIndexed
	}

	ows = append(rows, table.Row{
		"CLUSTER TOTAL", "",
		fmt.Sprintf("%.1f", totalCPU/float64(len(peerMetrics))),
		fmt.Sprintf("%.1f", totalMemory),
		fmt.Sprintf("%.1fMB/s", totalIORead+totalIOWrite),
		fmt.Sprintf("%d", totalFiles),
	})

	t := table.New()      // Fix: table.New() takes no arguments
	t.SetColumns(columns) // Fix: Set columns using SetColumns()
	t.SetRows(rows)

	fmt.Println(lipgloss.NewStyle().Bold(true).Render("\nPEER STATUS"))
	fmt.Println(t.View()) // Fix: Use t.View() instead of t.Render()
}
