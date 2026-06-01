package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeRecentRequests(b *strings.Builder) {
	b.WriteString("\nRecent requests\n")
	if len(m.requestRows) == 0 {
		b.WriteString("No request metadata.\n")
	}
	for _, row := range m.requestRows {
		credential := credentialDisplay(row.CredentialID, row.CredentialLabel)
		fallbackReason := ""
		if row.FallbackReason != "" {
			fallbackReason = " reason " + safeDisplay(row.FallbackReason)
		}
		route := safeEndpointDisplay(row.Endpoint)
		if row.Stream {
			route += " stream"
		}
		options := ""
		if row.RequestedServiceTier != "" {
			options += " service_tier " + safeDisplay(row.RequestedServiceTier)
		}
		if row.EffectiveServiceTier != "" && row.EffectiveServiceTier != row.RequestedServiceTier {
			options += " effective_tier " + safeDisplay(row.EffectiveServiceTier)
		}
		if row.ReasoningEffort != "" {
			options += " reasoning " + safeDisplay(row.ReasoningEffort)
		}
		if row.ThinkingType != "" {
			options += " thinking " + safeDisplay(row.ThinkingType)
		}
		fmt.Fprintf(b, "- %s %s %s status %d %s\n",
			formatTime(row.StartedAt), route, requestModelDisplay(row),
			row.HTTPStatus, safeDisplay(row.ErrorClass))
		fmt.Fprintf(b, "  credential %s attempts %d auth_retry %d fallback %d%s\n",
			credential, row.AttemptCount, row.AuthRetryCount, row.FallbackCount, fallbackReason)
		fmt.Fprintf(b, "  shape msg %d tools %d images %d%s\n",
			row.MessageCount, row.ToolCount, row.ImageCount, options)
		fmt.Fprintf(b, "  tokens prompt %d completion %d total %d reasoning %d reasoning_rate %.2f\n",
			row.PromptTokens, row.CompletionTokens, row.TotalTokens, row.ReasoningTokens, row.ReasoningTokenRate)
		fmt.Fprintf(b, "  cache_hit %d cache_hit_rate %.2f\n", row.CacheHitTokens, row.CacheHitRate)
		fmt.Fprintf(b, "  cache_miss %d cache_miss_rate %.2f\n", row.CacheMissTokens, row.CacheMissRate)
		fmt.Fprintf(b, "  cache_write %d cache_write_rate %.2f\n", row.CacheWriteTokens, row.CacheWriteRate)
		fmt.Fprintf(b, "  latency total %dms upstream %dms ttft %dms tps_total %.2f tps_after_ttft %.2f\n",
			row.TotalLatencyMS, row.UpstreamLatencyMS, row.TimeToFirstTokenMS,
			row.OutputTokensPerSecondTotal, row.OutputTokensPerSecondAfterTTFT)
	}
}
