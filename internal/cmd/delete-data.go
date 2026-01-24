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
	deleteIseHost       string
	deleteIseUsername   string
	deleteIsePassword   string
	deleteConnectorName string
	deleteBatchSize     int // 0 means no limit, only use 5MB constraint
	deleteBackoffTime   float64
)

var deleteDataCmd = &cobra.Command{
	Use:   "delete-data",
	Short: "Delete all objects from a pxGrid Direct push connector",
	Long: `Delete all objects held by a pxGrid Direct push connector by retrieving all objects
and submitting delete operations in batches to the bulk API endpoint.`,
	RunE: runDeleteData,
}

func init() {
	rootCmd.AddCommand(deleteDataCmd)

	deleteDataCmd.Flags().StringVarP(&deleteIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	deleteDataCmd.Flags().StringVarP(&deleteIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	deleteDataCmd.Flags().StringVarP(&deleteIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	deleteDataCmd.Flags().StringVarP(&deleteConnectorName, "connector", "c", "", "pxGrid Direct push connector name (required)")
	deleteDataCmd.Flags().IntVarP(&deleteBatchSize, "batch-size", "b", 0, "Number of objects to delete per API call (optional, defaults to 5MB payload limit)")
	deleteDataCmd.Flags().Float64VarP(&deleteBackoffTime, "backoff", "r", 0.5, "Seconds to wait on 429 rate limit (min: 0.001, max: 120)")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", deleteDataCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", deleteDataCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", deleteDataCmd.Flags().Lookup("password"))

	deleteDataCmd.MarkFlagRequired("connector")
}

func runDeleteData(cmd *cobra.Command, args []string) error {
	// Get values from Viper (checks env vars, flags, and config file)
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	batchSizeMsg := "5MB limit only"
	if deleteBatchSize > 0 {
		batchSizeMsg = fmt.Sprintf("%d objects", deleteBatchSize)
	}
	logger.Verbose("Configuration: host=%s, username=%s, connector=%s, batch-size=%s, backoff=%.3fs",
		host, username, deleteConnectorName, batchSizeMsg, deleteBackoffTime)

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
	if deleteBackoffTime < 0.001 {
		return fmt.Errorf("backoff time must be at least 0.001 seconds, got %.3f", deleteBackoffTime)
	}
	if deleteBackoffTime > 120 {
		return fmt.Errorf("backoff time must be at most 120 seconds, got %.3f", deleteBackoffTime)
	}

	// Create API client
	logger.Verbose("Creating ISE API client for host: %s", host)
	client := api.NewClient(host, username, password)

	// Retrieve connector configuration to get unique identifier field
	fmt.Printf("Retrieving connector configuration for '%s' from %s...\n", deleteConnectorName, host)
	logger.Verbose("Fetching connector configuration to discover unique identifier field")
	connectorConfig, err := client.GetConnectorConfig(deleteConnectorName)
	if err != nil {
		return fmt.Errorf("failed to retrieve connector config: %w", err)
	}

	// Extract unique identifier field name
	uniqueIDField := connectorConfig.Response.Connector.Attributes.UniqueIdentifier
	if len(uniqueIDField) > 2 && uniqueIDField[:2] == "$." {
		uniqueIDField = uniqueIDField[2:]
	}

	if uniqueIDField == "" {
		return fmt.Errorf("no unique identifier field found in connector configuration")
	}
	logger.Verbose("Using unique identifier field: %s", uniqueIDField)

	// Retrieve all objects from the connector
	fmt.Printf("Retrieving all objects from connector '%s'...\n", deleteConnectorName)
	logger.Verbose("Fetching all objects from connector: %s", deleteConnectorName)
	allObjects, err := client.GetAllPushConnectorObjects(deleteConnectorName)
	if err != nil {
		return fmt.Errorf("failed to retrieve objects from connector: %w", err)
	}

	if len(allObjects) == 0 {
		fmt.Printf("No objects found in connector '%s'\n", deleteConnectorName)
		return nil
	}

	fmt.Printf("Found %d objects to delete\n", len(allObjects))
	logger.Verbose("Retrieved %d objects from connector", len(allObjects))

	// Extract only the unique identifiers to minimize memory usage
	logger.Verbose("Extracting unique identifiers from objects to minimize memory usage")
	var minimalObjects []map[string]interface{}
	for i, obj := range allObjects {
		if uniqueID, ok := obj[uniqueIDField]; ok {
			// Create minimal object with only the unique identifier
			minimalObj := map[string]interface{}{
				uniqueIDField: uniqueID,
			}
			minimalObjects = append(minimalObjects, minimalObj)
		} else {
			logger.Verbose("Warning: object %d missing unique identifier field '%s' - skipping", i+1, uniqueIDField)
		}
	}

	// Clear the original objects array to free memory
	allObjects = nil

	if len(minimalObjects) == 0 {
		return fmt.Errorf("no objects with valid unique identifiers found")
	}

	logger.Verbose("Extracted %d unique identifiers, ready for deletion", len(minimalObjects))

	// Process data in batches with 5MB size limit
	const maxBatchSizeBytes = 5 * 1024 * 1024 // 5MB
	var batches [][]map[string]interface{}
	var currentBatch []map[string]interface{}
	var currentBatchSize int

	for _, obj := range minimalObjects {
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
		if deleteBatchSize > 0 && len(currentBatch) >= deleteBatchSize {
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
	if deleteBatchSize > 0 {
		batchConstraint = fmt.Sprintf("max %d objects or 5MB per batch", deleteBatchSize)
	}
	logger.Verbose("Processing %d objects in %d batches (%s)", len(minimalObjects), totalBatches, batchConstraint)

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

		logger.Verbose("Batch %d: deleting %d objects (%s)", batchNum, len(batch), batchSizeStr)

		batchStart := time.Now()
		response, err := client.BulkDeleteData(deleteConnectorName, batch, deleteBackoffTime)
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
		fmt.Printf("\nInterrupted: Deleted %d objects from connector '%s' before stopping\n", successCount, deleteConnectorName)
		logger.Verbose("Delete operation interrupted: %d objects successfully deleted", successCount)
	} else {
		fmt.Printf("\nSuccessfully deleted %d objects from connector '%s'\n", successCount, deleteConnectorName)
		logger.Verbose("Delete operation completed: %d objects successfully deleted", successCount)
	}
	return nil
}
