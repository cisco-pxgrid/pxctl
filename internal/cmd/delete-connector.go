package cmd

import (
	"fmt"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	delConnIseHost       string
	delConnIseUsername   string
	delConnIsePassword   string
	delConnConnectorName string
)

var deleteConnectorCmd = &cobra.Command{
	Use:   "delete-connector",
	Short: "Delete a pxGrid Direct connector",
	Long:  `Delete a named pxGrid Direct push or pull connector.`,
	RunE:  runDeleteConnector,
}

func init() {
	rootCmd.AddCommand(deleteConnectorCmd)

	deleteConnectorCmd.Flags().StringVarP(&delConnIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	deleteConnectorCmd.Flags().StringVarP(&delConnIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	deleteConnectorCmd.Flags().StringVarP(&delConnIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	deleteConnectorCmd.Flags().StringVarP(&delConnConnectorName, "connector", "c", "", "pxGrid Direct connector name (required)")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", deleteConnectorCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", deleteConnectorCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", deleteConnectorCmd.Flags().Lookup("password"))

	deleteConnectorCmd.MarkFlagRequired("connector")
}

func runDeleteConnector(cmd *cobra.Command, args []string) error {
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	logger.Verbose("Configuration: host=%s, username=%s, connector=%s",
		host, username, delConnConnectorName)

	if host == "" {
		return fmt.Errorf("ISE host is required (use --host flag or PXCTL_ISE_HOST environment variable)")
	}
	if username == "" {
		return fmt.Errorf("ISE username is required (use --username flag or PXCTL_ISE_USERNAME environment variable)")
	}
	if password == "" {
		return fmt.Errorf("ISE password is required (use --password flag or PXCTL_ISE_PASSWORD environment variable)")
	}

	logger.Verbose("Creating ISE API client for host: %s", host)
	client := api.NewClient(host, username, password)

	fmt.Printf("Deleting connector '%s'...\n", delConnConnectorName)
	logger.Verbose("Attempting to delete connector: %s", delConnConnectorName)

	if err := client.DeleteConnector(delConnConnectorName); err != nil {
		return fmt.Errorf("failed to delete connector: %w", err)
	}

	fmt.Printf("Successfully deleted connector '%s'\n", delConnConnectorName)
	logger.Verbose("Connector deleted successfully")

	return nil
}
