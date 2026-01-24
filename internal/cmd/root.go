package cmd

import (
	"os"

	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
	rootCmd = &cobra.Command{
		Use:   "pxctl",
		Short: "pxGrid Direct test data generation and loading tool",
		Long: `pxctl is a CLI tool for generating and loading test data for Cisco ISE pxGrid Direct connectors.
It connects to ISE, retrieves connector configurations, generates sample data according to
the connector's schema, and can load that data into ISE via push connectors.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger.SetVerbose(verbose)
		},
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.pxctl.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging to stderr")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".pxctl")
	}

	viper.AutomaticEnv()
	viper.ReadInConfig()
}
