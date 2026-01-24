package generator

import (
	"strings"
	"testing"

	"github.com/einarnn/pxctl/internal/api"
)

func TestStripJSONPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "attribute with $. prefix",
			input:    "$.macAddress",
			expected: "macAddress",
		},
		{
			name:     "attribute without $. prefix",
			input:    "macAddress",
			expected: "macAddress",
		},
		{
			name:     "nested path with $. prefix",
			input:    "$.result.sys_id",
			expected: "result.sys_id",
		},
		{
			name:     "only $. prefix",
			input:    "$.",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripJSONPath(tt.input)
			if result != tt.expected {
				t.Errorf("stripJSONPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateTestData_StripsDollarPrefix(t *testing.T) {
	// Create a mock connector config with $. prefixed attributes
	config := &api.ConnectorConfig{
		Response: api.ConnectorConfigResponse{
			Connector: api.ConnectorDetails{
				ConnectorName: "TEST_CONNECTOR",
				Attributes: api.ConnectorAttributes{
					CorrelationIdentifier: "$.macAddress",
					UniqueIdentifier:      "$.sys_id",
					VersionIdentifier:     "$.sys_updated_on",
					TopLevelObject:        "result",
					AttributeMapping: []api.AttributeMapping{
						{
							JSONAttribute:       "$.macAddress",
							DictionaryAttribute: "macAddress",
							IncludeInDictionary: true,
						},
						{
							JSONAttribute:       "$.sys_id",
							DictionaryAttribute: "systemId",
							IncludeInDictionary: true,
						},
						{
							JSONAttribute:       "$.sys_updated_on",
							DictionaryAttribute: "version",
							IncludeInDictionary: true,
						},
						{
							JSONAttribute:       "$.hostname",
							DictionaryAttribute: "hostname",
							IncludeInDictionary: true,
						},
					},
				},
			},
		},
		Version: "1.0.0",
	}

	// Generate test data
	result := GenerateTestData(config, 2)

	// Check that result contains the top level object
	if _, ok := result["result"]; !ok {
		t.Fatal("Expected 'result' key in output")
	}

	// Get the data array
	dataArray, ok := result["result"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected result to be an array of maps")
	}

	if len(dataArray) != 2 {
		t.Fatalf("Expected 2 elements, got %d", len(dataArray))
	}

	// Check that attributes don't have $. prefix
	for i, element := range dataArray {
		// Should have stripped attributes (without $. prefix)
		if _, ok := element["macAddress"]; !ok {
			t.Errorf("Element %d: expected 'macAddress' key (without $. prefix)", i)
		}
		if _, ok := element["sys_id"]; !ok {
			t.Errorf("Element %d: expected 'sys_id' key (without $. prefix)", i)
		}
		if _, ok := element["sys_updated_on"]; !ok {
			t.Errorf("Element %d: expected 'sys_updated_on' key (without $. prefix)", i)
		}
		if _, ok := element["hostname"]; !ok {
			t.Errorf("Element %d: expected 'hostname' key (without $. prefix)", i)
		}

		// Should NOT have attributes with $. prefix
		if _, ok := element["$.macAddress"]; ok {
			t.Errorf("Element %d: should not have '$.macAddress' key (should be stripped)", i)
		}

		// Verify MAC address format (correlation ID)
		if mac, ok := element["macAddress"].(string); ok {
			if !strings.Contains(mac, ":") {
				t.Errorf("Element %d: macAddress (correlation ID) should be in MAC format", i)
			}
		}

		// Verify UUID format (unique identifier)
		if uuid, ok := element["sys_id"].(string); ok {
			if !strings.Contains(uuid, "-") {
				t.Errorf("Element %d: sys_id (unique identifier) should be in UUID format", i)
			}
		}

		// Verify timestamp format (version identifier)
		if timestamp, ok := element["sys_updated_on"].(string); ok {
			if !strings.Contains(timestamp, "T") {
				t.Errorf("Element %d: sys_updated_on should be in ISO 8601 format", i)
			}
		}

		// Verify hostname is a random string
		if hostname, ok := element["hostname"].(string); ok {
			if len(hostname) < 8 || len(hostname) > 64 {
				t.Errorf("Element %d: hostname length should be between 8 and 64, got %d", i, len(hostname))
			}
		}
	}
}
