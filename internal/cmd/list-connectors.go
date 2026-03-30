package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	listIseHost     string
	listIseUsername string
	listIsePassword string
)

var listConnectorsCmd = &cobra.Command{
	Use:   "list-connectors",
	Short: "List all pxGrid Direct connector names",
	Long:  `Retrieve and display the names of all pxGrid Direct connectors as a prettified JSON array of strings.`,
	RunE:  runListConnectors,
}

func init() {
	rootCmd.AddCommand(listConnectorsCmd)

	listConnectorsCmd.Flags().StringVarP(&listIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	listConnectorsCmd.Flags().StringVarP(&listIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	listConnectorsCmd.Flags().StringVarP(&listIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", listConnectorsCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", listConnectorsCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", listConnectorsCmd.Flags().Lookup("password"))
}

func runListConnectors(cmd *cobra.Command, args []string) error {
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	logger.Verbose("Configuration: host=%s, username=%s", host, username)

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

	logger.Verbose("Fetching all connector names")
	names, err := client.GetAllConnectorNames()
	if err != nil {
		return fmt.Errorf("failed to retrieve connector names: %w", err)
	}

	logger.Verbose("Retrieved %d connector names", len(names))

	prettyJSON, err := json.MarshalIndent(names, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format connector names as JSON: %w", err)
	}

	fmt.Println(string(prettyJSON))
	return nil
}
