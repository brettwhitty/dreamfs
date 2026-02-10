package commands

import (
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tracked files and their sync status",
	Long:  `Lists all files found in the configured sources and the wiki, showing their synchronization status.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := getConfig(cmd)
		if err != nil {
			printFatal("Configuration Error", err)
		}

		if err := validateWikiDir(cfg.WikiDir); err != nil {
			printFatal("Wiki Directory Invalid", err)
		}

		items, err := ScanAll(cfg)
		if err != nil {
			printFatal("Scan Failed", err)
		}

		if err := runListTUI(items, cfg); err != nil {
			printFatal("TUI Error", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
