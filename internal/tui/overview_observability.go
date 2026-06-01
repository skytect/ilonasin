package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeOverviewObservabilitySummary(b *strings.Builder) {
	b.WriteString("\nObservability summary\n")
	fmt.Fprintf(b, "- recent requests %d\n", len(m.requestRows))
	for _, row := range m.usageRows {
		fmt.Fprintf(b, "- %s %d req total %d cache_hit_rate %.2f cache_miss_rate %.2f reasoning_rate %.2f\n",
			safeDisplay(row.ProviderInstanceID), row.RequestCount, row.TotalTokens,
			row.CacheHitRate, row.CacheMissRate, row.ReasoningTokenRate)
	}
	for _, row := range m.latencyRows {
		fmt.Fprintf(b, "- %s avg latency %dms upstream %dms ttft %dms tps_after_ttft %.2f\n",
			safeDisplay(row.ProviderInstanceID), row.AverageLatencyMS,
			row.AverageUpstreamLatencyMS, row.AverageTimeToFirstTokenMS,
			row.AverageOutputTPSAfterTTFT)
	}
}
