package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

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
	if len(m.requestRows) > 0 {
		b.WriteString(requestTableHeader(width))
		b.WriteByte('\n')
	}
	for _, row := range m.requestRows {
		b.WriteString(requestSummaryRow(row, now, width))
		b.WriteString("\n\n")
	}
}

func requestSummaryRow(row management.RequestSummary, nowTime time.Time, width int) string {
	state := statusState(row.HTTPStatus, row.ErrorClass)
	head := requestTableRow(row, nowTime, state, width)
	model := wrappedDisplayField("model", requestModelRoute(row), width)
	tokens := wrappedMetricLine(width,
		compactTokenMixLine(row.PromptTokens, row.CompletionTokens, row.ReasoningTokens, row.CacheHitTokens, row.CacheMissTokens, row.CacheWriteTokens, width),
		metricChip("total", compactInt(row.TotalTokens)),
		compactRateBars(width, rateMetric{"hit", row.CacheHitRate * 100}),
	)
	retry := wrappedMetricLine(width,
		mutedStyle.Render(fullCredentialDisplay(row.CredentialID, row.CredentialLabel)),
		metricChip("try", fmt.Sprintf("%d", row.AttemptCount)),
		metricChip("auth", fmt.Sprintf("%d", row.AuthRetryCount)),
		metricChip("fb", fmt.Sprintf("%d", row.FallbackCount)),
		msText("lat", row.TotalLatencyMS),
		msText("ttft", row.TimeToFirstTokenMS),
		tpsText("tps", row.OutputTokensPerSecondTotal),
	)
	extras := requestSummaryExtras(row, width)
	lines := []string{head, model, tokens, retry}
	if extras != "" {
		lines = append(lines, extras)
	}
	return wrapTargetedLines(width, lines...)
}

func requestTableHeader(width int) string {
	columns := requestTableColumns(width)
	labels := []string{"st", "rt", "http", "time", "io", "cred", "try", "lat", "tok"}
	cells := make([]string, 0, len(columns))
	for i, column := range columns {
		cells = append(cells, fitPlainCell(labels[i], column))
	}
	return mutedStyle.Render(strings.Join(cells, " "))
}

func requestTableRow(row management.RequestSummary, nowTime time.Time, state string, width int) string {
	columns := requestTableColumns(width)
	stream := "sync"
	if row.Stream {
		stream = "sse"
	}
	route := shortEndpointDisplay(row.Endpoint)
	cells := []string{
		shortRequestState(state),
		route,
		fmt.Sprintf("%d", row.HTTPStatus),
		formatRelativeTimeNoClock(nowTime, row.StartedAt),
		stream,
		compactCredentialID(row.CredentialID),
		fmt.Sprintf("%d/%d/%d", row.AttemptCount, row.AuthRetryCount, row.FallbackCount),
		fmt.Sprintf("%dms", row.TotalLatencyMS),
		compactInt(row.TotalTokens),
	}
	for i := range cells {
		cells[i] = fitPlainCell(cells[i], columns[i])
	}
	return strings.Join(cells, " ")
}

func shortEndpointDisplay(value string) string {
	switch safeEndpointDisplay(value) {
	case "chat_completions":
		return "chat"
	case "responses":
		return "resp"
	case "anthropic_messages":
		return "msg"
	case "anthropic_count_tokens":
		return "count"
	default:
		return "unknown"
	}
}

func shortRequestState(state string) string {
	switch state {
	case "fresh":
		return "ok"
	case "warning":
		return "warn"
	case "error":
		return "err"
	default:
		return safeMetricLabel(state)
	}
}

func compactCredentialID(id int64) string {
	if id == 0 {
		return "none"
	}
	return fmt.Sprintf("%d", id)
}

func fullCredentialDisplay(id int64, label string) string {
	if id == 0 {
		return "credential none"
	}
	safe := safeFullWrappedDisplay(label)
	if safe == "" || safe == "[redacted]" {
		return fmt.Sprintf("credential %d", id)
	}
	return fmt.Sprintf("credential %d %s", id, safe)
}

func requestTableColumns(width int) []int {
	columns := []int{6, 8, 4, 8, 3, 4, 5, 5, 5}
	available := width - (len(columns) - 1)
	if available <= 0 {
		return columns
	}
	total := 0
	for _, column := range columns {
		total += column
	}
	for available < total && total > len(columns) {
		for i := range columns {
			if total <= available {
				break
			}
			if columns[i] > 1 {
				columns[i]--
				total--
			}
		}
	}
	if available > total {
		grow := available - total
		for grow > 0 {
			for _, i := range []int{1, 3, 5, 7, 8} {
				if grow == 0 {
					break
				}
				columns[i]++
				grow--
			}
		}
	}
	return columns
}

func fitPlainCell(value string, width int) string {
	value = strings.Join(strings.Fields(safeWrappedChromeDisplay(value)), " ")
	if width <= 0 {
		return value
	}
	chunks := wrapDisplayChunks(value, width)
	if len(chunks) > 0 {
		value = chunks[0]
	}
	valueWidth := ansi.StringWidth(value)
	if valueWidth < width {
		value += strings.Repeat(" ", width-valueWidth)
	}
	return value
}

func requestSummaryExtras(row management.RequestSummary, width int) string {
	parts := []string{}
	if row.FallbackReason != "" {
		parts = append(parts, wrappedMetricChip("fallback-reason", row.FallbackReason))
	}
	if endpoint := safeEndpointDisplay(row.Endpoint); endpoint != "" {
		parts = append(parts, metricChip("endpoint", endpoint))
	}
	if row.ErrorClass != "" {
		parts = append(parts, badBadgeStyle.Render(safeFullWrappedDisplay(row.ErrorClass)))
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
		parts = append(parts, wrappedMetricChip("tier", row.RequestedServiceTier))
	}
	if row.EffectiveServiceTier != "" && row.EffectiveServiceTier != row.RequestedServiceTier {
		parts = append(parts, wrappedMetricChip("effective", row.EffectiveServiceTier))
	}
	if row.ReasoningEffort != "" {
		parts = append(parts, wrappedMetricChip("reasoning", row.ReasoningEffort))
	}
	if row.ThinkingType != "" {
		parts = append(parts, wrappedMetricChip("thinking", row.ThinkingType))
	}
	return wrappedMetricBlock(width, parts...)
}

func requestModelRoute(row management.RequestSummary) string {
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
		return requested + " -> " + resolved
	}
	return resolved
}

func safeWrappedRequestDisplay(value string) string {
	return safeFullWrappedDisplay(value)
}
