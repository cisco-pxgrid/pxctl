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
	"github.com/einarnn/pxctl/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	loadIseHost            string
	loadIseUsername        string
	loadIsePassword        string
	loadConnectorName      string
	loadInputFile          string
	loadBatchSize          int // 0 means no limit, only use 5MB constraint
	loadBackoffTime        float64
	loadEmptyCorrelationID bool // Deliberately empty the correlation ID to create bad requests
	loadTUI                bool
	load429Adaptive        bool
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
	loadDataCmd.Flags().BoolVar(&loadTUI, "tui", false, "Show an interactive progress UI (mutually exclusive with --verbose)")
	loadDataCmd.Flags().BoolVar(&load429Adaptive, "429-adaptive", false, "Adapt the 429 retry timer to observed batch duration")

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
	fmt.Printf("Scanning test data from %s...\n", loadInputFile)
	objectCount, err := countJSONObjects(loadInputFile)
	if err != nil {
		return fmt.Errorf("failed to scan JSON input file: %w", err)
	}
	if objectCount == 0 {
		return fmt.Errorf("no data found in input file")
	}
	fmt.Printf("Found %d objects to load\n", objectCount)
	logger.Verbose("Pre-scanned %d data objects from input file", objectCount)

	// Create API client
	logger.Verbose("Creating ISE API client for host: %s", host)
	client := api.NewClient(host, username, password)

	correlationIDField := ""
	// If --empty-correlation-id flag is set, retrieve connector config and empty the correlation ID field
	if loadEmptyCorrelationID {
		fmt.Printf("Retrieving connector configuration to identify correlation ID field...\n")
		logger.Verbose("Fetching connector configuration to discover correlation identifier field")
		connectorConfig, err := client.GetConnectorConfig(loadConnectorName)
		if err != nil {
			return fmt.Errorf("failed to retrieve connector config: %w", err)
		}

		// Extract correlation identifier from connector config
		correlationIDField = connectorConfig.Response.Connector.Attributes.CorrelationIdentifier

		// Remove the $. prefix if present
		if len(correlationIDField) > 2 && correlationIDField[:2] == "$." {
			correlationIDField = correlationIDField[2:]
		}

		if correlationIDField == "" {
			return fmt.Errorf("no correlation identifier field found in connector configuration")
		}

		fmt.Printf("Emptying correlation ID field '%s' in all objects to create bad requests...\n", correlationIDField)
		logger.Verbose("Will empty correlation ID field '%s' while streaming objects", correlationIDField)
	}

	// Process data in batches with 5MB size limit. The second pass streams one
	// object at a time and never retains the complete input document.
	const maxBatchSizeBytes = 5 * 1024 * 1024 // 5MB
	batchConstraint := "5MB limit"
	if loadBatchSize > 0 {
		batchConstraint = fmt.Sprintf("max %d objects or 5MB per batch", loadBatchSize)
	}
	logger.Verbose("Processing %d objects in streamed batches (%s)", objectCount, batchConstraint)
	stream, err := newJSONObjectStream(loadInputFile)
	if err != nil {
		return fmt.Errorf("failed to open JSON input stream: %w", err)
	}
	defer stream.Close()
	var ui *tui.Reporter
	if loadTUI {
		ui = tui.Start(objectCount)
		logger.SetEventSink(ui.Log)
		logger.SetRetrySink(func(seconds float64) { ui.SetRetryTimer(time.Duration(seconds * float64(time.Second))) })
		ui.SetMainRetryTimer(time.Duration(loadBackoffTime * float64(time.Second)))
	}
	var uiInterrupt <-chan struct{}
	if ui != nil {
		uiInterrupt = ui.Interrupt()
	}
	adaptiveBackoff := loadBackoffTime
	stopUI := func() {
		if ui != nil {
			logger.SetEventSink(nil)
			logger.SetRetrySink(nil)
			ui.Close()
			ui = nil
		}
	}

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
	var streamErr error
	var pending map[string]interface{}
	pendingSize := 0
	hasPending := false
	batchIndex := 0
	var pacingDuration time.Duration

	// Process batches
	for {
		// Check for interrupt signal
		select {
		case <-sigChan:
			interrupted = true
			logger.Verbose("Received SIGINT - stopping after current batch")
			goto printResults
		case <-uiInterrupt:
			interrupted = true
			logger.Verbose("Received CTRL-C - stopping after current batch")
			goto printResults
		default:
		}

		batch := make([]map[string]interface{}, 0)
		batchSizeBytes := 0
		for {
			var obj map[string]interface{}
			var objSize int
			if hasPending {
				obj, objSize, hasPending = pending, pendingSize, false
			} else {
				var done bool
				obj, done, streamErr = stream.Next()
				if streamErr != nil {
					if ui != nil {
						ui.Failure(fmt.Sprintf("Input stream failed: %v", streamErr))
					}
					goto printResults
				}
				if done {
					break
				}
				if correlationIDField != "" {
					if _, exists := obj[correlationIDField]; exists {
						obj[correlationIDField] = ""
					}
				}
				encoded, err := json.Marshal(obj)
				if err != nil {
					streamErr = fmt.Errorf("failed to marshal object: %w", err)
					goto printResults
				}
				objSize = len(encoded)
			}
			if len(batch) > 0 && (batchSizeBytes+objSize > maxBatchSizeBytes || (loadBatchSize > 0 && len(batch) >= loadBatchSize)) {
				pending, pendingSize, hasPending = obj, objSize, true
				break
			}
			batch = append(batch, obj)
			batchSizeBytes += objSize
		}
		if len(batch) == 0 {
			break
		}
		if pacingDuration > 0 {
			if ui != nil {
				ui.Log(fmt.Sprintf("Pacing next batch for %v", pacingDuration.Round(time.Millisecond)))
			}
			if !waitForPacing(pacingDuration, sigChan, uiInterrupt) {
				interrupted = true
				goto printResults
			}
			pacingDuration = 0
		}
		batchNum := batchIndex + 1
		batchIndex++

		// Calculate batch size in bytes
		batchJSON, _ := json.Marshal(batch)
		batchSizeBytes = len(batchJSON)
		batchSizeStr := formatBytes(batchSizeBytes)

		logger.Verbose("Batch %d: submitting %d objects (%s)", batchNum, len(batch), batchSizeStr)
		if ui != nil {
			ui.Log(fmt.Sprintf("Submitting batch %d (%d records)", batchNum, len(batch)))
		}

		batchStart := time.Now()
		had429 := false
		requestMainDuration := time.Duration(adaptiveBackoff * float64(time.Second))
		if ui != nil {
			ui.SetRetryTimer(time.Duration(adaptiveBackoff * float64(time.Second)))
		}
		response, err := client.BulkPushDataWithRetryAdaptive(loadConnectorName, batch, adaptiveBackoff, load429Adaptive, func() { had429 = true })
		batchDuration := time.Since(batchStart)
		if load429Adaptive {
			pacingDuration = requestMainDuration - batchDuration
			if pacingDuration < 0 {
				pacingDuration = 0
			}
			adaptiveBackoff = adaptiveRetryDelay(adaptiveBackoff, batchDuration, !had429)
			if ui != nil {
				ui.SetMainRetryTimer(time.Duration(adaptiveBackoff * float64(time.Second)))
			}
		}

		if err != nil {
			if ui != nil {
				ui.Failure(fmt.Sprintf("Batch %d failed: %v", batchNum, err))
			}
			// Print results accumulated so far before returning error
			goto printResults
		}

		logger.Verbose("Batch %d completed successfully: %s (took %v)", batchNum, response.Status, batchDuration)
		if ui != nil {
			ui.Success(len(batch), fmt.Sprintf("Batch %d succeeded (%s, %v)", batchNum, response.Status, batchDuration.Round(time.Millisecond)))
		}

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
	stopUI()
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
	return streamErr
}
