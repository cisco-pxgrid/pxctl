package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

var (
	createPushIseHost     string
	createPushIseUsername string
	createPushIsePassword string
	createPushConfigFile  string
)

var createPushConnectorCmd = &cobra.Command{
	Use:   "create-push-connector",
	Short: "Create a new pxGrid Direct push connector",
	Long: `Create a new pxGrid Direct push connector from a YAML configuration file.

The YAML file defines the connector parameters including name, description,
and attributes. See the sample YAML file in the examples/ directory for reference.

The flexibleUrl section is always cleared for push connectors.`,
	RunE: runCreatePushConnector,
}

func init() {
	rootCmd.AddCommand(createPushConnectorCmd)

	createPushConnectorCmd.Flags().StringVarP(&createPushIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	createPushConnectorCmd.Flags().StringVarP(&createPushIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	createPushConnectorCmd.Flags().StringVarP(&createPushIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	createPushConnectorCmd.Flags().StringVarP(&createPushConfigFile, "config-file", "f", "", "YAML file with push connector configuration (required)")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", createPushConnectorCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", createPushConnectorCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", createPushConnectorCmd.Flags().Lookup("password"))

	createPushConnectorCmd.MarkFlagRequired("config-file")
}

func runCreatePushConnector(cmd *cobra.Command, args []string) error {
	// Get values from Viper (checks env vars, flags, and config file)
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	logger.Verbose("Configuration: host=%s, username=%s, config-file=%s",
		host, username, createPushConfigFile)

	// Validate required parameters
	if host == "" {
		return fmt.Errorf("ISE host is required (use --host flag or PXCTL_ISE_HOST environment variable)")
	}
	if username == "" {
		return fmt.Errorf("ISE username is required (use --username flag or PXCTL_ISE_USERNAME environment variable)")
	}
	if password == "" {
		return fmt.Errorf("ISE password is required (use --password flag or PXCTL_ISE_PASSWORD environment variable)")
	}

	// Read and parse the YAML configuration file
	fmt.Printf("Reading connector configuration from '%s'...\n", createPushConfigFile)
	logger.Verbose("Reading YAML config file: %s", createPushConfigFile)

	yamlData, err := os.ReadFile(createPushConfigFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	logger.Verbose("Read %d bytes from config file", len(yamlData))

	// Parse YAML into a generic map to preserve all fields
	var yamlConfig map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &yamlConfig); err != nil {
		return fmt.Errorf("failed to parse YAML config file: %w", err)
	}

	// Build the connector config payload
	// The YAML should define the connector fields directly; we wrap it in the
	// required {"connector": {...}} structure for the API
	connectorConfig := convertYAMLToJSON(yamlConfig)

	// Apply push connector defaults to match the required API payload structure
	if connMap, ok := connectorConfig.(map[string]interface{}); ok {
		connMap["connectorType"] = "urlpusher"
		logger.Verbose("Set connectorType to 'urlpusher'")

		// Set defaults for fields not provided in YAML
		setDefault(connMap, "description", nil)
		setDefault(connMap, "coaType", "nocoa")
		setDefault(connMap, "protocol", "http")
		setDefault(connMap, "skipCertificateValidations", false)
		setDefault(connMap, "enabled", true)
		setDefault(connMap, "fullsyncSchedule", nil)
		setDefault(connMap, "deltasyncSchedule", nil)
		setDefault(connMap, "additionalProperties", nil)

		// flexibleUrl MUST have no content for push connectors
		connMap["flexibleUrl"] = map[string]interface{}{
			"bulk":        nil,
			"incremental": nil,
		}
		logger.Verbose("Set flexibleUrl with null bulk/incremental")

		// url section with defaults
		if _, ok := connMap["url"]; !ok {
			connMap["url"] = map[string]interface{}{
				"bulkUrl":            " ",
				"incrementalUrl":     nil,
				"authenticationType": "basic",
				"userName":           nil,
				"password":           nil,
			}
			logger.Verbose("Set default url section")
		}

		// groupArray with defaults
		if _, ok := connMap["groupArray"]; !ok {
			connMap["groupArray"] = []interface{}{
				map[string]interface{}{
					"GroupName":       "Super Admin",
					"GroupPermission": "write",
				},
				map[string]interface{}{
					"GroupName":       "ERS Admin",
					"GroupPermission": "write",
				},
			}
			logger.Verbose("Set default groupArray")
		}

		if name, ok := connMap["connectorName"].(string); ok {
			logger.Verbose("Connector name: %s", name)
		}
	}

	payload := map[string]interface{}{
		"connector": connectorConfig,
	}

	// Log the payload in verbose mode
	if logger.IsVerbose() {
		prettyJSON, err := json.MarshalIndent(payload, "", "  ")
		if err == nil {
			logger.Verbose("Request payload:\n%s", string(prettyJSON))
		}
	}

	// Create API client
	logger.Verbose("Creating ISE API client for host: %s", host)
	client := api.NewClient(host, username, password)

	// Create the connector
	connectorName := "unknown"
	if connMap, ok := connectorConfig.(map[string]interface{}); ok {
		if name, ok := connMap["connectorName"].(string); ok {
			connectorName = name
		}
	}

	fmt.Printf("Creating push connector '%s' on %s...\n", connectorName, host)
	logger.Verbose("Submitting connector creation request")

	err = client.CreateConnector(payload)
	if err != nil {
		// On failure, display the URL and JSON payload
		postURL := fmt.Sprintf("https://%s/api/v1/pxgrid-direct/connector-config", host)
		fmt.Printf("\nError creating connector. Details:\n")
		fmt.Printf("URL: %s\n", postURL)
		fmt.Printf("\nJSON Payload:\n")
		prettyJSON, jsonErr := json.MarshalIndent(payload, "", "  ")
		if jsonErr == nil {
			fmt.Printf("%s\n", string(prettyJSON))
		} else {
			fmt.Printf("(Failed to prettify JSON: %v)\n", jsonErr)
		}
		return fmt.Errorf("failed to create push connector: %w", err)
	}

	fmt.Printf("Successfully created push connector '%s'\n", connectorName)
	logger.Verbose("Push connector created successfully")

	return nil
}

// setDefault sets a key in the map only if it is not already present.
func setDefault(m map[string]interface{}, key string, value interface{}) {
	if _, ok := m[key]; !ok {
		m[key] = value
	}
}

// convertYAMLToJSON recursively converts a YAML-parsed value to JSON-compatible types.
// The go.yaml.in/yaml/v3 library unmarshals maps as map[string]interface{} when
// unmarshaling into interface{}, but nested values may need conversion.
func convertYAMLToJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(val))
		for k, v := range val {
			result[k] = convertYAMLToJSON(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = convertYAMLToJSON(v)
		}
		return result
	default:
		return val
	}
}
