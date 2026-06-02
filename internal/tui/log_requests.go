package tui

import (
	"fmt"
	"strings"
	"time"

	"ilonasin/internal/management"
)

func (m Model) writeRecentRequests(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(renderSectionBanner(width, "Request metadata", fmt.Sprintf("recent %d", len(m.requestRows))))
	b.WriteByte('\n')
	if len(m.requestRows) == 0 {
		b.WriteString(metricLine(
			statusBadge("enabled"),
			cardTitleStyle.Render("metadata ledger"),
			metricChip("recent", "0"),
			metricChip("visibility", "metadata-only"),
			metricChip("content", "redacted"),
			metricChip("io", ioCaptureMode(m.runtime.CaptureIO)),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.requestRows {
		b.WriteString(requestSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

func requestSummaryRow(row management.RequestSummary, nowTime time.Time, width int) string {
	state := statusState(row.HTTPStatus, row.ErrorClass)
	head := wrappedMetricLine(width,
		statusBadge(state),
		endpointMetricChip("route", row.Endpoint),
		cardTitleStyle.Render(wrappedRequestModelDisplay(row, width)),
		metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
		timeChip("at", nowTime, row.StartedAt),
		streamChip(row.Stream),
	)
	tokens := wrappedMetricLine(width,
		compactTokenMixLine(row.PromptTokens, row.CompletionTokens, row.ReasoningTokens, row.CacheHitTokens, row.CacheMissTokens, row.CacheWriteTokens, width),
		metricChip("total", compactInt(row.TotalTokens)),
		compactRateBars(width, rateMetric{"hit", row.CacheHitRate * 100}),
	)
	route := wrappedMetricLine(width,
		mutedStyle.Render(credentialDisplay(row.CredentialID, row.CredentialLabel)),
		metricChip("try", fmt.Sprintf("%d", row.AttemptCount)),
		metricChip("auth", fmt.Sprintf("%d", row.AuthRetryCount)),
		metricChip("fb", fmt.Sprintf("%d", row.FallbackCount)),
		msText("lat", row.TotalLatencyMS),
		msText("ttft", row.TimeToFirstTokenMS),
		tpsText("tps", row.OutputTokensPerSecondTotal),
	)
	extras := requestSummaryExtras(row, width)
	lines := []string{head, tokens, route}
	if extras != "" {
		lines = append(lines, extras)
	}
	return strings.Join(lines, "\n")
}

func requestSummaryExtras(row management.RequestSummary, width int) string {
	parts := []string{}
	if row.FallbackReason != "" {
		parts = append(parts, metricChip("fallback-reason", row.FallbackReason))
	}
	if row.ErrorClass != "" {
		parts = append(parts, badBadgeStyle.Render(safeDisplay(row.ErrorClass)))
	}
	if !narrowMetrics(width) {
		parts = append(parts,
			metricChip("messages", fmt.Sprintf("%d", row.MessageCount)),
			metricChip("tools", fmt.Sprintf("%d", row.ToolCount)),
			metricChip("images", fmt.Sprintf("%d", row.ImageCount)),
		)
	}
	if width >= 96 {
		parts = append(parts, msText("up", row.UpstreamLatencyMS))
	}
	if row.RequestedServiceTier != "" {
		parts = append(parts, metricChip("tier", row.RequestedServiceTier))
	}
	if row.EffectiveServiceTier != "" && row.EffectiveServiceTier != row.RequestedServiceTier {
		parts = append(parts, metricChip("effective", row.EffectiveServiceTier))
	}
	if row.ReasoningEffort != "" {
		parts = append(parts, metricChip("reasoning", row.ReasoningEffort))
	}
	if row.ThinkingType != "" {
		parts = append(parts, metricChip("thinking", row.ThinkingType))
	}
	return wrappedMetricLine(width, parts...)
}

func wrappedRequestModelDisplay(row management.RequestSummary, width int) string {
	requestedProvider := row.RequestedProviderID
	requestedModel := row.RequestedModelID
	resolvedProvider := row.ResolvedProviderID
	resolvedModel := row.ResolvedModelID
	if requestedProvider == "" {
		requestedProvider = row.ProviderInstanceID
	}
	if requestedModel == "" {
		requestedModel = row.ModelID
	}
	if resolvedProvider == "" {
		resolvedProvider = row.ProviderInstanceID
	}
	if resolvedModel == "" {
		resolvedModel = row.ModelID
	}
	requested := safeWrappedRequestDisplay(requestedProvider) + "/" + safeWrappedRequestDisplay(requestedModel)
	resolved := safeWrappedRequestDisplay(resolvedProvider) + "/" + safeWrappedRequestDisplay(resolvedModel)
	if requested != resolved {
		return strings.Join(wrapDisplayChunks(requested+" -> "+resolved, width), "\n")
	}
	return strings.Join(wrapDisplayChunks(resolved, width), "\n")
}

func safeWrappedRequestDisplay(value string) string {
	return safeWrappedDisplay(value)
}
