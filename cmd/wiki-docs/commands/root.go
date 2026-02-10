package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	cfgWikiPath string

	rootCmd = &cobra.Command{
		Use:   "wiki-docs",
		Short: "Manage documentation between local docs and Gitea Wiki",
		Long: `wiki-docs manages the synchronization and organization of documentation.
It supports pushing local 'docs/' to a Gitea Wiki repository
and pulling changes back, with support for interactive selection.`,
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cwd, _ := os.Getwd()
	defaultWiki := os.Getenv("WIKI_PATH")
	if defaultWiki == "" {
		defaultWiki = filepath.Join(cwd, "wiki")
	}

	rootCmd.PersistentFlags().StringVar(&cfgWikiPath, "wiki-path", defaultWiki, "Path to the local clone of the wiki repository (env: WIKI_PATH)")
}
