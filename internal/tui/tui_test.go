package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderBarChartCompressesLongTimeline(t *testing.T) {
	history := make([]time.Duration, 1000)
	for i := range history {
		history[i] = time.Duration(i+1) * time.Millisecond
	}
	rendered := renderBarChart(history, 80)
	if rendered == "" {
		t.Fatal("expected bar chart output")
	}
	if lines := strings.Split(rendered, "\n"); len(lines) != 6 {
		t.Fatalf("expected title plus five chart rows, got %d lines", len(lines))
	}
	for _, line := range strings.Split(rendered, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("bar chart line exceeds terminal width: %d", lipgloss.Width(line))
		}
	}
}
