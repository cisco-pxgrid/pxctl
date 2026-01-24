package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/generator"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	iseHost       string
	iseUsername   string
	isePassword   string
	connectorName string
	numElements   int
	outputFile    string
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate test data for a pxGrid Direct connector",
	Long: `Generate test data for a pxGrid Direct connector by retrieving the connector
configuration from ISE and creating sample data according to the connector's schema.`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringVarP(&iseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	generateCmd.Flags().StringVarP(&iseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	generateCmd.Flags().StringVarP(&isePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	generateCmd.Flags().StringVarP(&connectorName, "connector", "c", "", "pxGrid Direct connector name (required)")
	generateCmd.Flags().IntVarP(&numElements, "count", "n", 10, "Number of random data elements to create")
	generateCmd.Flags().StringVarP(&outputFile, "output", "o", "testdata.json", "Output file name")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", generateCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", generateCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", generateCmd.Flags().Lookup("password"))

	generateCmd.MarkFlagRequired("connector")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	// Get values from Viper (checks env vars, flags, and config file)
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	logger.Verbose("Configuration: host=%s, username=%s, connector=%s, count=%d, output=%s",
		host, username, connectorName, numElements, outputFile)

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

	// Create API client
	logger.Verbose("Creating ISE API client for host: %s", host)
	client := api.NewClient(host, username, password)

	// Retrieve connector configuration
	fmt.Printf("Retrieving connector configuration for '%s' from %s...\n", connectorName, host)
	logger.Verbose("Fetching connector configuration for: %s", connectorName)
	connectorConfig, err := client.GetConnectorConfig(connectorName)
	if err != nil {
		return fmt.Errorf("failed to retrieve connector config: %w", err)
	}

	fmt.Printf("Successfully retrieved connector configuration\n")
	logger.Verbose("Connector type: %s, enabled: %v",
		connectorConfig.Response.Connector.ConnectorType,
		connectorConfig.Response.Connector.Enabled)
	logger.Verbose("Connector has %d attribute mappings",
		len(connectorConfig.Response.Connector.Attributes.AttributeMapping))

	// Generate test data
	fmt.Printf("Generating %d test data elements...\n", numElements)
	logger.Verbose("Starting test data generation with %d elements", numElements)
	testData := generator.GenerateTestData(connectorConfig, numElements)
	logger.Verbose("Test data generation completed")

	// Write to output file
	fmt.Printf("Writing test data to %s...\n", outputFile)
	logger.Verbose("Marshaling test data to JSON")
	data, err := json.MarshalIndent(testData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal test data: %w", err)
	}
	logger.Verbose("JSON size: %d bytes", len(data))

	logger.Verbose("Writing to file: %s", outputFile)
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	fmt.Printf("Successfully generated %d test data elements in %s\n", numElements, outputFile)
	return nil
}
