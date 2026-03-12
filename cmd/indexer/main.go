package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/fatih/color"
	"github.com/hashicorp/memberlist"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gnomatix/dreamfs/v2/pkg/api"
	"gnomatix/dreamfs/v2/pkg/storage"
	"gnomatix/dreamfs/v2/pkg/network"
	"gnomatix/dreamfs/v2/pkg/fileprocessor"
	"gnomatix/dreamfs/v2/pkg/utils"
	"gnomatix/dreamfs/v2/pkg/config"
	"gnomatix/dreamfs/v2/pkg/ui"
)

// Global swarm delegate.
var swarmDelegate *network.SwarmDelegate

// Shared indexer state — updated by the fileprocessor, read by the API server.
var indexerState = api.NewIndexerState()

// Version information set via ldflags or VERSION file
var (
	Version = "0.1.1"
	Build   = "dev"
)

var cfgFile string // Declare cfgFile at package level

var rootCmd = &cobra.Command{
	Use:     "dreamfs",
	Version: Version + "-" + Build,
	Short:   "DreamFS: Distributed Datastore for Extended File Attributes",
	Long: `DreamFS: Distributed Datastore for Extended File Attributes
Version: ` + Version + ` Build: ` + Build + `

DreamFS is a lightweight, zero-config distributed datastore for extended file attributes.
It provides a unified view of metadata across your entire digital swarm.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		ui.SetTheme(viper.GetString("theme"))
		if !viper.GetBool("quiet") {
			fmt.Print(ui.RenderHeader(Version, Build))
		}
	},

}

func init() { // Use init function for Cobra setup
	cobra.OnInitialize(func() {
		config.InitConfig(cfgFile)
		utils.SetHostID()
	})

	// Global flags.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: dreamfs.json in XDG config directory)")
	rootCmd.PersistentFlags().String("dbpath", utils.DefaultBoltDBPath(), "Path to the BoltDB file (default: XDG data directory)")
	rootCmd.PersistentFlags().Bool("quiet", config.DefaultQuiet, "Suppress spinner and progress messages")
	rootCmd.PersistentFlags().String("theme", "gnomatix", "UI theme (gnomatix, zissou, budapest, rushmore, royal1, royal2, darjeeling1, darjeeling2, fantasticfox, moonrise[1-3], chevalier, cavalcanti, isleofdogs[1-2], frenchdispatch, asteroidcity1)")
	
	viper.BindPFlag("dbpath", rootCmd.PersistentFlags().Lookup("dbpath"))
	viper.BindPFlag("quiet", rootCmd.PersistentFlags().Lookup("quiet"))
	viper.BindPFlag("theme", rootCmd.PersistentFlags().Lookup("theme"))

	// Setup custom Help and Usage templates
	rootCmd.SetUsageTemplate(`Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`)

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		ui.SetTheme(viper.GetString("theme"))
		
		// Config Summary
		cfg := [][]string{
			{"Theme", viper.GetString("theme")},
			{"Database Path", viper.GetString("dbpath")},
			{"Quiet Mode", fmt.Sprintf("%v", viper.GetBool("quiet"))},
			{"Swarm Enabled", fmt.Sprintf("%v", viper.GetBool("swarm"))},
			{"Web Server", fmt.Sprintf("%v", !viper.GetBool("no-web"))},
		}
		if viper.GetBool("swarm") {
			cfg = append(cfg, []string{"Swarm Port", fmt.Sprintf("%d", viper.GetInt("swarmPort"))})
		}
		if cmd.Name() == "index" || cmd.Name() == "dreamfs" {
			cfg = append(cfg, []string{"Workers", fmt.Sprintf("%d", viper.GetInt("workers"))})
		}
		fmt.Print(ui.RenderCard("Active Configuration", cfg))
		
		// Standard Usage
		fmt.Print(cmd.UsageString())

		// System Identity & License at absolute bottom
		fmt.Printf("\n  %s %s (%s)   (c) 2024-6 Brett Whitty, GNOMATIX\n\n",
			ui.StyleKey.Render("DreamFS"), Version, Build)
	})

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
				ml, swarmDelegate, err = network.StartSwarm(ps) // Assign to global swarmDelegate
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

			// Start Gin web console in the background (unless --no-web is set).
			if !viper.GetBool("no-web") {
				apiServer := api.NewServer(ps, indexerState)
				webAddr := viper.GetString("web-addr")
				if webAddr == "" {
					webAddr = ":8080"
				}
				color.Cyan("Web console: http://localhost%s/", webAddr)
				go func() {
					if err := apiServer.Run(ctx, webAddr); err != nil {
						log.Printf("web console: %v", err)
					}
				}()
			}

			// Initialize CacheWriter for batched database writes
			batchSize := viper.GetInt("batchSize")
			if batchSize == 0 {
				batchSize = config.DefaultBatchSize
			}
			cw := storage.NewCacheWriter(ps, batchSize, config.DefaultSyncInterval)
			defer cw.Close()

			workers := viper.GetInt("workers")
			if err := fileprocessor.ProcessAllDirectories(ctx, dir, cw, workers, swarmDelegate); err != nil {
				color.Red("Error during directory processing: %v", err)
			}
			
			// Ensure cache is flushed immediately after completion
			cw.FlushNow()
		},
	}
	
	// Add indexCmd flags
	indexCmd.Flags().String("addr", ":8080", "Address to serve the replication endpoint")
	indexCmd.Flags().Int("workers", config.DefaultWorkers, "Number of concurrent workers for indexing (default: 1, use --all-procs to use all available CPUs)")
	indexCmd.Flags().Bool("all-procs", false, "Use all available processors (overrides --workers)")
	indexCmd.Flags().Bool("swarm", false, "Enable swarm mode for p2p replication")
	indexCmd.Flags().StringSlice("peers", []string{}, "Comma-separated list of peer addresses to join")
	indexCmd.Flags().Int("swarmPort", config.DefaultSwarmPort, "Port for swarm memberlist")
	indexCmd.Flags().Bool("stealth", config.DefaultStealth, "Enable stealth mode which disables mDNS auto-discovery (requires manual peer list)")
	indexCmd.Flags().String("peerListURL", config.DefaultPeerListURL, "HTTP/HTTPS URL that returns a JSON array of peer addresses")
	indexCmd.Flags().Bool("no-web", false, "Disable the built-in Gin web console")
	indexCmd.Flags().String("web-addr", ":8080", "Address for the built-in web console (e.g. :8080)")
	
	viper.BindPFlag("addr", indexCmd.Flags().Lookup("addr"))
	viper.BindPFlag("workers", indexCmd.Flags().Lookup("workers"))
	viper.BindPFlag("all-procs", indexCmd.Flags().Lookup("all-procs"))
	viper.BindPFlag("swarm", indexCmd.Flags().Lookup("swarm"))
	viper.BindPFlag("peers", indexCmd.Flags().Lookup("peers"))
	viper.BindPFlag("swarmPort", indexCmd.Flags().Lookup("swarmPort"))
	viper.BindPFlag("stealth", indexCmd.Flags().Lookup("stealth"))
	viper.BindPFlag("peerListURL", indexCmd.Flags().Lookup("peerListURL"))
	viper.BindPFlag("no-web", indexCmd.Flags().Lookup("no-web"))
	viper.BindPFlag("web-addr", indexCmd.Flags().Lookup("web-addr"))

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
				ml, swarmDelegate, err = network.StartSwarm(ps) // Assign to global swarmDelegate
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
			go func() { <-sigCh; cancel() }()

			webAddr := viper.GetString("web-addr")
			if webAddr == "" { webAddr = addr }
			if webAddr == "" { webAddr = ":8080" }
			apiServer := api.NewServer(ps, indexerState)
			color.Cyan("Web console: http://localhost%s/", webAddr)
			if err := apiServer.Run(ctx, webAddr); err != nil {
				log.Printf("serve: %v", err)
			}
		},
	}
	
	// Add serveCmd flags
	serveCmd.Flags().String("addr", ":8080", "Address to serve the replication endpoint")
	serveCmd.Flags().Bool("swarm", false, "Enable swarm mode for p2p replication")
	serveCmd.Flags().StringSlice("peers", []string{}, "Comma-separated list of peer addresses to join")
	serveCmd.Flags().Int("swarmPort", config.DefaultSwarmPort, "Port for swarm memberlist")
	serveCmd.Flags().Bool("stealth", config.DefaultStealth, "Enable stealth mode which disables mDNS auto-discovery (requires manual peer list)")
	serveCmd.Flags().String("peerListURL", config.DefaultPeerListURL, "HTTP/HTTPS URL that returns a JSON array of peer addresses")
	serveCmd.Flags().Bool("no-web", false, "Disable the built-in Gin web console")
	serveCmd.Flags().String("web-addr", ":8080", "Address for the built-in web console (e.g. :8080)")
	
	viper.BindPFlag("addr", serveCmd.Flags().Lookup("addr"))
	viper.BindPFlag("swarm", serveCmd.Flags().Lookup("swarm"))
	viper.BindPFlag("peers", serveCmd.Flags().Lookup("peers"))
	viper.BindPFlag("swarmPort", serveCmd.Flags().Lookup("swarmPort"))
	viper.BindPFlag("stealth", serveCmd.Flags().Lookup("stealth"))
	viper.BindPFlag("peerListURL", serveCmd.Flags().Lookup("peerListURL"))
	viper.BindPFlag("no-web", serveCmd.Flags().Lookup("no-web"))
	viper.BindPFlag("web-addr", serveCmd.Flags().Lookup("web-addr"))

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

	// "monitor" command: Implementation moved to pkg/ui
	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "Live dashboard of swarm metrics and indexer progress",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(ui.PrintLogo())
			fmt.Println(ui.StyleHeader.Render(" SWARM MONITOR "))
			fmt.Println(ui.StyleValue.Render("TUI Monitor starting soon... (Part 6 Implementation in Progress)"))
			// TODO: Start Bubble Tea model here
		},
	}
	rootCmd.AddCommand(monitorCmd)
	// Add config command
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage DreamFS configuration",
	}
	configShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			settings := viper.AllSettings()
			fmt.Print(ui.RenderJSON(settings))
		},
	}
	configEditCmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				if runtime.GOOS == "windows" {
					editor = "notepad"
				} else {
					editor = "nano"
				}
			}
			cfgPath := viper.ConfigFileUsed()
			if cfgPath == "" {
				color.Red("No configuration file used. Please create one.")
				os.Exit(1)
			}
			
			// Use the editor command. Split to check if it has args.
			execCmd := exec.Command(editor, cfgPath)
			execCmd.Stdin = os.Stdin
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			
			if err := execCmd.Run(); err != nil {
				color.Red("Failed to execute editor: %v", err)
			}
		},
	}
	configCmd.AddCommand(configShowCmd, configEditCmd)
	rootCmd.AddCommand(configCmd)

	rootCmd.AddCommand(monitorCmd)
	// Add version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("  %s %s (%s)   (c) 2024-6 Brett Whitty, GNOMATIX\n",
				ui.StyleKey.Render("DreamFS"), Version, Build)
		},
	}
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
