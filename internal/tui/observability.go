package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeObservability(b *strings.Builder) {
	if m.snapshot == nil {
		return
	}
	m.writeRecentRequests(b)
	m.writeUsageMetrics(b)
	m.writeHealthAndQuota(b)
	m.writeSubscriptionUsage(b)
	b.WriteString("\nFallbacks\n")
	if len(m.fallbackRows) == 0 {
		b.WriteString("No fallback metadata.\n")
	}
	for _, row := range m.fallbackRows {
		fmt.Fprintf(b, "- %s %s/%s %s -> %s %s\n",
			formatTime(row.OccurredAt), safeDisplay(row.ProviderInstanceID), safeDisplay(row.ModelID),
			credentialDisplay(row.FromCredentialID, row.FromCredentialLabel),
			credentialDisplay(row.ToCredentialID, row.ToCredentialLabel), safeDisplay(row.Reason))
	}
}

func (m Model) writePruning(b *strings.Builder) {
	if m.pruner == nil && !m.pruningAvailable {
		return
	}
	b.WriteString("\nTelemetry pruning\n")
	b.WriteString("Retention keep forever until pruned.\n")
	b.WriteString("Manual prune cutoff older than 30 days.\n")
	if m.pruneResult != nil {
		fmt.Fprintf(b, "Last prune before %s: requests %d streams %d fallbacks %d health %d quotas %d\n",
			formatPreciseTime(m.pruneResult.Cutoff), m.pruneResult.Requests, m.pruneResult.Streams,
			m.pruneResult.Fallbacks, m.pruneResult.Health, m.pruneResult.Quotas)
	}
}
