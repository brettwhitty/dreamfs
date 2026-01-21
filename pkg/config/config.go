package config

import (
	"time"

	"github.com/fatih/color"
	"github.com/spf13/viper"

	"gnomatix/dreamfs/v2/pkg/utils"
)

const (
	DefaultSwarmPort    = 7946
	DefaultWorkers      = 1 // unless --all-procs is provided
	DefaultQuiet        = false
	DefaultStealth      = false
	DefaultPeerListURL  = ""
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
