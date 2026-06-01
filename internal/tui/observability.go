package tui

import "strings"

func (m Model) writeObservability(b *strings.Builder) {
	if m.snapshot == nil {
		return
	}
	m.writeRecentRequests(b)
	m.writeUsageMetrics(b)
	m.writeHealthAndQuota(b)
	m.writeSubscriptionUsage(b)
	m.writeFallbacks(b)
}
