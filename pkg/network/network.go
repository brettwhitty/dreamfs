package network

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/spf13/viper"
)

// ------------------------
// Global Peer List for HTTP-based Discovery
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