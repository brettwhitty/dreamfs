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
		Long: `wiki-docs is a document deployment tool for Gitea wikis.
The wiki is the authority for document content and metadata.
Pull stages approved/versioned docs from the wiki into the repo.
Push feeds back local edits to the wiki for review.`,
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
