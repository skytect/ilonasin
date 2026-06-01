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

func (m Model) writeHelp(b *strings.Builder) {
	b.WriteString("Keys\n")
	b.WriteString("- tab / shift+tab switch tabs\n")
	b.WriteString("- 1-4 jump to overview, accounts, observability, help\n")
	b.WriteString("- up/down or j/k scroll content outside accounts\n")
	b.WriteString("- up/down or j/k select local token on accounts\n")
	b.WriteString("- pgup/pgdown, ctrl+u/ctrl+d, home/end scroll content\n")
	b.WriteString("- n create local token on accounts\n")
	b.WriteString("- a add API key on accounts\n")
	b.WriteString("- d disable selected local token on accounts\n")
	b.WriteString("- x disable first enabled API key credential on accounts\n")
	b.WriteString("- l login or relogin OAuth on accounts\n")
	b.WriteString("- o select OAuth account on accounts\n")
	b.WriteString("- r refresh selected OAuth account on accounts\n")
	b.WriteString("- f/F enable or disable first credential group fallback on accounts\n")
	b.WriteString("- p prune telemetry older than 30 days on observability\n")
	b.WriteString("- esc clears transient messages or cancels OAuth login\n")
	b.WriteString("- q quits\n")
	b.WriteString("\nPrivacy\n")
	b.WriteString("The TUI renders snapshot metadata and redacted display values only. It does not render prompts, completions, request bodies, response bodies, raw streams, tool arguments, tool results, provider payloads, provider request IDs, full local tokens, or full provider account IDs.\n")
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
