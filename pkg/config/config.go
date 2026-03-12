package config

import (
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/viper"

	"gnomatix/dreamfs/v2/pkg/utils"
)

const (
	DefaultSwarmPort   = 7946
	DefaultWorkers     = 1 // unless --all-procs is provided
	DefaultQuiet       = false
	DefaultStealth     = false
	DefaultPeerListURL = ""
	DefaultSyncInterval = 1 * time.Second
	DefaultBatchSize    = 100
)

// ------------------------
// Configuration and CLI Setup
// ------------------------

func InitConfig(cfgFile string) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Use XDG Config Home: e.g. $HOME/.config/dreamfs/dreamfs.json
		configPath := utils.XDGConfigFile("dreamfs.json")
		viper.SetConfigFile(configPath)

		// Create default config if it doesn't exist
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			// Set default values for the initial file
			viper.Set("dbpath", utils.DefaultBoltDBPath())
			viper.Set("addr", ":8080")
			viper.Set("swarm", false)
			viper.Set("theme", "gnomatix")

			if err := viper.SafeWriteConfig(); err != nil {
				color.Yellow("Warning: Failed to create default config: %v", err)
			} else {
				color.Green("Created default configuration: %s", configPath)
			}
		}
	}
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		color.Magenta("Using config file: %s", viper.ConfigFileUsed())
	} else {
		color.Yellow("No config file found; using defaults and flags")
	}
}