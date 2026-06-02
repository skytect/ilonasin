package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeRecentRequests(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(renderSectionBanner(width, "Request metadata", fmt.Sprintf("recent %d", len(m.requestRows))))
	b.WriteByte('\n')
	if len(m.requestRows) == 0 {
		b.WriteString(renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("42"),
			cardTitleStyle.Render("metadata ledger")+" "+statusBadge("enabled"),
			metricLine(metricChip("recent", "0"), metricChip("visibility", "metadata-only")),
			metricLine(metricChip("content", "redacted"), metricChip("io", ioCaptureMode(m.runtime.CaptureIO))),
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
	head := metricLine(
		statusBadge(state),
		endpointMetricChip("route", row.Endpoint),
		metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
		timeChip("at", nowTime, row.StartedAt),
		streamChip(row.Stream),
		cardTitleStyle.Render(requestModelDisplay(row)),
	)
	tokens := metricLine(
		compactTokenMixLine(row.PromptTokens, row.CompletionTokens, row.ReasoningTokens, row.CacheHitTokens, row.CacheMissTokens, row.CacheWriteTokens, width),
		metricChip("total", compactInt(row.TotalTokens)),
	)
	timing := metricLine(
		mutedStyle.Render(credentialDisplay(row.CredentialID, row.CredentialLabel)),
		metricChip("try", fmt.Sprintf("%d", row.AttemptCount)),
		metricChip("auth", fmt.Sprintf("%d", row.AuthRetryCount)),
		metricChip("fb", fmt.Sprintf("%d", row.FallbackCount)),
	)
	performance := metricLine(
		msText("lat", row.TotalLatencyMS),
		msText("ttft", row.TimeToFirstTokenMS),
		tpsText("tps", row.OutputTokensPerSecondTotal),
		compactRateBars(width, rateMetric{"hit", row.CacheHitRate * 100}),
	)
	extras := requestSummaryExtras(row, width)
	lines := []string{head, tokens, timing, performance}
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
	return metricLine(parts...)
}
