package cmd

import (
	"fmt"
	"os"
	"time"
)

// formatBytes formats byte size into human-readable format
func formatBytes(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func adaptiveRetryDelay(current float64, duration time.Duration, clean bool) float64 {
	seconds := current
	if seconds <= 0 {
		seconds = duration.Seconds()
	}

	if clean {
		// Probe downward slowly. Never choose an interval shorter than the
		// request itself; pacing cannot make a request complete faster.
		seconds *= 0.98
		if requestSeconds := duration.Seconds(); seconds < requestSeconds {
			seconds = requestSeconds
		}
	} else {
		// A 429 is congestion feedback. Increase the main interval decisively,
		// then let clean requests walk it down again.
		seconds *= 1.25
	}
	if seconds < 0.001 {
		seconds = 0.001
	}
	if seconds > 120 {
		seconds = 120
	}
	return seconds
}

func waitForPacing(duration time.Duration, signals <-chan os.Signal, interrupt <-chan struct{}) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-signals:
		return false
	case <-interrupt:
		return false
	}
}
