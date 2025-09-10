package network

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/mdns"
	"github.com/hashicorp/memberlist"
	"github.com/spf13/viper"

	"gnomatix/dreamfs/v2/pkg/metadata"
	"gnomatix/dreamfs/v2/pkg/storage"
)

// ------------------------
// HTTP Server: Replication and Peer List Endpoints
// ------------------------

var (
	peerList      []string
	peerListMutex sync.Mutex
)

func HandlePeerList(w http.ResponseWriter, r *http.Request) {
	// Extract remote IP address.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peerAddr := fmt.Sprintf("%s:%d", host, viper.GetInt("swarmPort"))

	peerListMutex.Lock()
	defer peerListMutex.Unlock()
	// Add if not already present.
	found := false
	for _, p := range peerList {
		if p == peerAddr {
			found = true
			break
		}
	}
	if !found {
		peerList = append(peerList, peerAddr)
		log.Printf("Added new peer via HTTP: %s", peerAddr)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(peerList); err != nil {
		http.Error(w, "failed to encode peer list", http.StatusInternalServerError)
	}
}

func StartHTTPServer(addr string, ps *storage.PersistentStore) {
	http.HandleFunc("/_changes", func(w http.ResponseWriter, r *http.Request) {
		metas, err := ps.GetAll()
		if err != nil {
			http.Error(w, "failed to get metadata", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metas); err != nil {
			color.Red("failed to encode changes: %v", err)
		}
	})
	http.HandleFunc("/peerlist", HandlePeerList) // Corrected call

	color.Blue("Starting HTTP server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// ------------------------
// Database Dump
// ------------------------

func DumpDB(ps *storage.PersistentStore, format string) {
	metas, err := ps.GetAll()
	if err != nil {
		log.Fatalf("failed to get metadata: %v", err)
	}
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(metas); err != nil {
			log.Fatalf("failed to encode JSON: %v", err)
		}
	case "tsv":
		w := csv.NewWriter(os.Stdout)
		w.Comma = '\t'
		w.Write([]string{"_id", "filePath", "size", "modTime"})
		for _, meta := range metas {
			w.Write([]string{meta.ID, meta.FilePath, strconv.FormatInt(meta.Size, 10), meta.ModTime})
		}
		w.Flush()
	default:
		log.Fatalf("unknown dump format: %s", format)
	}
}

// ------------------------
// Swarm Replication using memberlist with mDNS or HTTP-based Peer Discovery
// ------------------------

type FileMetaBroadcast struct {
	Msg []byte
}

func (f *FileMetaBroadcast) Message() []byte { return f.Msg }
func (f *FileMetaBroadcast) Finished()       {}

type PeerMetaBroadcast struct {
	Msg []byte
}

func (p *PeerMetaBroadcast) Message() []byte { return p.Msg }
func (p *PeerMetaBroadcast) Finished()       {}
func (p *PeerMetaBroadcast) Invalidates(other memberlist.Broadcast) bool {
	return false
}

type SwarmDelegate struct {
	ps         *storage.PersistentStore
	Broadcasts *memberlist.TransmitLimitedQueue // Exported Broadcasts
}

func NewSwarmDelegate(ps *storage.PersistentStore, ml *memberlist.Memberlist) *SwarmDelegate {
	d := &SwarmDelegate{ps: ps}
	d.Broadcasts = &memberlist.TransmitLimitedQueue{ // Use Broadcasts
		NumNodes: func() int { return len(ml.Members()) },
		RetransmitMult: 3,
	}
	return d
}

func (d *SwarmDelegate) NodeMeta(limit int) []byte {
	return []byte{}
}

func (d *SwarmDelegate) NotifyMsg(msg []byte) {
	var meta metadata.FileMetadata
	if err := json.Unmarshal(msg, &meta); err != nil {
		log.Printf("Swarm: failed to unmarshal metadata: %v", err)
		return
	}
	if err := d.ps.Put(meta); err != nil {
		log.Printf("Swarm: failed to store metadata for %s: %v", meta.FilePath, err)
		return
	}
	log.Printf("Swarm: received and stored metadata for %s", meta.FilePath)
}

func (d *SwarmDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	return d.Broadcasts.GetBroadcasts(overhead, limit) // Use Broadcasts
}

func (d *SwarmDelegate) LocalState(join bool) []byte {
	metas, err := d.ps.GetAll()
	if err != nil {
		return nil
	}
	data, err := json.Marshal(metas)
	if err != nil {
		return nil
	}
	return data
}

func (d *SwarmDelegate) MergeRemoteState(buf []byte, join bool) {
	var metas []metadata.FileMetadata
	if err := json.Unmarshal(buf, &metas); err != nil {
		log.Printf("Swarm: failed to merge remote state: %v", err)
		return
	}
	for _, meta := range metas {
		if err := d.ps.Put(meta); err != nil {
			log.Printf("Swarm: failed to merge metadata for %s: %v", meta.FilePath, err)
		}
	}
}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func GetPeerListFromHTTP(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var peers []string
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, err
	}
	return peers, nil
}

func StartSwarm(ps *storage.PersistentStore) (*memberlist.Memberlist, *SwarmDelegate, error) {
	cfg := memberlist.DefaultLocalConfig() // Corrected typo
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "node"
	}
	cfg.Name = fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
	cfg.BindPort = viper.GetInt("swarmPort")

	ml, err := memberlist.Create(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create memberlist: %w", err)
	}
	d := NewSwarmDelegate(ps, ml)
	cfg.Delegate = d

	peerListURL := viper.GetString("peerListURL")
	if peerListURL != "" {
		discovered, err := GetPeerListFromHTTP(peerListURL)
		if err != nil {
			log.Printf("HTTP peer list lookup error: %v", err)
		} else if len(discovered) > 0 {
			n, err := ml.Join(discovered)
			if err != nil {
				log.Printf("Failed to join HTTP-discovered peers: %v", err)
			}
			log.Printf("Joined %d HTTP-discovered peers", n)
		} else {
			log.Printf("No peers discovered from HTTP endpoint")
		}
	} else if !viper.GetBool("stealth") {
		ip := net.ParseIP(GetLocalIP())
		srv, err := mdns.NewMDNSService(hostname, "_indexer._tcp", "", "", viper.GetInt("swarmPort"), []net.IP{ip}, []string{"Hello friend"})
		if err != nil {
			log.Printf("mDNS service error: %v", err)
		} else {
			mdnsServer, err := mdns.NewServer(&mdns.Config{Zone: srv})
			if err != nil {
				log.Printf("mDNS server error: %v", err)
			}
			go func() {
				<-time.After(10 * time.Minute)
				mdnsServer.Shutdown()
			}()
		}
		var discovered []string
		entriesCh := make(chan *mdns.ServiceEntry, 4)
		go func() {
			for entry := range entriesCh {
				if entry.AddrV4.String() == ip.String() {
					continue
				}
				discovered = append(discovered, fmt.Sprintf("%s:%d", entry.AddrV4.String(), viper.GetInt("swarmPort")))
			}
		}()
		err = mdns.Query(&mdns.QueryParam{
			Service: "_indexer._tcp",
			Domain:  "local",
			Timeout: time.Second * 3,
			Entries: entriesCh,
		})
		close(entriesCh)
		if len(discovered) > 0 {
			n, err := ml.Join(discovered)
			if err != nil {
				log.Printf("Swarm auto-discovery join error: %v", err)
			}
			log.Printf("Swarm auto-discovery: joined %d peers", n)
		} else {
			log.Printf("Swarm auto-discovery: no peers found")
		}
	} else {
		peers := viper.GetStringSlice("peers")
		if len(peers) > 0 {
			n, err := ml.Join(peers)
			if err != nil {
				log.Printf("Swarm: failed to join manual peers: %v", err)
			}
			log.Printf("Swarm: joined %d manual peers", n)
		}
	}

	log.Printf("Swarm: node %s started on port %d", cfg.Name, cfg.BindPort)
	return ml, d, nil
}
