package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/einarnn/pxctl/internal/logger"
)

// PutDataResponse represents the PUT API response
type PutDataResponse struct {
	Status string `json:"status"`
}

// PutData submits a single object to the PUT API endpoint with retry logic for 429 rate limiting
func (c *Client) PutData(connectorName string, uniqueID string, data map[string]interface{}, backoffSeconds float64) (*PutDataResponse, error) {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/push/%s/%s", c.BaseURL, connectorName, uniqueID)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	logger.Verbose("Prepared PUT request for object %s (%d bytes)", uniqueID, len(jsonData))

	// Retry loop for 429 errors
	for {
		req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.SetBasicAuth(c.Username, c.Password)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		logger.HTTPRequest("PUT", url)
		startTime := time.Now()

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		duration := time.Since(startTime)
		logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

		// Handle 429 rate limiting
		if resp.StatusCode == http.StatusTooManyRequests {
			logger.Retry("received 429 Too Many Requests", backoffSeconds)
			backoffDuration := time.Duration(backoffSeconds * float64(time.Second))
			time.Sleep(backoffDuration)
			continue // Retry the request
		}

		// Handle other error statuses
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Success - parse and return response
		var response PutDataResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		return &response, nil
	}
}
