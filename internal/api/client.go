package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/einarnn/pxctl/internal/logger"
)

// Client represents an ISE API client
type Client struct {
	BaseURL    string
	Username   string
	Password   string
	HTTPClient *http.Client
}

// ConnectorConfig represents the connector configuration response
type ConnectorConfig struct {
	Response ConnectorConfigResponse `json:"response"`
	Version  string                  `json:"version"`
}

// ConnectorConfigResponse represents the connector details
type ConnectorConfigResponse struct {
	Connector ConnectorDetails `json:"connector"`
}

// ConnectorDetails contains the connector configuration details
type ConnectorDetails struct {
	ConnectorName string              `json:"connectorName"`
	Description   string              `json:"description"`
	Enabled       bool                `json:"enabled"`
	ConnectorType string              `json:"connectorType"`
	Attributes    ConnectorAttributes `json:"attributes"`
}

// ConnectorAttributes contains the attribute mappings for the connector
type ConnectorAttributes struct {
	CorrelationIdentifier string             `json:"correlationIdentifier"`
	UniqueIdentifier      string             `json:"uniqueIdentifier"`
	VersionIdentifier     string             `json:"versionIdentifier"`
	TopLevelObject        string             `json:"topLevelObject"`
	AttributeMapping      []AttributeMapping `json:"attributeMapping"`
}

// AttributeMapping represents a single attribute mapping
type AttributeMapping struct {
	JSONAttribute       string `json:"jsonAttribute"`
	DictionaryAttribute string `json:"dictionaryAttribute"`
	IncludeInDictionary bool   `json:"includeInDictionary"`
	CoaSignificance     bool   `json:"coaSignificance"`
}

// NewClient creates a new ISE API client
func NewClient(host, username, password string) *Client {
	// Create HTTP client with TLS verification disabled (for testing)
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	return &Client{
		BaseURL:    fmt.Sprintf("https://%s", host),
		Username:   username,
		Password:   password,
		HTTPClient: httpClient,
	}
}

// GetAllConnectorNames retrieves names of all connectors
func (c *Client) GetAllConnectorNames() ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/connector-config", c.BaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.HTTPRequest("GET", url)
	startTime := time.Now()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)
	logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response []struct {
			Connector struct {
				ConnectorName string `json:"connectorName"`
			} `json:"connector"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	names := make([]string, 0, len(result.Response))
	for _, item := range result.Response {
		if item.Connector.ConnectorName != "" {
			names = append(names, item.Connector.ConnectorName)
		}
	}

	return names, nil
}

// GetConnectorConfig retrieves the connector configuration by name
func (c *Client) GetConnectorConfig(connectorName string) (*ConnectorConfig, error) {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/connector-config/%s", c.BaseURL, connectorName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.HTTPRequest("GET", url)
	startTime := time.Now()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)
	logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var config ConnectorConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &config, nil
}

// GetConnectorConfigRaw retrieves the full connector configuration as a map,
// preserving ALL fields including credentials, schedules, and any other fields
// that may not be defined in the ConnectorDetails struct.
func (c *Client) GetConnectorConfigRaw(connectorName string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/connector-config/%s", c.BaseURL, connectorName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.HTTPRequest("GET", url)
	startTime := time.Now()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)
	logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(body, &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return configMap, nil
}

// BulkPushRequest represents the bulk push API request body
type BulkPushRequest struct {
	Operation string                   `json:"operation"`
	Data      []map[string]interface{} `json:"data"`
}

// BulkPushResponse represents the bulk push API response
type BulkPushResponse struct {
	ConnectorName string `json:"connectorName"`
	Status        string `json:"status"`
	Data          string `json:"data"`
}

// BulkPushData submits data to the bulk push API endpoint
func (c *Client) BulkPushData(connectorName string, data []map[string]interface{}) (*BulkPushResponse, error) {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/push/%s/bulk", c.BaseURL, connectorName)

	// Create request body
	requestBody := BulkPushRequest{
		Operation: "create",
		Data:      data,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response BulkPushResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// GetAllPushConnectorObjects retrieves all objects from a push connector with pagination
func (c *Client) GetAllPushConnectorObjects(connectorName string) ([]map[string]interface{}, error) {
	var allObjects []map[string]interface{}
	page := 0
	size := 1000 // Objects per page

	for {
		url := fmt.Sprintf("%s/api/v1/pxgrid-direct/push/%s?page=%d&size=%d", c.BaseURL, connectorName, page, size)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.SetBasicAuth(c.Username, c.Password)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		logger.HTTPRequest("GET", url)
		startTime := time.Now()

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}
		defer resp.Body.Close()

		duration := time.Since(startTime)
		logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var response struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if len(response.Data) == 0 {
			break
		}

		allObjects = append(allObjects, response.Data...)
		logger.Verbose("Retrieved page %d with %d objects (total so far: %d)", page, len(response.Data), len(allObjects))

		// If we got fewer objects than the page size, we've reached the end
		if len(response.Data) < size {
			break
		}

		page++
	}

	return allObjects, nil
}

// BulkDeleteData submits delete operations to the bulk push API endpoint with retry logic for 429 rate limiting
func (c *Client) BulkDeleteData(connectorName string, data []map[string]interface{}, backoffSeconds float64) (*BulkPushResponse, error) {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/push/%s/bulk", c.BaseURL, connectorName)

	// Create request body
	requestBody := BulkPushRequest{
		Operation: "delete",
		Data:      data,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	logger.Verbose("Prepared bulk delete request with %d objects (%d bytes)", len(data), len(jsonData))

	// Retry loop for 429 errors
	for {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.SetBasicAuth(c.Username, c.Password)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		logger.HTTPRequest("POST", url)
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
		var response BulkPushResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		return &response, nil
	}
}

// BulkPushDataWithRetry submits data to the bulk push API endpoint with retry logic for 429 rate limiting
func (c *Client) BulkPushDataWithRetry(connectorName string, data []map[string]interface{}, backoffSeconds float64) (*BulkPushResponse, error) {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/push/%s/bulk", c.BaseURL, connectorName)

	// Create request body
	requestBody := BulkPushRequest{
		Operation: "create",
		Data:      data,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	logger.Verbose("Prepared bulk push request with %d objects (%d bytes)", len(data), len(jsonData))
	logger.VerbosePrettyJSON("Request body", jsonData)

	// Retry loop for 429 errors
	for {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.SetBasicAuth(c.Username, c.Password)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")

		logger.HTTPRequest("POST", url)
		logger.Verbose("Request headers: Content-Type=application/json, Accept=application/json")
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
		logger.VerbosePrettyJSON("Response body", body)

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
		var response BulkPushResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		return &response, nil
	}
}

// DeleteConnector deletes a connector by name
func (c *Client) DeleteConnector(connectorName string) error {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/connector-config/%s", c.BaseURL, connectorName)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.HTTPRequest("DELETE", url)
	startTime := time.Now()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)
	logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CreateConnector creates a new connector with the provided configuration
func (c *Client) CreateConnector(config map[string]interface{}) error {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/connector-config", c.BaseURL)

	jsonData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.HTTPRequest("POST", url)
	startTime := time.Now()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)
	logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SyncNowConnector triggers a sync-now operation for a pull connector
func (c *Client) SyncNowConnector(connectorName string, syncType string) error {
	url := fmt.Sprintf("%s/api/v1/pxgrid-direct/syncnow", c.BaseURL)

	requestBody := map[string]interface{}{
		"connector": map[string]interface{}{
			"connectorName": connectorName,
			"SyncType":      syncType,
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	logger.HTTPRequest("POST", url)
	startTime := time.Now()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime)
	logger.HTTPResponse(resp.StatusCode, resp.Status, duration)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
