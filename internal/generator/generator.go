package generator

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/einarnn/pxctl/internal/api"
	"github.com/google/uuid"
)

const (
	charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// stripJSONPath removes the $. prefix from JSON path attributes
func stripJSONPath(attr string) string {
	return strings.TrimPrefix(attr, "$.")
}

// GenerateTestData generates test data based on the connector configuration
func GenerateTestData(config *api.ConnectorConfig, count int) map[string]interface{} {
	data := make([]map[string]interface{}, count)

	for i := 0; i < count; i++ {
		element := make(map[string]interface{})

		// Strip $. prefix from attribute names
		correlationAttr := stripJSONPath(config.Response.Connector.Attributes.CorrelationIdentifier)
		uniqueAttr := stripJSONPath(config.Response.Connector.Attributes.UniqueIdentifier)
		versionAttr := stripJSONPath(config.Response.Connector.Attributes.VersionIdentifier)

		// Generate correlation identifier (MAC address in locally administered range)
		element[correlationAttr] = generateLocalMAC()

		// Generate unique identifier (UUID4)
		element[uniqueAttr] = uuid.New().String()

		// Generate version identifier (UTC timestamp in ISO format)
		element[versionAttr] = time.Now().UTC().Format(time.RFC3339)

		// Generate random strings for all other mapped attributes
		for _, mapping := range config.Response.Connector.Attributes.AttributeMapping {
			strippedAttr := stripJSONPath(mapping.JSONAttribute)

			// Skip if we already generated this field
			if strippedAttr == correlationAttr ||
				strippedAttr == uniqueAttr ||
				strippedAttr == versionAttr {
				continue
			}

			// Generate random ASCII string between 8 and 64 characters
			element[strippedAttr] = generateRandomString(8, 64)
		}

		data[i] = element
	}

	// Wrap in top level object if specified
	result := make(map[string]interface{})
	if config.Response.Connector.Attributes.TopLevelObject != "" {
		result[config.Response.Connector.Attributes.TopLevelObject] = data
	} else {
		result["data"] = data
	}

	return result
}

// generateLocalMAC generates a MAC address in the locally administered range
// Sets the second least significant bit of the first octet to 1
func generateLocalMAC() string {
	mac := make([]byte, 6)
	rand.Read(mac)

	// Set the locally administered bit (bit 1 of the first byte)
	// and clear the multicast bit (bit 0 of the first byte)
	mac[0] = (mac[0] | 0x02) & 0xFE

	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

// generateRandomString generates a random ASCII string of random length between min and max
func generateRandomString(minLen, maxLen int) string {
	// Generate random length between min and max
	lengthRange := maxLen - minLen + 1
	lengthBig, _ := rand.Int(rand.Reader, big.NewInt(int64(lengthRange)))
	length := int(lengthBig.Int64()) + minLen

	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		idx, _ := rand.Int(rand.Reader, charsetLen)
		result[i] = charset[idx.Int64()]
	}

	return string(result)
}
