package logger

import (
	"fmt"
	"os"
	"time"
)

var verboseEnabled bool

// SetVerbose enables or disables verbose logging
func SetVerbose(enabled bool) {
	verboseEnabled = enabled
}

// IsVerbose returns whether verbose logging is enabled
func IsVerbose() bool {
	return verboseEnabled
}

// Verbose logs a message to stderr if verbose mode is enabled
func Verbose(format string, args ...interface{}) {
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
	Verbose("Retry: %s - backing off for %.3f seconds", reason, backoffSeconds)
}

// Info logs an informational message (always shown)
func Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
