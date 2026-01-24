package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	loadIseHost       string
	loadIseUsername   string
	loadIsePassword   string
	loadConnectorName string
	loadInputFile     string
	loadBatchSize     int // 0 means no limit, only use 5MB constraint
	loadBackoffTime   float64
	loadEmptyCorrelationID bool // Deliberately empty the correlation ID to create bad requests
)

var loadDataCmd = &cobra.Command{
	Use:   "load-data",
	Short: "Load test data into ISE via pxGrid Direct push connector",
	Long: `Load test data into ISE via pxGrid Direct push connector by reading a JSON file
and submitting the data in batches to the bulk API endpoint.`,
	RunE: runLoadData,
}

func init() {
	rootCmd.AddCommand(loadDataCmd)

	loadDataCmd.Flags().StringVarP(&loadIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	loadDataCmd.Flags().StringVarP(&loadIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	loadDataCmd.Flags().StringVarP(&loadIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	loadDataCmd.Flags().StringVarP(&loadConnectorName, "connector", "c", "", "pxGrid Direct push connector name (required)")
	loadDataCmd.Flags().StringVarP(&loadInputFile, "input", "i", "", "Input JSON file containing test data (required)")
	loadDataCmd.Flags().IntVarP(&loadBatchSize, "batch-size", "b", 0, "Number of objects to submit per API call (optional, defaults to 5MB payload limit)")
	loadDataCmd.Flags().Float64VarP(&loadBackoffTime, "backoff", "r", 0.5, "Seconds to wait on 429 rate limit (min: 0.001, max: 120)")
	loadDataCmd.Flags().BoolVar(&loadEmptyCorrelationID, "empty-correlation-id", false, "Deliberately empty the correlation ID field to create bad requests (for testing)")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", loadDataCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", loadDataCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", loadDataCmd.Flags().Lookup("password"))

	loadDataCmd.MarkFlagRequired("connector")
	loadDataCmd.MarkFlagRequired("input")
}

func runLoadData(cmd *cobra.Command, args []string) error {
	// Get values from Viper (checks env vars, flags, and config file)
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	batchSizeMsg := "5MB limit only"
	if loadBatchSize > 0 {
		batchSizeMsg = fmt.Sprintf("%d objects", loadBatchSize)
	}
	logger.Verbose("Configuration: host=%s, username=%s, connector=%s, input=%s, batch-size=%s, backoff=%.3fs, empty-correlation-id=%t",
		host, username, loadConnectorName, loadInputFile, batchSizeMsg, loadBackoffTime, loadEmptyCorrelationID)

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

	// Validate backoff time
	if loadBackoffTime < 0.001 {
		return fmt.Errorf("backoff time must be at least 0.001 seconds, got %.3f", loadBackoffTime)
	}
	if loadBackoffTime > 120 {
		return fmt.Errorf("backoff time must be at most 120 seconds, got %.3f", loadBackoffTime)
	}

	// Read input file
	fmt.Printf("Reading test data from %s...\n", loadInputFile)
	logger.Verbose("Reading input file: %s", loadInputFile)
	fileData, err := os.ReadFile(loadInputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}
	logger.Verbose("Read %d bytes from input file", len(fileData))

	// Parse JSON data
	logger.Verbose("Parsing JSON data")
	var inputData map[string]interface{}
	if err := json.Unmarshal(fileData, &inputData); err != nil {
		return fmt.Errorf("failed to parse JSON input file: %w", err)
	}

	// Extract the data array from the top-level object
	logger.Verbose("Extracting data array from JSON")
	var dataArray []map[string]interface{}
	for _, value := range inputData {
		if arr, ok := value.([]interface{}); ok {
			// Convert []interface{} to []map[string]interface{}
			for _, item := range arr {
				if obj, ok := item.(map[string]interface{}); ok {
					dataArray = append(dataArray, obj)
				}
			}
			break
		}
	}

	if len(dataArray) == 0 {
		return fmt.Errorf("no data found in input file")
	}

	fmt.Printf("Found %d objects to load\n", len(dataArray))
	logger.Verbose("Extracted %d data objects from input file", len(dataArray))

	// Create API client
	logger.Verbose("Creating ISE API client for host: %s", host)
	client := api.NewClient(host, username, password)

	// If --empty-correlation-id flag is set, retrieve connector config and empty the correlation ID field
	if loadEmptyCorrelationID {
		fmt.Printf("Retrieving connector configuration to identify correlation ID field...\n")
		logger.Verbose("Fetching connector configuration to discover correlation identifier field")
		connectorConfig, err := client.GetConnectorConfig(loadConnectorName)
		if err != nil {
			return fmt.Errorf("failed to retrieve connector config: %w", err)
		}

		// Extract correlation identifier from connector config
		correlationIDField := connectorConfig.Response.Connector.Attributes.CorrelationIdentifier

		// Remove the $. prefix if present
		if len(correlationIDField) > 2 && correlationIDField[:2] == "$." {
			correlationIDField = correlationIDField[2:]
		}

		if correlationIDField == "" {
			return fmt.Errorf("no correlation identifier field found in connector configuration")
		}

		fmt.Printf("Emptying correlation ID field '%s' in all objects to create bad requests...\n", correlationIDField)
		logger.Verbose("Emptying correlation ID field '%s' in %d objects", correlationIDField, len(dataArray))

		// Empty the correlation ID field in all objects
		modifiedCount := 0
		for _, obj := range dataArray {
			if _, exists := obj[correlationIDField]; exists {
				obj[correlationIDField] = ""
				modifiedCount++
			}
		}

		logger.Verbose("Modified %d objects by emptying correlation ID field", modifiedCount)
		fmt.Printf("Modified %d objects by emptying correlation ID field '%s'\n", modifiedCount, correlationIDField)
	}

	// Process data in batches with 5MB size limit
	const maxBatchSizeBytes = 5 * 1024 * 1024 // 5MB
	var batches [][]map[string]interface{}
	var currentBatch []map[string]interface{}
	var currentBatchSize int

	for _, obj := range dataArray {
		objJSON, err := json.Marshal(obj)
		if err != nil {
			return fmt.Errorf("failed to marshal object: %w", err)
		}
		objSize := len(objJSON)

		// Check if we need to start a new batch
		shouldStartNewBatch := false

		// Always check 5MB limit
		if currentBatchSize+objSize > maxBatchSizeBytes && len(currentBatch) > 0 {
			shouldStartNewBatch = true
		}

		// If batch size is specified (> 0), also check object count limit
		if loadBatchSize > 0 && len(currentBatch) >= loadBatchSize {
			shouldStartNewBatch = true
		}

		if shouldStartNewBatch {
			batches = append(batches, currentBatch)
			currentBatch = []map[string]interface{}{}
			currentBatchSize = 0
		}

		currentBatch = append(currentBatch, obj)
		currentBatchSize += objSize
	}

	// Add the last batch if it has any objects
	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	totalBatches := len(batches)

	batchConstraint := "5MB limit"
	if loadBatchSize > 0 {
		batchConstraint = fmt.Sprintf("max %d objects or 5MB per batch", loadBatchSize)
	}
	logger.Verbose("Processing %d objects in %d batches (%s)", len(dataArray), totalBatches, batchConstraint)

	// Set up signal handling for SIGINT
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// Structure to hold batch results
	type batchResult struct {
		batchNum     int
		objectCount  int
		size         string
		duration     time.Duration
		status       string
		successCount int
	}

	var results []batchResult
	interrupted := false

	// Process batches
	for batchIndex, batch := range batches {
		// Check for interrupt signal
		select {
		case <-sigChan:
			interrupted = true
			logger.Verbose("Received SIGINT - stopping after current batch")
			goto printResults
		default:
		}

		batchNum := batchIndex + 1

		// Calculate batch size in bytes
		batchJSON, _ := json.Marshal(batch)
		batchSizeBytes := len(batchJSON)
		batchSizeStr := formatBytes(batchSizeBytes)

		logger.Verbose("Batch %d: submitting %d objects (%s)", batchNum, len(batch), batchSizeStr)

		batchStart := time.Now()
		response, err := client.BulkPushDataWithRetry(loadConnectorName, batch, loadBackoffTime)
		batchDuration := time.Since(batchStart)

		if err != nil {
			// Print results accumulated so far before returning error
			goto printResults
		}

		logger.Verbose("Batch %d completed successfully: %s (took %v)", batchNum, response.Status, batchDuration)

		results = append(results, batchResult{
			batchNum:     batchNum,
			objectCount:  len(batch),
			size:         batchSizeStr,
			duration:     batchDuration,
			status:       response.Status,
			successCount: len(batch),
		})
	}

printResults:
	// Flush stderr before printing results to stdout
	os.Stderr.Sync()

	// Print header
	fmt.Printf("\n%-10s %-15s %-15s %-15s %s\n", "Batch", "Objects", "Size", "Duration", "Status")
	fmt.Printf("%-10s %-15s %-15s %-15s %s\n", "-----", "-------", "----", "--------", "------")

	// Print all accumulated results
	successCount := 0
	for _, result := range results {
		fmt.Printf("%-10d %-15d %-15s %-15s %s\n",
			result.batchNum,
			result.objectCount,
			result.size,
			result.duration.Round(time.Millisecond),
			result.status)
		successCount += result.successCount
	}

	if interrupted {
		fmt.Printf("\nInterrupted: Loaded %d objects to connector '%s' before stopping\n", successCount, loadConnectorName)
		logger.Verbose("Load operation interrupted: %d objects successfully loaded", successCount)
	} else {
		fmt.Printf("\nSuccessfully loaded %d objects to connector '%s'\n", successCount, loadConnectorName)
		logger.Verbose("Load operation completed: %d objects successfully loaded", successCount)
	}
	return nil
}
