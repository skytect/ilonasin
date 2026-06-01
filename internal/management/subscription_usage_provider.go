package management

import (
	"time"

	"ilonasin/internal/provider"
)

func windowUsed(window *provider.CodexRateLimitWindow) float64 {
	if window == nil {
		return 0
	}
	return window.UsedPercent
}

func windowMinutes(window *provider.CodexRateLimitWindow) int {
	if window == nil {
		return 0
	}
	return window.WindowMinutes
}

func windowReset(window *provider.CodexRateLimitWindow) *time.Time {
	if window == nil || window.ResetsAt == nil {
		return nil
	}
	reset := window.ResetsAt.UTC()
	return &reset
}
