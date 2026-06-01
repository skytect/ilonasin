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
	b.WriteString("\nUsage totals\n")
	if len(m.usageRows) == 0 {
		b.WriteString("No usage metadata.\n")
	}
	for _, row := range m.usageRows {
		fmt.Fprintf(b, "- %s %d req cost_microunits %d\n",
			safeDisplay(row.ProviderInstanceID), row.RequestCount, row.CostMicrounits)
		fmt.Fprintf(b, "  tokens prompt %d completion %d total %d reasoning %d reasoning_rate %.2f\n",
			row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.ReasoningTokens, row.ReasoningTokenRate)
		fmt.Fprintf(b, "  cache_hit %d cache_hit_rate %.2f\n", row.CacheHitTokens, row.CacheHitRate)
		fmt.Fprintf(b, "  cache_miss %d cache_miss_rate %.2f\n", row.CacheMissTokens, row.CacheMissRate)
		fmt.Fprintf(b, "  cache_write %d cache_write_rate %.2f\n", row.CacheWriteTokens, row.CacheWriteRate)
	}
	b.WriteString("\nLatency\n")
	if len(m.latencyRows) == 0 {
		b.WriteString("No latency metadata.\n")
	}
	for _, row := range m.latencyRows {
		fmt.Fprintf(b, "- %s %d req avg latency %dms upstream %dms ttft %dms tps %.2f tps_total %.2f tps_after_ttft %.2f\n",
			safeDisplay(row.ProviderInstanceID), row.RequestCount, row.AverageLatencyMS,
			row.AverageUpstreamLatencyMS, row.AverageTimeToFirstTokenMS, row.AverageOutputTPS,
			row.AverageOutputTPSTotal, row.AverageOutputTPSAfterTTFT)
	}
	b.WriteString("\nStreams\n")
	if len(m.streamRows) == 0 {
		b.WriteString("No stream metadata.\n")
	}
	for _, row := range m.streamRows {
		fmt.Fprintf(b, "- %s %d streams %d chunks\n", safeDisplay(row.CompletionStatus), row.StreamCount, row.ChunkCount)
	}
	b.WriteString("\nHealth\n")
	if len(m.healthRows) == 0 {
		b.WriteString("No health metadata.\n")
	}
	for _, row := range m.healthRows {
		retryAfter := ""
		if row.RetryAfter != nil {
			retryAfter = " retry_after " + formatTime(*row.RetryAfter)
		}
		fmt.Fprintf(b, "- %s/%s %s status %d %s %s at %s%s\n",
			safeDisplay(row.ProviderInstanceID), healthModelDisplay(row.ModelID),
			safeDisplay(row.EventClass), row.HTTPStatus, safeDisplay(row.ErrorClass),
			credentialDisplay(row.CredentialID, row.CredentialLabel), formatTime(row.OccurredAt), retryAfter)
	}
	b.WriteString("\nQuota\n")
	if len(m.quotaRows) == 0 {
		b.WriteString("No quota metadata.\n")
	}
	for _, row := range m.quotaRows {
		retryAfter := ""
		if row.RetryAfter != nil {
			retryAfter = " retry_after " + formatTime(*row.RetryAfter)
		}
		resetAt := ""
		if row.ResetAt != nil {
			resetAt = " reset " + formatTime(*row.ResetAt)
		}
		fmt.Fprintf(b, "- %s/%s %s status %d %s %s count %d at %s%s%s\n",
			safeDisplay(row.ProviderInstanceID), healthModelDisplay(row.ModelID),
			safeDisplay(row.Source), row.HTTPStatus, safeDisplay(row.ErrorClass),
			credentialDisplay(row.CredentialID, row.CredentialLabel), row.Count,
			formatTime(row.ObservedAt), retryAfter, resetAt)
	}
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
