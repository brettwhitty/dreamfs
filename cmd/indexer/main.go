package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/fatih/color"
	"github.com/hashicorp/memberlist"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gnomatix/dreamfs/v2/pkg/storage"
	"gnomatix/dreamfs/v2/pkg/network"
	"gnomatix/dreamfs/v2/pkg/fileprocessor"
	"gnomatix/dreamfs/v2/pkg/utils"
)

// ------------------------
// Configurable Defaults
// ------------------------

const (
	defaultSwarmPort = 7946
	defaultWorkers   = 1 // unless --all-procs is provided
	defaultQuiet     = false
	defaultStealth   = false
	defaultPeerListURL = ""
)

// Global swarm delegate.
var swarmDelegate *network.SwarmDelegate

// ------------------------
// Configuration and CLI Setup
// ------------------------

func initConfig(cfgFile string) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		xdgConfigDir := utils.XDGDataHome()
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
	rootCmd.PersistentFlags().String("dbpath", utils.DefaultBoltDBPath(), "Path to the BoltDB file (default: XDG data directory)")
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
				ml, swarmDelegate, err = network.StartSwarm(ps)
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
				ml, swarmDelegate, err = network.StartSwarm(ps)
				if err != nil {
					color.Red("failed to start swarm: %v", err)
					os.Exit(1)
				}
				defer ml.Shutdown()
			}
			network.StartHTTPServer(addr, ps)
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
			network.DumpDB(ps, format)
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