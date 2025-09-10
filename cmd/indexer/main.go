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
	"github.com/karrick/godirwalk"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zeebo/blake3"

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
// Canonicalize Paths for Physical Uniqueness
// ------------------------

func canonicalizePath(absPath string) (string, error) {
        // Windows UNC paths.
        if runtime.GOOS == "windows" {
                if strings.HasPrefix(absPath, `\\`) {
                        parts := strings.SplitN(absPath[2:], `\`, 3)
                        if len(parts) >= 2 {
                                server := parts[0]
                                share := parts[1]
                                rest := ""
                                if len(parts) == 3 {
                                        rest = "/" + parts[2]
                                }
                                return fmt.Sprintf("%s:/%s%s", server, share, rest), nil
                        }
                }
                return absPath, nil
        }

        parts, err := fileprocessor.GetPartitions()
        if err != nil {
                return absPath, err
        }
        var bestMatch disk.PartitionStat
        bestLen := 0
        for _, p := range parts {
                if strings.HasPrefix(absPath, p.Mountpoint) && len(p.Mountpoint) > bestLen {
                        bestLen = len(p.Mountpoint)
                        bestMatch = p
                }
        }
        if bestLen > 0 {
                networkFSTypes := map[string]bool{
                        "nfs":   true,
                        "nfs4":  true,
                        "cifs":  true,
                        "smbfs": true,
                        "afp":   true,
                }
                if networkFSTypes[strings.ToLower(bestMatch.Fstype)] {
                        relPath := absPath[len(bestMatch.Mountpoint):]
                        if !strings.HasPrefix(relPath, "/") {
                                relPath = "/" + relPath
                        }
                        return fmt.Sprintf("%s:%s", bestMatch.Device, relPath), nil
                }
        }
        return absPath, nil
}

// ------------------------
// Fingerprinting and File Processing
// ------------------------

const fileSampleSize = 1 << 20

func FingerprintFile(path string) (string, error) {
        f, err := os.Open(path)
        if err != nil {
                return "", fmt.Errorf("open file: %w", err)
        }
        defer f.Close()

        info, err := f.Stat()
        if err != nil {
                return "", fmt.Errorf("stat file: %w", err)
        }

        var data []byte
        if info.Size() < 3*fileSampleSize {
                data, err = io.ReadAll(f)
                if err != nil {
                        return "", fmt.Errorf("read file: %w", err)
                }
        } else {
                data = make([]byte, 0, 3*fileSampleSize)
                head := make([]byte, fileSampleSize)
                if _, err := f.Read(head); err != nil {
                        return "", fmt.Errorf("read head: %w", err)
                }
                data = append(data, head...)

                midOffset := info.Size() / 2
                if _, err := f.Seek(midOffset, io.SeekStart); err != nil {
                        return "", fmt.Errorf("seek middle: %w", err)
                }
                mid := make([]byte, fileSampleSize)
                if _, err := io.ReadFull(f, mid); err != nil {
                        return "", fmt.Errorf("read middle: %w", err)
                }
                data = append(data, mid...)

                tailOffset := info.Size() - fileSampleSize
                if _, err := f.Seek(tailOffset, io.SeekStart); err != nil {
                        return "", fmt.Errorf("seek tail: %w", err)
                }
                tail := make([]byte, fileSampleSize)
                if _, err := io.ReadFull(f, tail); err != nil {
                        return "", fmt.Errorf("read tail: %w", err)
                }
                data = append(data, tail...)
        }

        hash := blake3.Sum256(data)
        return fmt.Sprintf("%x", hash), nil
}

// Global swarm delegate.
var swarmDelegate *network.SwarmDelegate

func ProcessFile(ctx context.Context, filePath string, ps *storage.PersistentStore, store bool) (string, error) {
        select {
        case <-ctx.Done():
                return "", ctx.Err()
        default:
        }
        info, err := os.Stat(filePath)
        if err != nil {
                return "", fmt.Errorf("failed to stat %s: %w", filePath, err)
        }
        if info.IsDir() {
                return "", nil
        }
        absPath, err := filepath.Abs(filePath)
        if err != nil {
                return "", fmt.Errorf("failed to get absolute path for %s: %w", filePath, err)
        }
        canonicalPath, err := canonicalizePath(absPath)
        if err != nil {
                canonicalPath = absPath
        }
        fingerprint, err := FingerprintFile(filePath)
        if err != nil {
                return "", fmt.Errorf("failed to fingerprint %s: %w", filePath, err)
        }
        if store {
                meta := metadata.FileMetadata{
                        ID:       fingerprint,
                        FilePath: canonicalPath,
                        Size:     info.Size(),
                        ModTime:  info.ModTime().Format(time.RFC3339),
                        Extra:    map[string]interface{}{},
                }
                if err := ps.Put(meta); err != nil {
                        return "", fmt.Errorf("failed to store metadata for %s: %w", filePath, err)
                }
                if swarmDelegate != nil {
                        data, err := json.Marshal(meta)
                        if err == nil {
                                swarmDelegate.broadcasts.QueueBroadcast(&network.FileMetaBroadcast{msg: data})
                        }
                }
        }
        return fingerprint, nil
}

// ------------------------
// Directory Processing with godirwalk and Charm UI Feedback
// ------------------------

// processAllDirectories scans the root directory and processes its files,
// then collects subdirectories and processes them one at a time. A spinner is
// shown while reading directories, and a progress bar is updated per subdirectory.
func processAllDirectories(ctx context.Context, root string, ps *storage.PersistentStore) error {
        quiet := viper.GetBool("quiet")
        if !quiet {
                fmt.Println("Reading files...")
        }
        // Process files in the root directory.
        if !quiet {
                fmt.Printf("Processing root directory: %s\n", root)
        }
        err := godirwalk.Walk(root, &godirwalk.Options{
                Unsorted: true,
                Callback: func(path string, de *godirwalk.Dirent) error {
                        select {
                        case <-ctx.Done():
                                return ctx.Err()
                        default:
                        }
                        // Only process files directly in root.
                        if de.IsDir() && path != root {
                                return godirwalk.SkipNode
                        }
                        if !de.IsDir() {
                                _, err := ProcessFile(ctx, path, ps, true)
                                if err != nil && !quiet {
                                        fmt.Printf("Error processing %s: %v\n", path, err)
                                }
                        }
                        return nil
                },
        })
        if err != nil {
                return err
        }

        // Collect all subdirectories.
        var subdirs []string
        err = godirwalk.Walk(root, &godirwalk.Options{
                Unsorted: true,
                Callback: func(path string, de *godirwalk.Dirent) error {
                        select {
                        case <-ctx.Done():
                                return ctx.Err()
                        default:
                        }
                        if de.IsDir() && path != root {
                                subdirs = append(subdirs, path)
                        }
                        return nil
                },
        })
        if err != nil {
                return err
        }

        // Process each subdirectory.
        for i, dir := range subdirs {
                select {
                case <-ctx.Done():
                        return ctx.Err()
                default:
                }
                if !quiet {
                        fmt.Printf("\nProcessing directory (%d/%d): %s\n", i+1, len(subdirs), dir)
                }
                // Collect files in the subdirectory.
                var filesInDir []string
                err = godirwalk.Walk(dir, &godirwalk.Options{
                        Unsorted: true,
                        Callback: func(path string, de *godirwalk.Dirent) error {
                                select {
                                case <-ctx.Done():
                                        return ctx.Err()
                                default:
                                }
                                if !de.IsDir() {
                                        filesInDir = append(filesInDir, path)
                                }
                                return nil
                        },
                })
                if err != nil {
                        if !quiet {
                                fmt.Printf("Error reading directory %s: %v\n", dir, err)
                        }
                        continue
                }
                totalFiles := len(filesInDir)
                if totalFiles == 0 {
                        continue
                }
                // Initialize progress bar and spinner.
                var sp spinner.Model
                if !quiet {
                        sp = spinner.New()
                        sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
                        sp.Start()
                        fmt.Printf("Processing files in %s...\n", dir)
                }
                p := progress.New(progress.WithDefaultGradient())
                var processed int64
                for _, fpath := range filesInDir {
                        select {
                        case <-ctx.Done():
                                return ctx.Err()
                        default:
                        }
                        _, err := ProcessFile(ctx, fpath, ps, true)
                        if err != nil && !quiet {
                                fmt.Printf("Error processing %s: %v\n", fpath, err)
                        }
                        processed++
                        if !quiet {
                                percent := float64(processed) / float64(totalFiles)
                                fmt.Printf("\r%s", lipgloss.NewStyle().Bold(true).Render(p.View(percent)))
                        }
                }
                if !quiet {
                        sp.Stop()
                        fmt.Println()
                }
        }
        return nil
}

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
                        }
                        log.Printf("Joined %d HTTP-discovered peers", n)
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
                                _, err := ProcessFile(ctx, path, nil, false)
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

                        if err := processAllDirectories(ctx, dir, ps); err != nil {
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