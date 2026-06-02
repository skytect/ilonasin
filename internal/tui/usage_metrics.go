package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeUsageMetrics(b *strings.Builder) {
	width := m.viewWidth()
	b.WriteString(renderPaneSubhead(width, "Token usage", fmt.Sprintf("providers %d", len(m.usageRows))))
	b.WriteByte('\n')
	if len(m.usageRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("42"), "token ledger",
			metricLine(metricChip("providers", "0"), metricChip("requests", "0")),
			metricLine(metricChip("tokens", "0"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.usageRows {
		b.WriteString(usageSummaryRow(row, width))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Performance", fmt.Sprintf("providers %d", len(m.latencyRows))))
	b.WriteByte('\n')
	if len(m.latencyRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "performance ledger",
			metricLine(metricChip("providers", "0"), metricChip("requests", "0")),
			metricLine(msText("lat", 0), msText("ttft", 0), tpsText("tps", 0)),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.latencyRows {
		b.WriteString(latencySummaryRow(row, width))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Streams", fmt.Sprintf("states %d", len(m.streamRows))))
	b.WriteByte('\n')
	if len(m.streamRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "stream ledger",
			metricLine(metricChip("states", "0"), metricChip("streams", "0")),
			metricLine(metricChip("chunks", "0"), metricChip("status", "none")),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.streamRows {
		b.WriteString(streamSummaryRow(row))
		b.WriteByte('\n')
	}
}

func usageSummaryRow(row management.UsageSummary, width int) string {
	lines := []string{
		metricLine(
			statusBadge("fresh"),
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)),
			metricChip("requests", fmt.Sprintf("%d", row.RequestCount)),
			metricChip("total", compactInt(row.TotalTokens)),
			metricChip("cost", compactInt64(row.CostMicrounits)+"u"),
		),
		compactTokenMixLine(row.PromptTokens, row.CompletionTokens, row.ReasoningTokens, row.CacheHitTokens, row.CacheMissTokens, row.CacheWriteTokens, width),
	}
	lines = append(lines, usageRateLines(width,
		rateMetric{"hit", row.CacheHitRate * 100},
		rateMetric{"miss", row.CacheMissRate * 100},
		rateMetric{"write", row.CacheWriteRate * 100},
		rateMetric{"reason", row.ReasoningTokenRate * 100},
	)...)
	return strings.Join(lines, "\n")
}

func usageRateLines(width int, rates ...rateMetric) []string {
	if width >= 88 || len(rates) <= 2 {
		return []string{compactRateBars(width, rates...)}
	}
	return []string{
		compactRateBars(width, rates[:2]...),
		compactRateBars(width, rates[2:]...),
	}
}

func latencySummaryRow(row management.LatencySummary, width int) string {
	state := latencyState(row.AverageLatencyMS)
	return strings.Join([]string{
		metricLine(
			statusBadge(state),
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)),
			metricChip("requests", fmt.Sprintf("%d", row.RequestCount)),
			msText("lat", row.AverageLatencyMS),
			msText("up", row.AverageUpstreamLatencyMS),
			msText("ttft", row.AverageTimeToFirstTokenMS),
		),
		strings.Join(latencyShapeLines(width, row), "\n"),
	}, "\n")
}

func latencyShapeLines(width int, row management.LatencySummary) []string {
	if width >= 128 {
		return []string{latencyShapeLine(width, row.AverageLatencyMS, row.AverageUpstreamLatencyMS, row.AverageTimeToFirstTokenMS, row.AverageOutputTPS, row.AverageOutputTPSTotal, row.AverageOutputTPSAfterTTFT)}
	}
	return []string{
		metricLine(
			mutedStyle.Render("time"),
			durationBar("lat", row.AverageLatencyMS, 10_000, compactMetricBarWidth(width)),
			durationBar("up", row.AverageUpstreamLatencyMS, 10_000, compactMetricBarWidth(width)),
			durationBar("ttft", row.AverageTimeToFirstTokenMS, 5_000, compactMetricBarWidth(width)),
		),
		metricLine(
			tpsText("output", row.AverageOutputTPS),
			tpsText("total", row.AverageOutputTPSTotal),
			tpsText("post", row.AverageOutputTPSAfterTTFT),
		),
	}
}

func streamSummaryRow(row management.StreamSummary) string {
	state := "fresh"
	if row.CompletionStatus != "completed" {
		state = "warning"
	}
	return metricLine(
		statusBadge(state),
		cardTitleStyle.Render(safeDisplay(row.CompletionStatus)),
		metricChip("streams", fmt.Sprintf("%d", row.StreamCount)),
		metricChip("chunks", compactInt(row.ChunkCount)),
	)
}
