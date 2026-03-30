package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

var (
	dumpIseHost       string
	dumpIseUsername   string
	dumpIsePassword   string
	dumpConnectorName string
	dumpAsYAML        bool
)

var dumpConnectorCmd = &cobra.Command{
	Use:   "dump-connector",
	Short: "Dump the configuration of a pxGrid Direct connector",
	Long: `Retrieve and display the full configuration of a named pxGrid Direct connector.

By default outputs prettified JSON. Use --yaml to output in the same YAML format
accepted by the create-push-connector command.`,
	RunE: runDumpConnector,
}

func init() {
	rootCmd.AddCommand(dumpConnectorCmd)

	dumpConnectorCmd.Flags().StringVarP(&dumpIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	dumpConnectorCmd.Flags().StringVarP(&dumpIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	dumpConnectorCmd.Flags().StringVarP(&dumpIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	dumpConnectorCmd.Flags().StringVarP(&dumpConnectorName, "connector", "c", "", "pxGrid Direct connector name (required)")
	dumpConnectorCmd.Flags().BoolVar(&dumpAsYAML, "yaml", false, "Output in YAML format compatible with create-push-connector")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", dumpConnectorCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", dumpConnectorCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", dumpConnectorCmd.Flags().Lookup("password"))

	dumpConnectorCmd.MarkFlagRequired("connector")
}

func runDumpConnector(cmd *cobra.Command, args []string) error {
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	logger.Verbose("Configuration: host=%s, username=%s, connector=%s, yaml=%t",
		host, username, dumpConnectorName, dumpAsYAML)

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

	logger.Verbose("Fetching connector configuration for: %s", dumpConnectorName)
	configMap, err := client.GetConnectorConfigRaw(dumpConnectorName)
	if err != nil {
		return fmt.Errorf("failed to retrieve connector config: %w", err)
	}

	if dumpAsYAML {
		// Extract $.response.connector to produce the same format as create-push-connector input
		connector, err := extractConnector(configMap)
		if err != nil {
			return err
		}
		logger.Verbose("Outputting connector config as YAML")
		yamlData, err := yaml.Marshal(connector)
		if err != nil {
			return fmt.Errorf("failed to format connector config as YAML: %w", err)
		}
		fmt.Print(string(yamlData))
	} else {
		prettyJSON, err := json.MarshalIndent(configMap, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format connector config as JSON: %w", err)
		}
		fmt.Println(string(prettyJSON))
	}

	return nil
}

// extractConnector extracts the connector object from $.response.connector
func extractConnector(configMap map[string]interface{}) (map[string]interface{}, error) {
	response, ok := configMap["response"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected config format: missing 'response' field")
	}
	connector, ok := response["connector"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected config format: missing 'response.connector' field")
	}
	return connector, nil
}
