package config

import (
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/viper"

	"gnomatix/dreamfs/v2/pkg/utils"
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