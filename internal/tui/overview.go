package tui

import (
	"fmt"
	"sort"
	"strings"

	"ilonasin/internal/management"
)

func (m Model) writeOverview(b *strings.Builder) {
	fmt.Fprintf(b, "Providers: %d\nBind: %s\n", len(m.cfg.Providers), m.cfg.Server.Bind)
	b.WriteString("\nProvider instances\n")
	for _, instance := range m.providers {
		apiKey := "api-key disabled"
		if instance.APIKey {
			apiKey = "api-key"
		}
		oauth := "oauth disabled"
		if instance.OAuth {
			oauth = "oauth"
		}
		fmt.Fprintf(b, "- %s %s %s %s %s\n", instance.ID, instance.Type, instance.BaseURL, apiKey, oauth)
	}
	b.WriteString("\nModel cache\n")
	summaries := modelCacheSummaries(m.modelRows)
	if len(summaries) == 0 {
		b.WriteString("No cached models.\n")
	}
	for _, summary := range summaries {
		fmt.Fprintf(b, "- %s %d models updated %s\n", summary.ProviderInstanceID, summary.Count, summary.UpdatedAt)
	}
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
	m.writePruning(b)
}

type modelCacheSummary struct {
	ProviderInstanceID string
	Count              int
	UpdatedAt          string
}

func modelCacheSummaries(rows []management.ModelMetadata) []modelCacheSummary {
	byProvider := map[string]modelCacheSummary{}
	for _, row := range rows {
		summary := byProvider[row.ProviderInstanceID]
		summary.ProviderInstanceID = row.ProviderInstanceID
		summary.Count++
		updated := row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
		if updated > summary.UpdatedAt {
			summary.UpdatedAt = updated
		}
		byProvider[row.ProviderInstanceID] = summary
	}
	out := make([]modelCacheSummary, 0, len(byProvider))
	for _, summary := range byProvider {
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ProviderInstanceID < out[j].ProviderInstanceID
	})
	return out
}
