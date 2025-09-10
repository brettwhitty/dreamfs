package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/adrg/xdg"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/hashicorp/mdns"
	"github.com/hashicorp/memberlist"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gnomatix/dreamfs/v2/pkg/metadata"
	"gnomatix/dreamfs/v2/pkg/storage"
	"gnomatix/dreamfs/v2/pkg/network"
	"gnomatix/dreamfs/v2/pkg/fileprocessor"
)

// ------------------------
// Configurable Defaults
// ------------------------

// DefaultBoltDBPath returns the system-appropriate default DB path.
func DefaultBoltDBPath() string {
        // Use XDG data home; for Windows or macOS this resolves appropriately. 
        dataHome := xdg.DataHome
        return filepath.Join(dataHome, "indexer", "indexer.db")
}

const (
        defaultSwarmPort = 7946
        defaultWorkers   = 1 // unless --all-procs is provided
        defaultQuiet     = false
        defaultStealth   = false
        defaultPeerListURL = ""
)

// ------------------------
// HTTP Server: Replication and Peer List Endpoints
// ------------------------

func startHTTPServer(addr string, ps *storage.PersistentStore) {
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
        http.HandleFunc("/peerlist", network.HandlePeerList)

        color.Blue("Starting HTTP server on %s", addr)
        if err := http.ListenAndServe(addr, nil); err != nil {
                log.Fatalf("HTTP server error: %v", err)
        }
}

// ------------------------
// Database Dump
// ------------------------

func dumpDB(ps *storage.PersistentStore, format string) {
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

type fileMetaBroadcast struct {
        msg []byte
}

func (f *fileMetaBroadcast) Message() []byte { return f.msg }
func (f *fileMetaBroadcast) Finished()       {}

type SwarmDelegate struct {
        ps         *storage.PersistentStore
        broadcasts *memberlist.TransmitLimitedQueue
}

func NewSwarmDelegate(ps *storage.PersistentStore, ml *memberlist.Memberlist) *SwarmDelegate {
        d := &SwarmDelegate{ps: ps}
        d.broadcasts = &memberlist.TransmitLimitedQueue{
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
        return d.broadcasts.GetBroadcasts(overhead, limit)
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

func getLocalIP() string {
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

func getPeerListFromHTTP(url string) ([]string, error) {
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

func startSwarm(ps *storage.PersistentStore) (*memberlist.Memberlist, *network.SwarmDelegate, error) {
        cfg := memberlist.DefaultLocalConfig()
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
        d := network.NewSwarmDelegate(ps, ml)
        cfg.Delegate = d

        peerListURL := viper.GetString("peerListURL")
        if peerListURL != "" {
                discovered, err := getPeerListFromHTTP(peerListURL)
                if err != nil {
                        log.Printf("HTTP peer list lookup error: %v", err)
                } else if len(discovered) > 0 {
                        n, err := ml.Join(discovered)
                        if err != nil {
                                log.Printf("Failed to join HTTP-discovered peers: %v", err)
                        } else {
                                log.Printf("Joined %d HTTP-discovered peers", n)
                        }
                } else {
                        log.Printf("No peers discovered from HTTP endpoint")
                }
        } else if !viper.GetBool("stealth") {
                ip := net.ParseIP(getLocalIP())
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

// ------------------------
// Configuration and CLI Setup
// ------------------------

func initConfig(cfgFile string) {
        if cfgFile != "" {
                viper.SetConfigFile(cfgFile)
        } else {
                xdgConfigDir := xdg.DataHome
                viper.AddConfigPath(xdgConfigDir)
                viper.SetConfigName("indexer")
                viper.SetConfigType("json")
        }
        viper.AutomaticEnv()
        if err := viper.ReadInConfig(); err == nil {
                color.Magenta("Using config file: %s", viper.ConfigFileUsed())
        } else {
                color.Yellow("No config file found; using defaults and flags")
        }
}

func main() {
        var cfgFile string

        rootCmd := &cobra.Command{
                Use:   "indexer [path]",
                Short: "Index files and expose a replication source endpoint",
                Run: func(cmd *cobra.Command, args []string) {
                        // Default: list file fingerprints (like md5sum) for each file.
                        if len(args) < 1 {
                                cmd.Help()
                                return
                        }
                        ctx, cancel := context.WithCancel(context.Background())
                        defer cancel()
                        sigCh := make(chan os.Signal, 1)
                        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
                        go func() {
                                <-sigCh
                                cancel()
                        }()
                        for _, path := range args {
                                _, err := fileprocessor.ProcessFile(ctx, path, nil, false)
                                if err != nil {
                                        log.Printf("Error processing %s: %v", path, err)
                                }
                        }
                },
        }

        cobra.OnInitialize(func() {
                initConfig(cfgFile)
        })

        // Global flags.
        rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: indexer.json in XDG config directory)")
        rootCmd.PersistentFlags().String("dbpath", storage.DefaultBoltDBPath(), "Path to the BoltDB file (default: XDG data directory)")
        rootCmd.PersistentFlags().String("addr", ":8080", "Address to serve the replication endpoint")
        // Default workers is 1 unless --all-procs is set.
        rootCmd.PersistentFlags().Int("workers", defaultWorkers, "Number of concurrent workers for indexing (default: 1, use --all-procs to use all available CPUs)")
        rootCmd.PersistentFlags().Bool("all-procs", false, "Use all available processors (overrides --workers)")	rootCmd.PersistentFlags().Bool("quiet", defaultQuiet, "Suppress spinner and progress messages")
        rootCmd.PersistentFlags().Bool("swarm", false, "Enable swarm mode for p2p replication")
        rootCmd.PersistentFlags().StringSlice("peers", []string{}, "Comma-separated list of peer addresses to join")
        rootCmd.PersistentFlags().Int("swarmPort", defaultSwarmPort, "Port for swarm memberlist")
        rootCmd.PersistentFlags().Bool("stealth", defaultStealth, "Enable stealth mode which disables mDNS auto-discovery (requires manual peer list)")
        rootCmd.PersistentFlags().String("peerListURL", defaultPeerListURL, "HTTP/HTTPS URL that returns a JSON array of peer addresses")
        viper.BindPFlag("dbpath", rootCmd.PersistentFlags().Lookup("dbpath"))
        viper.BindPFlag("addr", rootCmd.PersistentFlags().Lookup("addr"))
        viper.BindPFlag("workers", rootCmd.PersistentFlags().Lookup("workers"))
        viper.BindPFlag("all-procs", rootCmd.PersistentFlags().Lookup("all-procs"))
        viper.BindPFlag("quiet", rootCmd.PersistentFlags().Lookup("quiet"))
        viper.BindPFlag("swarm", rootCmd.PersistentFlags().Lookup("swarm"))
        viper.BindPFlag("peers", rootCmd.PersistentFlags().Lookup("peers"))
        viper.BindPFlag("swarmPort", rootCmd.PersistentFlags().Lookup("swarmPort"))
        viper.BindPFlag("stealth", rootCmd.PersistentFlags().Lookup("stealth"))
        viper.BindPFlag("peerListURL", rootCmd.PersistentFlags().Lookup("peerListURL"))

        // "index" command: Process a directory with per-subdirectory status and progress.
        indexCmd := &cobra.Command{
                Use:   "index [directory]",
                Short: "Scan a directory and index files with live status updates",
                Args:  cobra.ExactArgs(1),
                Run: func(cmd *cobra.Command, args []string) {
                        dir := args[0]
                        dbPath := viper.GetString("dbpath")
                        ps, err := storage.NewPersistentStore(dbPath)
                        if err != nil {
                                color.Red("failed to open persistent store: %v", err)
                                os.Exit(1)
                        }
                        defer ps.Close()

                        // Handle workers: if --all-procs is set, override workers.
                        if viper.GetBool("all-procs") {
                                viper.Set("workers", runtime.NumCPU())
                        }
                        // If swarm is enabled, start memberlist.
                        var ml *memberlist.Memberlist
                        if viper.GetBool("swarm") {
                                ml, swarmDelegate, err = startSwarm(ps)
                                if err != nil {
                                        color.Red("failed to start swarm: %v", err)
                                        os.Exit(1)
                                }
                                defer ml.Shutdown()
                        }

                        ctx, cancel := context.WithCancel(context.Background())
                        defer cancel()
                        sigCh := make(chan os.Signal, 1)
                        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
                        go func() {
                                <-sigCh
                                cancel()
                        }()

                        if err := fileprocessor.ProcessAllDirectories(ctx, dir, ps); err != nil {
                                color.Red("Error during directory processing: %v", err)
                        }
                },
        }

        // "serve" command.
        serveCmd := &cobra.Command{
                Use:   "serve",
                Short: "Run in daemon mode, exposing replication (/ _changes) and peer list (/peerlist) endpoints",
                Run: func(cmd *cobra.Command, args []string) {
                        dbPath := viper.GetString("dbpath")
                        addr := viper.GetString("addr")
                        ps, err := storage.NewPersistentStore(dbPath)
                        if err != nil {
                                color.Red("failed to open persistent store: %v", err)
                                os.Exit(1)
                        }
                        defer ps.Close()
                        var ml *memberlist.Memberlist
                        if viper.GetBool("swarm") {
                                ml, swarmDelegate, err = startSwarm(ps)
                                if err != nil {
                                        color.Red("failed to start swarm: %v", err)
                                        os.Exit(1)
                                }
                                defer ml.Shutdown()
                        }
                        startHTTPServer(addr, ps)
                },
        }

        // "dump" command.
        dumpCmd := &cobra.Command{
                Use:   "dump",
                Short: "Dump the persistent database contents",
                Run: func(cmd *cobra.Command, args []string) {
                        dbPath := viper.GetString("dbpath")
                        format := viper.GetString("format")
                        ps, err := storage.NewPersistentStore(dbPath)
                        if err != nil {
                                color.Red("failed to open persistent store: %v", err)
                                os.Exit(1)
                        }
                        defer ps.Close()
                        dumpDB(ps, format)
                },
        }
        dumpCmd.Flags().String("format", "json", "Dump format: json or tsv")
        viper.BindPFlag("format", dumpCmd.Flags().Lookup("format"))

        rootCmd.AddCommand(indexCmd)
        rootCmd.AddCommand(serveCmd)
        rootCmd.AddCommand(dumpCmd)

        if err := rootCmd.Execute(); err != nil {
                log.Fatal(err)
        }
}
