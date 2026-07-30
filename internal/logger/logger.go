package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

var verboseEnabled bool
var eventSink func(string)
var retrySink func(float64)

// SetVerbose enables or disables verbose logging
func SetVerbose(enabled bool) {
	verboseEnabled = enabled
}

// SetEventSink sends operation messages to an alternate UI, such as the TUI.
func SetEventSink(sink func(string)) {
	eventSink = sink
}

func SetRetrySink(sink func(float64)) {
	retrySink = sink
}

// Event emits an operation message without enabling verbose stderr logging.
func Event(format string, args ...interface{}) {
	if eventSink != nil {
		eventSink(fmt.Sprintf(format, args...))
	}
}

// IsVerbose returns whether verbose logging is enabled
func IsVerbose() bool {
	return verboseEnabled
}

// Verbose logs a message to stderr if verbose mode is enabled
func Verbose(format string, args ...interface{}) {
	Event(format, args...)
	if verboseEnabled {
		timestamp := time.Now().Format("2006-01-02 15:04:05.000")
		message := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[%s] %s\n", timestamp, message)
	}
}

// HTTPRequest logs details about an HTTP request
func HTTPRequest(method, url string) {
	Verbose("HTTP Request: %s %s", method, url)
}

// HTTPResponse logs details about an HTTP response
func HTTPResponse(statusCode int, status string, duration time.Duration) {
	Verbose("HTTP Response: %d %s (took %v)", statusCode, status, duration)
}

// Retry logs details about a retry attempt
func Retry(reason string, backoffSeconds float64) {
	if retrySink != nil {
		retrySink(backoffSeconds)
	}
	if eventSink != nil {
		eventSink(fmt.Sprintf("429 received; backing off for %.3f seconds", backoffSeconds))
	}
	if verboseEnabled {
		timestamp := time.Now().Format("2006-01-02 15:04:05.000")
		fmt.Fprintf(os.Stderr, "[%s] Retry: %s - backing off for %.3f seconds\n", timestamp, reason, backoffSeconds)
	}
}

// VerbosePrettyJSON logs a label followed by prettified JSON, with each line
// individually timestamped. This keeps large payloads readable in log output.
func VerbosePrettyJSON(label string, data []byte) {
	if !verboseEnabled {
		return
	}
	var pretty []byte
	var buf interface{}
	if err := json.Unmarshal(data, &buf); err == nil {
		pretty, _ = json.MarshalIndent(buf, "  ", "  ")
	}
	if pretty == nil {
		// Fallback: log raw if prettification fails
		Verbose("%s: %s", label, string(data))
		return
	}
	Verbose("%s:", label)
	for _, line := range strings.Split(string(pretty), "\n") {
		Verbose("  %s", line)
	}
}

// Info logs an informational message (always shown)
func Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
