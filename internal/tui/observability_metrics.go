package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) writeUsageMetrics(b *strings.Builder) {
	b.WriteString("\nUsage totals\n")
	width := m.viewWidth()
	if len(m.usageRows) == 0 {
		b.WriteString("No usage metadata.\n")
	}
	usageCards := make([]string, 0, len(m.usageRows))
	for _, row := range m.usageRows {
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)) + " " + statusBadge("fresh"),
			metricLine(
				metricChip("requests", fmt.Sprintf("%d", row.RequestCount)),
				metricChip("cost", compactInt64(row.CostMicrounits)+"u"),
			),
			metricLine(
				metricChip("in", compactInt(row.PromptTokens)),
				metricChip("out", compactInt(row.CompletionTokens)),
				metricChip("total", compactInt(row.TotalTokens)),
				metricChip("reason", compactInt(row.ReasoningTokens)),
			),
		}
		if narrowObservability(width) {
			lines = append(lines, metricLine(
				compactPercentMetric("hit", row.CacheHitRate*100),
				compactPercentMetric("miss", row.CacheMissRate*100),
				compactPercentMetric("reason", row.ReasoningTokenRate*100),
			))
		} else {
			lines = append(lines,
				percentGaugeLine("cache hit", row.CacheHitRate*100, width),
				percentGaugeLine("cache miss", row.CacheMissRate*100, width),
				percentGaugeLine("reason", row.ReasoningTokenRate*100, width),
			)
		}
		usageCards = append(usageCards, renderObservabilityAccentCard(observabilityCardWidth(width), lipgloss.Color("42"), lines...))
	}
	if len(usageCards) > 0 {
		b.WriteString(renderObservabilityCardGrid(width, usageCards))
		b.WriteByte('\n')
	}
	b.WriteString("\nLatency\n")
	if len(m.latencyRows) == 0 {
		b.WriteString("No latency metadata.\n")
	}
	latencyCards := make([]string, 0, len(m.latencyRows))
	for _, row := range m.latencyRows {
		state := latencyState(row.AverageLatencyMS)
		accent := lipgloss.Color("42")
		if state == "warning" {
			accent = lipgloss.Color("214")
		}
		if state == "error" {
			accent = lipgloss.Color("160")
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)) + " " + statusBadge(state),
			metricLine(metricChip("requests", fmt.Sprintf("%d", row.RequestCount))),
			metricLine(
				msText("lat", row.AverageLatencyMS),
				msText("up", row.AverageUpstreamLatencyMS),
				msText("ttft", row.AverageTimeToFirstTokenMS),
			),
			metricLine(
				tpsText("output", row.AverageOutputTPS),
				tpsText("total", row.AverageOutputTPSTotal),
				tpsText("post-ttft", row.AverageOutputTPSAfterTTFT),
			),
		}
		latencyCards = append(latencyCards, renderObservabilityAccentCard(observabilityCardWidth(width), accent, lines...))
	}
	if len(latencyCards) > 0 {
		b.WriteString(renderObservabilityCardGrid(width, latencyCards))
		b.WriteByte('\n')
	}
	b.WriteString("\nStreams\n")
	if len(m.streamRows) == 0 {
		b.WriteString("No stream metadata.\n")
	}
	streamCards := make([]string, 0, len(m.streamRows))
	for _, row := range m.streamRows {
		state := "fresh"
		if row.CompletionStatus != "completed" {
			state = "warning"
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.CompletionStatus)) + " " + statusBadge(state),
			metricLine(
				metricChip("streams", fmt.Sprintf("%d", row.StreamCount)),
				metricChip("chunks", compactInt(row.ChunkCount)),
			),
		}
		streamCards = append(streamCards, renderObservabilityAccentCard(observabilityCardWidth(width), lipgloss.Color("110"), lines...))
	}
	if len(streamCards) > 0 {
		b.WriteString(renderObservabilityCardGrid(width, streamCards))
		b.WriteByte('\n')
	}
}
