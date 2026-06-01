package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeUsageMetrics(b *strings.Builder) {
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
}
