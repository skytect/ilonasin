package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeUsageMetrics(b *strings.Builder) {
	width := m.viewWidth()
	providerRows := combinedUsageProviderRows(m.usageRows, m.latencyRows)
	b.WriteString(renderPaneSubhead(width, "Provider usage", fmt.Sprintf("providers %d", len(providerRows))))
	b.WriteByte('\n')
	if summary := usageMetricsOverview(width, m.usageRows, m.latencyRows, m.streamRows); summary != "" {
		b.WriteString(summary)
		b.WriteByte('\n')
	}
	if len(providerRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("42"), "usage ledger",
			metricLine(metricChip("providers", "0"), metricChip("requests", "0")),
			metricLine(metricChip("tokens", "0"), metricChip("latency", "0ms"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
	}
	for index, row := range providerRows {
		if index > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(usageProviderRow(row, width))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(renderPaneSubhead(width, "Streams", fmt.Sprintf("streams %d", totalStreamCount(m.streamRows)), fmt.Sprintf("chunks %s", compactInt(totalStreamChunks(m.streamRows)))))
	b.WriteByte('\n')
	if len(m.streamRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "stream ledger",
			metricLine(metricChip("streams", "0"), metricChip("chunks", "0")),
			metricLine(metricChip("status", "quiet"), metricChip("visibility", "metadata-only")),
		))
		b.WriteByte('\n')
	}
	for index, row := range m.streamRows {
		if index > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(streamSummaryRow(row, width))
		b.WriteByte('\n')
	}
}

type usageProviderRowData struct {
	providerID string
	usage      *management.UsageSummary
	latency    *management.LatencySummary
}

func combinedUsageProviderRows(usageRows []management.UsageSummary, latencyRows []management.LatencySummary) []usageProviderRowData {
	byProvider := map[string]*usageProviderRowData{}
	for i := range usageRows {
		row := &usageRows[i]
		combined := usageProviderRowFor(byProvider, row.ProviderInstanceID)
		combined.usage = row
	}
	for i := range latencyRows {
		row := &latencyRows[i]
		combined := usageProviderRowFor(byProvider, row.ProviderInstanceID)
		combined.latency = row
	}
	out := make([]usageProviderRowData, 0, len(byProvider))
	for _, row := range byProvider {
		out = append(out, *row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].providerID < out[j].providerID
	})
	return out
}

func usageProviderRowFor(rows map[string]*usageProviderRowData, providerID string) *usageProviderRowData {
	key := "\x00" + providerID
	row := rows[key]
	if row == nil {
		row = &usageProviderRowData{providerID: providerID}
		rows[key] = row
	}
	return row
}

type usageMetricsOverviewData struct {
	Requests         int
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
	CacheHitTokens   int
	CacheMissTokens  int
	CacheWriteTokens int
	LatencyRequests  int
	AverageLatencyMS int64
	AverageTTFTMS    int64
	Streams          int
	Chunks           int
}

func usageMetricsOverview(width int, usageRows []management.UsageSummary, latencyRows []management.LatencySummary, streamRows []management.StreamSummary) string {
	data := usageMetricsOverviewDataFromRows(usageRows, latencyRows, streamRows)
	if data.Requests == 0 && data.LatencyRequests == 0 && data.Streams == 0 {
		return ""
	}
	return strings.Join([]string{
		wrappedMetricLine(width,
			statusBadge("fresh"),
			metricChip("requests", compactInt(data.Requests)),
			metricChip("tokens", compactInt(data.TotalTokens)),
			metricChip("streams", compactInt(data.Streams)),
			metricChip("chunks", compactInt(data.Chunks)),
		),
		compactTokenMixLine(data.PromptTokens, data.CompletionTokens, data.ReasoningTokens, data.CacheHitTokens, data.CacheMissTokens, data.CacheWriteTokens, width),
		usageOverviewRateLine(width, data),
		wrappedMetricLine(width,
			mutedStyle.Render("latency"),
			durationBar("avg", data.AverageLatencyMS, 10_000, compactMetricBarWidth(width)),
			durationBar("ttft", data.AverageTTFTMS, 5_000, compactMetricBarWidth(width)),
		),
	}, "\n")
}

func usageMetricsOverviewDataFromRows(usageRows []management.UsageSummary, latencyRows []management.LatencySummary, streamRows []management.StreamSummary) usageMetricsOverviewData {
	var data usageMetricsOverviewData
	requests := 0
	promptTokens := 0
	completionTokens := 0
	totalTokens := 0
	reasoningTokens := 0
	cacheHitTokens := 0
	cacheMissTokens := 0
	cacheWriteTokens := 0
	for _, row := range usageRows {
		requests += row.RequestCount
		promptTokens += row.PromptTokens
		completionTokens += row.CompletionTokens
		totalTokens += row.TotalTokens
		reasoningTokens += row.ReasoningTokens
		cacheHitTokens += row.CacheHitTokens
		cacheMissTokens += row.CacheMissTokens
		cacheWriteTokens += row.CacheWriteTokens
	}
	latencyRequests := 0
	weightedLatency := int64(0)
	weightedTTFT := int64(0)
	for _, row := range latencyRows {
		latencyRequests += row.RequestCount
		weightedLatency += row.AverageLatencyMS * int64(row.RequestCount)
		weightedTTFT += row.AverageTimeToFirstTokenMS * int64(row.RequestCount)
	}
	streams := 0
	chunks := 0
	for _, row := range streamRows {
		streams += row.StreamCount
		chunks += row.ChunkCount
	}
	avgLatency := int64(0)
	avgTTFT := int64(0)
	if latencyRequests > 0 {
		avgLatency = weightedLatency / int64(latencyRequests)
		avgTTFT = weightedTTFT / int64(latencyRequests)
	}
	data.Requests = requests
	data.PromptTokens = promptTokens
	data.CompletionTokens = completionTokens
	data.TotalTokens = totalTokens
	data.ReasoningTokens = reasoningTokens
	data.CacheHitTokens = cacheHitTokens
	data.CacheMissTokens = cacheMissTokens
	data.CacheWriteTokens = cacheWriteTokens
	data.LatencyRequests = latencyRequests
	data.AverageLatencyMS = avgLatency
	data.AverageTTFTMS = avgTTFT
	data.Streams = streams
	data.Chunks = chunks
	return data
}

func usageOverviewRateLine(width int, data usageMetricsOverviewData) string {
	return compactRateBars(width,
		rateMetric{"hit", tokenRatePercent(data.CacheHitTokens, data.PromptTokens)},
		rateMetric{"miss", tokenRatePercent(data.CacheMissTokens, data.PromptTokens)},
		rateMetric{"write", tokenRatePercent(data.CacheWriteTokens, data.PromptTokens)},
		rateMetric{"reason", tokenRatePercent(data.ReasoningTokens, data.CompletionTokens)},
	)
}

func tokenRatePercent(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator) * 100
}

func usageProviderRow(row usageProviderRowData, width int) string {
	state := "fresh"
	if row.latency != nil {
		state = latencyState(row.latency.AverageLatencyMS)
	}
	head := []string{
		statusBadge(state),
		cardTitleStyle.Render(usageProviderDisplay(row.providerID)),
	}
	if row.usage != nil {
		head = append(head,
			metricChip("requests", fmt.Sprintf("%d", row.usage.RequestCount)),
			metricChip("total", compactInt(row.usage.TotalTokens)),
			metricChip("cost", compactInt64(row.usage.CostMicrounits)+"u"),
		)
	}
	if row.latency != nil {
		if row.usage == nil {
			head = append(head, metricChip("requests", fmt.Sprintf("%d", row.latency.RequestCount)))
		}
		head = append(head,
			msText("lat", row.latency.AverageLatencyMS),
			msText("ttft", row.latency.AverageTimeToFirstTokenMS),
		)
	}
	lines := []string{wrapTargetedLines(width, wrappedMetricLine(width, head...))}
	if row.usage != nil {
		lines = append(lines,
			compactTokenMixLine(row.usage.PromptTokens, row.usage.CompletionTokens, row.usage.ReasoningTokens, row.usage.CacheHitTokens, row.usage.CacheMissTokens, row.usage.CacheWriteTokens, width),
		)
		lines = append(lines, usageRateLines(width,
			rateMetric{"hit", row.usage.CacheHitRate * 100},
			rateMetric{"miss", row.usage.CacheMissRate * 100},
			rateMetric{"write", row.usage.CacheWriteRate * 100},
			rateMetric{"reason", row.usage.ReasoningTokenRate * 100},
		)...)
	}
	if row.latency != nil {
		lines = append(lines, latencyShapeLines(width, *row.latency)...)
	}
	return strings.Join(lines, "\n")
}

func usageProviderDisplay(providerID string) string {
	display := safeFullWrappedDisplay(providerID)
	if display == "" {
		return "unknown"
	}
	return display
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

func latencyShapeLines(width int, row management.LatencySummary) []string {
	if width >= 128 {
		return []string{latencyShapeLine(width, row.AverageLatencyMS, row.AverageUpstreamLatencyMS, row.AverageTimeToFirstTokenMS, row.AverageOutputTPS, row.AverageOutputTPSTotal, row.AverageOutputTPSAfterTTFT)}
	}
	return []string{
		wrappedMetricLine(width,
			mutedStyle.Render("time"),
			durationBar("lat", row.AverageLatencyMS, 10_000, compactMetricBarWidth(width)),
			durationBar("up", row.AverageUpstreamLatencyMS, 10_000, compactMetricBarWidth(width)),
			durationBar("ttft", row.AverageTimeToFirstTokenMS, 5_000, compactMetricBarWidth(width)),
		),
		wrappedMetricLine(width,
			tpsMeter("output", row.AverageOutputTPS, width),
			tpsMeter("total", row.AverageOutputTPSTotal, width),
			tpsMeter("post", row.AverageOutputTPSAfterTTFT, width),
		),
	}
}

func streamSummaryRow(row management.StreamSummary, width int) string {
	state := "fresh"
	if row.CompletionStatus != "completed" {
		state = "warning"
	}
	return wrappedMetricLine(width,
		statusBadge(state),
		cardTitleStyle.Render(safeFullWrappedDisplay(row.CompletionStatus)),
		metricChip("streams", fmt.Sprintf("%d", row.StreamCount)),
		metricChip("chunks", compactInt(row.ChunkCount)),
	)
}

func totalStreamCount(rows []management.StreamSummary) int {
	total := 0
	for _, row := range rows {
		total += row.StreamCount
	}
	return total
}

func totalStreamChunks(rows []management.StreamSummary) int {
	total := 0
	for _, row := range rows {
		total += row.ChunkCount
	}
	return total
}
