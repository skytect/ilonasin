package management

import (
	"time"
)

func windowUsed(window *SubscriptionUsageFetchWindow) float64 {
	if window == nil {
		return 0
	}
	return window.UsedPercent
}

func windowMinutes(window *SubscriptionUsageFetchWindow) int {
	if window == nil {
		return 0
	}
	return window.WindowMinutes
}

func windowReset(window *SubscriptionUsageFetchWindow) *time.Time {
	if window == nil || window.ResetsAt == nil {
		return nil
	}
	reset := window.ResetsAt.UTC()
	return &reset
}
