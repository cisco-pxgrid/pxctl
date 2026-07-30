package cmd

import (
	"testing"
	"time"
)

func TestAdaptiveRetryDelayMovesInBoundedSteps(t *testing.T) {
	current := 10.0

	if got := adaptiveRetryDelay(current, 100*time.Millisecond, true); got != 9.8 {
		t.Fatalf("expected a 2%% clean decrease, got %.2f", got)
	}
	if got := adaptiveRetryDelay(current, 60*time.Second, false); got != 12.5 {
		t.Fatalf("expected a 25%% congestion increase, got %.2f", got)
	}
}

func TestAdaptiveRetryDelayCleanPeriodReducesTarget(t *testing.T) {
	got := adaptiveRetryDelay(10, 9*time.Second, true)
	if got < 9.799 || got > 9.801 {
		t.Fatalf("expected clean period to reduce the interval by 2%%, got %.2f", got)
	}
}

func TestAdaptiveRetryDelayNeverDropsBelowRequestDuration(t *testing.T) {
	if got := adaptiveRetryDelay(10, 20*time.Second, true); got != 20 {
		t.Fatalf("expected request duration floor, got %.2f", got)
	}
}
