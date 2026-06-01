package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) writeRecentRequests(b *strings.Builder) {
	b.WriteString("\nRecent requests\n")
	width := m.viewWidth()
	now := m.nowTime()
	if len(m.requestRows) == 0 {
		b.WriteString("No request metadata.\n")
	}
	cards := make([]string, 0, len(m.requestRows))
	for _, row := range m.requestRows {
		credential := credentialDisplay(row.CredentialID, row.CredentialLabel)
		options := []string{}
		if row.RequestedServiceTier != "" {
			options = append(options, metricChip("tier", row.RequestedServiceTier))
		}
		if row.EffectiveServiceTier != "" && row.EffectiveServiceTier != row.RequestedServiceTier {
			options = append(options, metricChip("effective", row.EffectiveServiceTier))
		}
		if row.ReasoningEffort != "" {
			options = append(options, metricChip("reasoning", row.ReasoningEffort))
		}
		if row.ThinkingType != "" {
			options = append(options, metricChip("thinking", row.ThinkingType))
		}
		state := statusState(row.HTTPStatus, row.ErrorClass)
		accent := lipgloss.Color("42")
		if state == "warning" {
			accent = lipgloss.Color("214")
		}
		if state == "error" {
			accent = lipgloss.Color("160")
		}
		lines := []string{
			cardTitleStyle.Render(requestModelDisplay(row)) + " " + statusBadge(state),
			metricLine(
				endpointMetricChip("route", row.Endpoint),
				metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
				timeChip("at", now, row.StartedAt),
				streamChip(row.Stream),
			),
			metricLine(
				mutedStyle.Render(credential),
				metricChip("attempts", fmt.Sprintf("%d", row.AttemptCount)),
				metricChip("auth", fmt.Sprintf("%d", row.AuthRetryCount)),
				metricChip("fallback", fmt.Sprintf("%d", row.FallbackCount)),
			),
			metricLine(
				metricChip("in", compactInt(row.PromptTokens)),
				metricChip("out", compactInt(row.CompletionTokens)),
				metricChip("total", compactInt(row.TotalTokens)),
				metricChip("reason", compactInt(row.ReasoningTokens)),
			),
			metricLine(
				msText("lat", row.TotalLatencyMS),
				msText("up", row.UpstreamLatencyMS),
				msText("ttft", row.TimeToFirstTokenMS),
				tpsText("tps", row.OutputTokensPerSecondTotal),
			),
			percentGaugeLine("cache", row.CacheHitRate*100, width),
		}
		if row.FallbackReason != "" {
			lines = append(lines, metricChip("reason", row.FallbackReason))
		}
		if row.ErrorClass != "" {
			lines = append(lines, badBadgeStyle.Render(safeDisplay(row.ErrorClass)))
		}
		if !narrowMetrics(width) {
			lines = append(lines, metricLine(
				metricChip("messages", fmt.Sprintf("%d", row.MessageCount)),
				metricChip("tools", fmt.Sprintf("%d", row.ToolCount)),
				metricChip("images", fmt.Sprintf("%d", row.ImageCount)),
			))
		}
		if len(options) > 0 {
			lines = append(lines, metricLine(options...))
		}
		cards = append(cards, renderMetricAccentCard(metricCardWidth(width), accent, lines...))
	}
	if len(cards) > 0 {
		b.WriteString(renderMetricCardGrid(width, cards))
		b.WriteByte('\n')
	}
}
