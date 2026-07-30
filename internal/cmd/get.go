package cmd

import (
	"bufio"
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
	getIseHost       string
	getIseUsername   string
	getIsePassword   string
	getConnectorName string
	getPageSize      int
	getStartPage     int
	getLimit         int
	getOutputFile    string
	getTUI           bool
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Retrieve objects from a pxGrid Direct push connector",
	Long: `Retrieve all objects from a pxGrid Direct push connector by iterating its
paginated GET endpoint. The output always contains one top-level data property.`,
	RunE: runGet,
}

func init() {
	rootCmd.AddCommand(getCmd)

	getCmd.Flags().StringVarP(&getIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	getCmd.Flags().StringVarP(&getIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	getCmd.Flags().StringVarP(&getIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	getCmd.Flags().StringVarP(&getConnectorName, "connector", "c", "", "pxGrid Direct push connector name (required)")
	getCmd.Flags().IntVar(&getPageSize, "size", 1000, "Number of objects to request per page")
	getCmd.Flags().IntVar(&getStartPage, "page", 0, "Page number to start retrieving from")
	getCmd.Flags().IntVar(&getLimit, "limit", 0, "Stop after this many objects (0 means no limit)")
	getCmd.Flags().StringVarP(&getOutputFile, "output", "o", "", "Output JSON file (default: stdout)")
	getCmd.Flags().BoolVar(&getTUI, "tui", false, "Show live GET latency and object statistics")

	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")
	viper.BindPFlag("ise.host", getCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", getCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", getCmd.Flags().Lookup("password"))

	getCmd.MarkFlagRequired("connector")
}

func runGet(cmd *cobra.Command, args []string) error {
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")
	if host == "" {
		return fmt.Errorf("ISE host is required (use --host flag or PXCTL_ISE_HOST environment variable)")
	}
	if username == "" {
		return fmt.Errorf("ISE username is required (use --username flag or PXCTL_ISE_USERNAME environment variable)")
	}
	if password == "" {
		return fmt.Errorf("ISE password is required (use --password flag or PXCTL_ISE_PASSWORD environment variable)")
	}
	if getPageSize < 1 {
		return fmt.Errorf("size must be at least 1, got %d", getPageSize)
	}
	if getStartPage < 0 {
		return fmt.Errorf("page must not be negative, got %d", getStartPage)
	}
	if getLimit < 0 {
		return fmt.Errorf("limit must not be negative, got %d", getLimit)
	}

	client := api.NewClient(host, username, password)
	var ui *tui.Reporter
	if getTUI {
		ui = tui.StartRead()
		logger.SetEventSink(ui.Log)
	}
	stopUI := func() {
		if ui != nil {
			logger.SetEventSink(nil)
			ui.Close()
			ui = nil
		}
	}
	defer stopUI()
	var uiInterrupt <-chan struct{}
	if ui != nil {
		uiInterrupt = ui.Interrupt()
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)
	var objects []map[string]interface{}
	var stdout *bufio.Writer
	firstObject := true
	totalObjects := 0
	if getOutputFile == "" {
		stdout = bufio.NewWriter(os.Stdout)
		if _, err := stdout.WriteString(`{"data":[`); err != nil {
			return fmt.Errorf("failed to start stdout output: %w", err)
		}
		if err := stdout.Flush(); err != nil {
			return fmt.Errorf("failed to flush stdout output: %w", err)
		}
	}
	page := getStartPage
	pageCount := 0
	var totalLatency time.Duration
	var requestLatencies []time.Duration
	var requestPages []int
	defer func() {
		if !getTUI {
			return
		}
		stopUI()
		printGetSummary(requestPages, requestLatencies, totalObjects)
	}()
	for {
		select {
		case <-sigChan:
			return fmt.Errorf("GET operation interrupted")
		case <-uiInterrupt:
			return fmt.Errorf("GET operation interrupted")
		default:
		}
		logger.Verbose("Fetching connector %q page %d with size %d", getConnectorName, page, getPageSize)
		pageObjects, latency, err := client.GetPushConnectorObjectsPageWithLatency(getConnectorName, getPageSize, page)
		if err != nil {
			if ui != nil {
				ui.Failure(fmt.Sprintf("GET page %d failed: %v", page, err))
			}
			return fmt.Errorf("failed to retrieve page %d: %w", page, err)
		}
		pageCount++
		totalLatency += latency
		requestLatencies = append(requestLatencies, latency)
		requestPages = append(requestPages, page)
		responseObjectCount := len(pageObjects)
		if ui != nil {
			ui.Read(len(pageObjects), latency)
		}
		shortPage := len(pageObjects) < getPageSize

		if getLimit > 0 && totalObjects+len(pageObjects) > getLimit {
			pageObjects = pageObjects[:getLimit-totalObjects]
		}
		totalObjects += len(pageObjects)
		if stdout != nil {
			for _, object := range pageObjects {
				if !firstObject {
					if err := stdout.WriteByte(','); err != nil {
						return fmt.Errorf("failed to write stdout output: %w", err)
					}
				}
				encoded, err := json.Marshal(object)
				if err != nil {
					return fmt.Errorf("failed to format object: %w", err)
				}
				if _, err := stdout.Write(encoded); err != nil {
					return fmt.Errorf("failed to write stdout output: %w", err)
				}
				firstObject = false
			}
			if err := stdout.Flush(); err != nil {
				return fmt.Errorf("failed to flush stdout output: %w", err)
			}
		} else {
			objects = append(objects, pageObjects...)
		}
		logger.Verbose("Retrieved page %d: response returned %d objects, accepted %d (total: %d)", page, responseObjectCount, len(pageObjects), totalObjects)
		if getLimit > 0 && totalObjects >= getLimit {
			break
		}
		if shortPage {
			break
		}
		page++
	}
	if ui != nil {
		average := time.Duration(0)
		if pageCount > 0 {
			average = totalLatency / time.Duration(pageCount)
		}
		summary := fmt.Sprintf("GET complete: %d operations, %d objects retrieved, average latency %s", pageCount, totalObjects, average.Round(time.Millisecond))
		select {
		case <-uiInterrupt:
			return fmt.Errorf("GET operation interrupted")
		default:
		}
		ui.Finish(summary)
		// Finish forces the final, possibly partial, five-request batch into the
		// TUI before the terminal is restored. The deferred summary handles both
		// normal completion and CTRL-C.
		stopUI()
	}

	if stdout != nil {
		if _, err := stdout.WriteString("]}\n"); err != nil {
			return fmt.Errorf("failed to finish stdout output: %w", err)
		}
		if err := stdout.Flush(); err != nil {
			return fmt.Errorf("failed to flush stdout output: %w", err)
		}
		return nil
	}

	output, err := json.MarshalIndent(map[string]interface{}{"data": objects}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}
	if getOutputFile == "" {
		fmt.Println(string(output))
		return nil
	}
	if err := os.WriteFile(getOutputFile, output, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}
	logger.Verbose("Wrote %d objects to %s", len(objects), getOutputFile)
	return nil
}

func printGetSummary(pages []int, latencies []time.Duration, objects int) {
	if len(latencies) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, tui.RenderBarChart(latencies, 100))
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "GET request summary")
	fmt.Fprintln(os.Stderr, "Request  Latency")
	fmt.Fprintln(os.Stderr, "-------  -------")
	for i, latency := range latencies {
		fmt.Fprintf(os.Stderr, "%7d  %s\n", pages[i], latency.Round(time.Millisecond))
	}
	fmt.Fprintf(os.Stderr, "\nTotal objects retrieved: %d\nTotal requests: %d\n", objects, len(latencies))
	if slope, intercept, ok := linearRegression(pages, latencies); ok {
		fmt.Fprintln(os.Stderr, "\nOrder-1 latency regression:")
		fmt.Fprintf(os.Stderr, "latency(page N) = %.3f ms + %.3f ms/page × N\n", intercept, slope)
	}
}

func linearRegression(pages []int, latencies []time.Duration) (slope, intercept float64, ok bool) {
	if len(pages) != len(latencies) || len(pages) == 0 {
		return 0, 0, false
	}
	var meanPage, meanLatency float64
	for i, page := range pages {
		meanPage += float64(page)
		meanLatency += float64(latencies[i]) / float64(time.Millisecond)
	}
	meanPage /= float64(len(pages))
	meanLatency /= float64(len(latencies))

	var covariance, variance float64
	for i, page := range pages {
		deltaPage := float64(page) - meanPage
		deltaLatency := float64(latencies[i])/float64(time.Millisecond) - meanLatency
		covariance += deltaPage * deltaLatency
		variance += deltaPage * deltaPage
	}
	if variance == 0 {
		return 0, meanLatency, true
	}
	slope = covariance / variance
	intercept = meanLatency - slope*meanPage
	return slope, intercept, true
}
