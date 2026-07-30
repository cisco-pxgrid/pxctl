package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/einarnn/pxctl/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	loadPutIseHost       string
	loadPutIseUsername   string
	loadPutIsePassword   string
	loadPutConnectorName string
	loadPutUniqueIDField string
	loadPutInputFile     string
	loadPutConcurrency   int
	loadPutDelay         float64
	loadPutBackoffTime   float64
	loadPutTUI           bool
	loadPut429Adaptive   bool
)

var loadDataPutCmd = &cobra.Command{
	Use:   "load-data-put",
	Short: "Load test data into ISE via pxGrid Direct push connector using PUT",
	Long: `Load test data into ISE via pxGrid Direct push connector using concurrent PUT operations.
Each object is submitted individually to /api/v1/pxgrid-direct/push/{ConnectorName}/{uniqueId}.`,
	RunE: runLoadDataPut,
}

func init() {
	rootCmd.AddCommand(loadDataPutCmd)

	loadDataPutCmd.Flags().StringVarP(&loadPutIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	loadDataPutCmd.Flags().StringVarP(&loadPutIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	loadDataPutCmd.Flags().StringVarP(&loadPutIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	loadDataPutCmd.Flags().StringVarP(&loadPutConnectorName, "connector", "c", "", "pxGrid Direct push connector name (required)")
	loadDataPutCmd.Flags().StringVarP(&loadPutUniqueIDField, "unique-id-field", "f", "", "JSON property name to use for uniqueId (optional, auto-discovered from connector if not specified)")
	loadDataPutCmd.Flags().StringVarP(&loadPutInputFile, "input", "i", "", "Input JSON file containing test data (required)")
	loadDataPutCmd.Flags().IntVarP(&loadPutConcurrency, "concurrency", "n", 10, "Number of concurrent PUT requests (default: 10)")
	loadDataPutCmd.Flags().Float64VarP(&loadPutDelay, "delay", "d", 0.0, "Inter-object delay in seconds (min: 0.0, max: 5.0)")
	loadDataPutCmd.Flags().Float64VarP(&loadPutBackoffTime, "backoff", "r", 0.5, "Seconds to wait on 429 rate limit (min: 0.001, max: 120)")
	loadDataPutCmd.Flags().BoolVar(&loadPutTUI, "tui", false, "Show an interactive progress UI (mutually exclusive with --verbose)")
	loadDataPutCmd.Flags().BoolVar(&loadPut429Adaptive, "429-adaptive", false, "Adapt the 429 retry timer to observed request duration")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", loadDataPutCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", loadDataPutCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", loadDataPutCmd.Flags().Lookup("password"))

	loadDataPutCmd.MarkFlagRequired("connector")
	loadDataPutCmd.MarkFlagRequired("input")
}

func runLoadDataPut(cmd *cobra.Command, args []string) error {
	// Get values from Viper (checks env vars, flags, and config file)
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	logger.Verbose("Configuration: host=%s, username=%s, connector=%s, unique-id-field=%s, input=%s, concurrency=%d, delay=%.3fs, backoff=%.3fs",
		host, username, loadPutConnectorName, loadPutUniqueIDField, loadPutInputFile, loadPutConcurrency, loadPutDelay, loadPutBackoffTime)

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

	// Validate delay time
	if loadPutDelay < 0.0 {
		return fmt.Errorf("delay must be at least 0.0 seconds, got %.3f", loadPutDelay)
	}
	if loadPutDelay > 5.0 {
		return fmt.Errorf("delay must be at most 5.0 seconds, got %.3f", loadPutDelay)
	}

	// Validate backoff time
	if loadPutBackoffTime < 0.001 {
		return fmt.Errorf("backoff time must be at least 0.001 seconds, got %.3f", loadPutBackoffTime)
	}
	if loadPutBackoffTime > 120 {
		return fmt.Errorf("backoff time must be at most 120 seconds, got %.3f", loadPutBackoffTime)
	}

	// Validate concurrency
	if loadPutConcurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1, got %d", loadPutConcurrency)
	}

	// Pre-scan the array to get the total without loading the file into memory.
	fmt.Printf("Scanning test data from %s...\n", loadPutInputFile)
	objectCount, err := countJSONObjects(loadPutInputFile)
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

	// Determine the unique identifier field name
	var uniqueIDField string
	if loadPutUniqueIDField != "" {
		// Use the user-provided field name
		uniqueIDField = loadPutUniqueIDField
		logger.Verbose("Using user-specified unique identifier field: %s", uniqueIDField)
	} else {
		// Auto-discover from connector configuration
		fmt.Printf("Retrieving connector configuration for '%s' from %s...\n", loadPutConnectorName, host)
		logger.Verbose("Fetching connector configuration to discover unique identifier field")
		connectorConfig, err := client.GetConnectorConfig(loadPutConnectorName)
		if err != nil {
			return fmt.Errorf("failed to retrieve connector config: %w", err)
		}

		// Extract unique identifier from connector config
		uniqueIDField = connectorConfig.Response.Connector.Attributes.UniqueIdentifier

		// Remove the $. prefix if present
		if len(uniqueIDField) > 2 && uniqueIDField[:2] == "$." {
			uniqueIDField = uniqueIDField[2:]
		}

		logger.Verbose("Discovered unique identifier field from connector config: %s", uniqueIDField)

		if uniqueIDField == "" {
			return fmt.Errorf("no unique identifier field found in connector configuration")
		}
		fmt.Printf("Auto-discovered unique identifier field: %s\n", uniqueIDField)
	}

	logger.Verbose("Processing %d objects with concurrency factor of %d", objectCount, loadPutConcurrency)
	var ui *tui.Reporter
	if loadPutTUI {
		ui = tui.Start(objectCount)
		logger.SetEventSink(ui.Log)
		logger.SetRetrySink(func(seconds float64) { ui.SetRetryTimer(time.Duration(seconds * float64(time.Second))) })
		ui.SetMainRetryTimer(time.Duration(loadPutBackoffTime * float64(time.Second)))
	}
	var uiInterrupt <-chan struct{}
	if ui != nil {
		uiInterrupt = ui.Interrupt()
	}

	// Set up signal handling for SIGINT
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// Structure to hold individual results
	type putResult struct {
		objectNum int
		uniqueID  string
		duration  time.Duration
		status    string
		err       error
	}

	results := make([]putResult, 0, objectCount)
	var resultsMutex sync.Mutex
	var interrupted atomic.Bool
	var processedCount atomic.Int64
	var adaptiveBackoff atomic.Int64
	adaptiveBackoff.Store(int64(loadPutBackoffTime * float64(time.Second)))

	// Create work channel
	type workItem struct {
		index int
		obj   map[string]interface{}
	}
	workChan := make(chan workItem, loadPutConcurrency)
	stopStream := make(chan struct{})
	producerDone := make(chan struct{})
	stream, err := newJSONObjectStream(loadPutInputFile)
	if err != nil {
		return fmt.Errorf("failed to open JSON input stream: %w", err)
	}
	streamErr := make(chan error, 1)
	var streamFailure error
	go func() {
		defer close(producerDone)
		defer stream.Close()
		defer close(workChan)
		for index := 0; ; index++ {
			obj, done, err := stream.Next()
			if err != nil {
				streamErr <- err
				return
			}
			if done {
				return
			}
			select {
			case workChan <- workItem{index: index, obj: obj}:
			case <-stopStream:
				return
			}
		}
	}()

	// Start worker goroutines
	var wg sync.WaitGroup
	for w := 0; w < loadPutConcurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for work := range workChan {
				// Check for interrupt
				if interrupted.Load() {
					return
				}
				select {
				case <-uiInterrupt:
					interrupted.Store(true)
					return
				default:
				}

				objNum := work.index + 1
				obj := work.obj

				// Extract unique identifier from the configured field
				var uniqueID string
				if idVal, ok := obj[uniqueIDField]; ok {
					if strVal, ok := idVal.(string); ok {
						uniqueID = strVal
					}
				}

				if uniqueID == "" {
					logger.Verbose("Worker %d: object %d missing unique identifier field '%s' - skipping", workerID, objNum, uniqueIDField)
					resultsMutex.Lock()
					results = append(results, putResult{
						objectNum: objNum,
						uniqueID:  "<missing>",
						duration:  0,
						status:    "error",
						err:       fmt.Errorf("missing unique identifier field: %s", uniqueIDField),
					})
					resultsMutex.Unlock()
					if ui != nil {
						ui.Failure(fmt.Sprintf("Object %d skipped: missing unique identifier field %q", objNum, uniqueIDField))
					}
					processedCount.Add(1)
					continue
				}

				logger.Verbose("Worker %d: submitting object %d (ID: %s)", workerID, objNum, uniqueID)

				startTime := time.Now()
				backoff := loadPutBackoffTime
				if loadPut429Adaptive {
					backoff = float64(adaptiveBackoff.Load()) / float64(time.Second)
				}
				had429 := false
				if ui != nil {
					ui.SetRetryTimer(time.Duration(backoff * float64(time.Second)))
				}
				response, err := client.PutDataAdaptive(loadPutConnectorName, uniqueID, obj, backoff, loadPut429Adaptive, func() { had429 = true })
				duration := time.Since(startTime)
				if loadPut429Adaptive && err == nil {
					current := float64(adaptiveBackoff.Load()) / float64(time.Second)
					adaptiveBackoff.Store(int64(adaptiveRetryDelay(current, duration, !had429) * float64(time.Second)))
					if ui != nil {
						ui.SetMainRetryTimer(time.Duration(adaptiveBackoff.Load()))
					}
				}

				result := putResult{
					objectNum: objNum,
					uniqueID:  uniqueID,
					duration:  duration,
					err:       err,
				}

				if err == nil {
					result.status = response.Status
					logger.Verbose("Worker %d: object %d completed successfully (took %v)", workerID, objNum, duration)
					if ui != nil {
						ui.Success(1, fmt.Sprintf("Object %d succeeded (%s, %v)", objNum, response.Status, duration.Round(time.Millisecond)))
					}
				} else {
					result.status = "error"
					logger.Verbose("Worker %d: object %d failed: %v", workerID, objNum, err)
					if ui != nil {
						ui.Failure(fmt.Sprintf("Object %d failed: %v", objNum, err))
					}
				}

				resultsMutex.Lock()
				results = append(results, result)
				resultsMutex.Unlock()

				processedCount.Add(1)
				if loadPut429Adaptive && err == nil {
					pacingDuration := time.Duration(backoff*float64(time.Second)) - duration
					if pacingDuration > 0 {
						if ui != nil {
							ui.Log(fmt.Sprintf("Pacing worker %d for %v before the next request", workerID, pacingDuration.Round(time.Millisecond)))
						}
						if !waitForPacing(pacingDuration, sigChan, uiInterrupt) {
							interrupted.Store(true)
							return
						}
					}
				}

				// Apply inter-object delay if configured
				if loadPutDelay > 0.0 {
					delayDuration := time.Duration(loadPutDelay * float64(time.Second))
					time.Sleep(delayDuration)
				}

				// Check for interrupt after each operation
				select {
				case <-sigChan:
					interrupted.Store(true)
					logger.Verbose("Worker %d: received SIGINT - stopping", workerID)
					return
				case <-uiInterrupt:
					interrupted.Store(true)
					logger.Verbose("Worker %d: received CTRL-C - stopping", workerID)
					return
				default:
				}
			}
		}(w)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(stopStream)
	<-producerDone
	select {
	case err := <-streamErr:
		if err != nil {
			streamFailure = fmt.Errorf("failed while streaming JSON input: %w", err)
			logger.Verbose("Input stream failed: %v", err)
		}
	default:
	}
	if ui != nil {
		logger.SetEventSink(nil)
		logger.SetRetrySink(nil)
		ui.Close()
		ui = nil
	}

	// Flush stderr before printing results to stdout
	os.Stderr.Sync()

	// Calculate statistics
	resultsMutex.Lock()
	successCount := 0
	errorCount := 0
	var totalDuration time.Duration
	var minDuration, maxDuration time.Duration

	for i, result := range results {
		if result.err == nil {
			successCount++
		} else {
			errorCount++
		}
		totalDuration += result.duration

		if i == 0 || result.duration < minDuration {
			minDuration = result.duration
		}
		if i == 0 || result.duration > maxDuration {
			maxDuration = result.duration
		}
	}
	resultsMutex.Unlock()

	// Print summary statistics
	fmt.Printf("\n%-20s %d\n", "Total Objects:", objectCount)
	fmt.Printf("%-20s %d\n", "Processed:", processedCount.Load())
	fmt.Printf("%-20s %d\n", "Successful:", successCount)
	if errorCount > 0 {
		fmt.Printf("%-20s %d\n", "Errors:", errorCount)
	}

	if len(results) > 0 {
		avgDuration := totalDuration / time.Duration(len(results))
		fmt.Printf("\n%-20s %s\n", "Min Duration:", minDuration.Round(time.Millisecond))
		fmt.Printf("%-20s %s\n", "Max Duration:", maxDuration.Round(time.Millisecond))
		fmt.Printf("%-20s %s\n", "Avg Duration:", avgDuration.Round(time.Millisecond))
	}

	if interrupted.Load() {
		fmt.Printf("\nInterrupted: Loaded %d objects to connector '%s' before stopping\n", successCount, loadPutConnectorName)
		logger.Verbose("Load operation interrupted: %d objects successfully loaded", successCount)
	} else {
		fmt.Printf("\nSuccessfully loaded %d objects to connector '%s'\n", successCount, loadPutConnectorName)
		logger.Verbose("Load operation completed: %d objects successfully loaded", successCount)
	}

	return streamFailure
}
