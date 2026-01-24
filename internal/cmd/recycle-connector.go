package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/einarnn/pxctl/internal/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	recycleIseHost       string
	recycleIseUsername   string
	recycleIsePassword   string
	recycleConnectorName string
	recycleCopyName      string
)

var recycleConnectorCmd = &cobra.Command{
	Use:   "recycle-connector",
	Short: "Delete and recreate or copy a pxGrid Direct connector",
	Long: `Delete and recreate or copy a pxGrid Direct connector with the same configuration.

By default, this command retrieves the connector configuration, deletes the connector, and recreates it
with updated schedule parameters.

With the --copy flag, it creates a copy of the connector without deleting the original.

For PULL connectors, it triggers a sync-now after recreation/copy.`,
	RunE: runRecycleConnector,
}

func init() {
	rootCmd.AddCommand(recycleConnectorCmd)

	recycleConnectorCmd.Flags().StringVarP(&recycleIseHost, "host", "H", "", "ISE FQDN or IP address (env: PXCTL_ISE_HOST)")
	recycleConnectorCmd.Flags().StringVarP(&recycleIseUsername, "username", "u", "", "ISE username (env: PXCTL_ISE_USERNAME)")
	recycleConnectorCmd.Flags().StringVarP(&recycleIsePassword, "password", "p", "", "ISE password (env: PXCTL_ISE_PASSWORD)")
	recycleConnectorCmd.Flags().StringVarP(&recycleConnectorName, "connector", "c", "", "pxGrid Direct connector name (required)")
	recycleConnectorCmd.Flags().StringVar(&recycleCopyName, "copy", "", "Name to copy the source connector to (if provided, creates a copy instead of deleting)")

	// Bind environment variables
	viper.BindEnv("ise.host", "PXCTL_ISE_HOST")
	viper.BindEnv("ise.username", "PXCTL_ISE_USERNAME")
	viper.BindEnv("ise.password", "PXCTL_ISE_PASSWORD")

	// Bind flags to viper
	viper.BindPFlag("ise.host", recycleConnectorCmd.Flags().Lookup("host"))
	viper.BindPFlag("ise.username", recycleConnectorCmd.Flags().Lookup("username"))
	viper.BindPFlag("ise.password", recycleConnectorCmd.Flags().Lookup("password"))

	recycleConnectorCmd.MarkFlagRequired("connector")
}

func runRecycleConnector(cmd *cobra.Command, args []string) error {
	// Get values from Viper (checks env vars, flags, and config file)
	host := viper.GetString("ise.host")
	username := viper.GetString("ise.username")
	password := viper.GetString("ise.password")

	logger.Verbose("Configuration: host=%s, username=%s, connector=%s",
		host, username, recycleConnectorName)

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

	// Step 1: Retrieve the named connector config (using raw method to preserve ALL fields)
	// This ensures we extract the FULL object at $.response.connector including credentials,
	// schedules, and any other fields not explicitly defined in the struct.
	fmt.Printf("Retrieving configuration for connector '%s'...\n", recycleConnectorName)
	logger.Verbose("Fetching connector configuration for: %s", recycleConnectorName)

	configMap, err := client.GetConnectorConfigRaw(recycleConnectorName)
	if err != nil {
		return fmt.Errorf("failed to retrieve connector config: %w", err)
	}

	// Extract connector type to determine if it's a PULL connector
	var connectorType string
	if response, ok := configMap["response"].(map[string]interface{}); ok {
		if connector, ok := response["connector"].(map[string]interface{}); ok {
			if ct, ok := connector["connectorType"].(string); ok {
				connectorType = ct
			}
		}
	}

	if connectorType == "" {
		return fmt.Errorf("failed to extract connector type from configuration")
	}

	isPullConnector := (connectorType == "urlfetcher" || connectorType == "vmware")

	fmt.Printf("Retrieved connector configuration (type: %s)\n", connectorType)
	logger.Verbose("Connector type: %s, isPullConnector: %t", connectorType, isPullConnector)

	// Determine the target connector name
	targetConnectorName := recycleConnectorName
	copyMode := recycleCopyName != ""
	if copyMode {
		// For copy mode, use the provided name
		targetConnectorName = recycleCopyName
		fmt.Printf("Copy mode: New connector will be named '%s'\n", targetConnectorName)
		logger.Verbose("Copy mode: Target connector name: %s", targetConnectorName)
	}

	// Step 2: Delete the connector (only if NOT in copy mode)
	if !copyMode {
		fmt.Printf("Deleting connector '%s'...\n", recycleConnectorName)
		logger.Verbose("Attempting to delete connector: %s", recycleConnectorName)

		if err := client.DeleteConnector(recycleConnectorName); err != nil {
			return fmt.Errorf("failed to delete connector: %w", err)
		}

		fmt.Printf("Successfully deleted connector '%s'\n", recycleConnectorName)
		logger.Verbose("Connector deleted successfully")

		// Wait 5 seconds before recreating
		fmt.Println("Waiting 5 seconds before recreating connector...")
		logger.Verbose("Sleeping for 5 seconds before recreation")
		time.Sleep(5 * time.Second)
	}

	// Step 3: Update schedule parameters to be at least 30 minutes in the future
	logger.Verbose("Updating schedule parameters to be at least 30 minutes in the future")

	futureTime := time.Now().Add(30 * time.Minute)
	futureTimeStr := futureTime.Format("2006-01-02T15:04:05")

	if response, ok := configMap["response"].(map[string]interface{}); ok {
		if connector, ok := response["connector"].(map[string]interface{}); ok {
			// Update connector name if in copy mode
			if copyMode {
				connector["connectorName"] = targetConnectorName
				logger.Verbose("Updated connector name to: %s", targetConnectorName)
			}

			// Update fullsyncSchedule if it exists
			if fullsync, ok := connector["fullsyncSchedule"].(map[string]interface{}); ok {
				fullsync["startDate"] = futureTimeStr
				logger.Verbose("Updated fullsyncSchedule startDate to: %s", futureTimeStr)
			}

			// Update deltasyncSchedule if it exists
			if deltasync, ok := connector["deltasyncSchedule"].(map[string]interface{}); ok {
				deltasync["startDate"] = futureTimeStr
				logger.Verbose("Updated deltasyncSchedule startDate to: %s", futureTimeStr)
			}
		}
	}

	// Step 4: Create or recreate the connector with retry logic
	if copyMode {
		fmt.Printf("Creating copy of connector as '%s'...\n", targetConnectorName)
		logger.Verbose("Creating connector copy with updated configuration")
	} else {
		fmt.Printf("Recreating connector '%s'...\n", targetConnectorName)
		logger.Verbose("Recreating connector with updated configuration")
	}

	// Extract just the connector config part (without version)
	recreateConfig := map[string]interface{}{
		"connector": configMap["response"].(map[string]interface{})["connector"],
	}

	// Construct the URL that will be used
	postURL := fmt.Sprintf("%s/api/v1/pxgrid-direct/connector-config", client.BaseURL)

	// Set up signal handling for CTRL+C
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nReceived interrupt signal. Cancelling operation...")
		logger.Verbose("User interrupted operation with CTRL+C")
		cancel()
	}()

	// Retry loop for connector creation
	retryDelay := 5 // Start with 5 seconds
	attemptNum := 0
	const endpointDeletionError = "When a connector with huge number of endpoints is deleted"

	for {
		attemptNum++
		if attemptNum > 1 {
			logger.Verbose("Retry attempt %d after %d second delay", attemptNum, retryDelay)
		}

		err := client.CreateConnector(recreateConfig)

		if err == nil {
			// Success!
			break
		}

		// Check if this is the specific endpoint deletion error
		errMsg := err.Error()
		isEndpointError := strings.Contains(errMsg, "status 400") &&
			strings.Contains(errMsg, endpointDeletionError)

		if !isEndpointError {
			// Not the specific error we're looking for - fail immediately
			fmt.Printf("\nError creating connector. Details:\n")
			fmt.Printf("URL: %s\n", postURL)
			fmt.Printf("\nJSON Payload:\n")
			prettyJSON, jsonErr := json.MarshalIndent(recreateConfig, "", "  ")
			if jsonErr == nil {
				fmt.Printf("%s\n", string(prettyJSON))
			} else {
				fmt.Printf("(Failed to prettify JSON: %v)\n", jsonErr)
			}

			if copyMode {
				return fmt.Errorf("failed to create connector copy: %w", err)
			}
			return fmt.Errorf("failed to recreate connector: %w", err)
		}

		// It's the endpoint deletion error - retry with backoff
		if attemptNum == 1 {
			fmt.Printf("Connector creation failed: endpoints from deleted connector are still being removed.\n")
			logger.Verbose("Detected endpoint deletion in progress: %s", errMsg)
		}

		fmt.Printf("Retrying in %d seconds (attempt %d)... Press CTRL+C to cancel.\n", retryDelay, attemptNum)
		logger.Verbose("Waiting %d seconds before retry attempt %d", retryDelay, attemptNum+1)

		// Wait with context cancellation support
		select {
		case <-ctx.Done():
			// User hit CTRL+C
			fmt.Println("Operation cancelled by user.")
			return fmt.Errorf("operation cancelled by user")
		case <-time.After(time.Duration(retryDelay) * time.Second):
			// Continue to next retry
		}

		// Increase delay by 5 seconds for next attempt (5, 10, 15, 20, ...)
		retryDelay += 5
	}

	// Clean up signal handler
	signal.Stop(sigChan)
	close(sigChan)

	if copyMode {
		fmt.Printf("Successfully created connector copy '%s'\n", targetConnectorName)
		logger.Verbose("Connector copy created successfully")
	} else {
		fmt.Printf("Successfully recreated connector '%s'\n", targetConnectorName)
		logger.Verbose("Connector recreated successfully")
	}

	// Step 5: If it's a PULL connector, trigger sync-now
	if isPullConnector {
		fmt.Printf("Triggering sync-now for PULL connector '%s'...\n", targetConnectorName)
		logger.Verbose("Triggering FULL sync-now for pull connector")

		if err := client.SyncNowConnector(targetConnectorName, "FULL"); err != nil {
			return fmt.Errorf("failed to trigger sync-now: %w", err)
		}

		fmt.Printf("Successfully triggered sync-now for connector '%s'\n", targetConnectorName)
		logger.Verbose("Sync-now triggered successfully")
	}

	if copyMode {
		fmt.Printf("\nConnector '%s' copied successfully as '%s'!\n", recycleConnectorName, targetConnectorName)
		logger.Verbose("Copy operation completed successfully")
	} else {
		fmt.Printf("\nConnector '%s' recycled successfully!\n", targetConnectorName)
		logger.Verbose("Recycle operation completed successfully")
	}

	return nil
}
