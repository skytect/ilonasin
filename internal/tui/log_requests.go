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
		if separator := requestTableSeparator(width); separator != "" {
			b.WriteString(separator)
			b.WriteByte('\n')
		}
	}
	for index, row := range m.requestRows {
		if index > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(requestSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

func requestSummaryRow(row management.RequestSummary, nowTime time.Time, width int) string {
	state := statusState(row.HTTPStatus, row.ErrorClass)
	head := requestTableRow(row, nowTime, state, width)
	model := requestWrappedFieldLine(width, "model", requestModelRoute(row))
	tokens := requestDetailLine(width, "tokens",
		compactTokenMixLine(row.PromptTokens, row.CompletionTokens, row.ReasoningTokens, row.CacheHitTokens, row.CacheMissTokens, row.CacheWriteTokens, width),
		metricChip("total", compactInt(row.TotalTokens)),
		compactRateBars(width, rateMetric{"hit", row.CacheHitRate * 100}),
	)
	retry := requestDetailLine(width, "retry",
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
	return wrapTargetedLinesPreserveBlank(width, lines...)
}

func requestWrappedFieldLine(width int, label, value string) string {
	return wrappedDisplayField(label, value, width)
}

func requestDetailLine(width int, label string, parts ...string) string {
	label = safeMetricLabel(label)
	if label == "" {
		label = "detail"
	}
	prefix := mutedStyle.Render(label)
	body := wrappedMetricLine(maxInt(1, width-len(label)-1), parts...)
	if body == "" {
		return prefix
	}
	bodyLines := splitBodyLines(body)
	indent := strings.Repeat(" ", len(label)+1)
	lines := make([]string, 0, len(bodyLines))
	for i, line := range bodyLines {
		if i == 0 {
			lines = append(lines, prefix+" "+line)
			continue
		}
		lines = append(lines, indent+line)
	}
	return strings.Join(lines, "\n")
}

func requestTableHeader(width int) string {
	columns := requestTableColumns(width)
	labels := requestTableLabels(columns)
	cells := make([]string, 0, len(columns))
	for i, column := range columns {
		cells = append(cells, fitPlainCellFirstLine(labels[i], column))
	}
	return mutedStyle.Render(strings.Join(cells, " "))
}

func requestTableSeparator(width int) string {
	if width <= 0 {
		return ""
	}
	columns := requestTableColumns(width)
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		if column < 1 {
			column = 1
		}
		parts = append(parts, strings.Repeat("-", column))
	}
	line := strings.Join(parts, " ")
	return mutedStyle.Render(line)
}

func requestTableRow(row management.RequestSummary, nowTime time.Time, state string, width int) string {
	columns := requestTableColumns(width)
	stream := "sync"
	if row.Stream {
		stream = "sse"
	}
	route := shortEndpointDisplay(row.Endpoint)
	detail := compactRequestTableDetail(row)
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
	if len(columns) > len(cells) {
		cells = append(cells, detail)
	}
	return wrappedPlainTableRow(cells, columns)
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
	if width < 96 {
		return fitTableColumns(width, []int{4, 5, 4, 7, 3, 4, 5, 5, 5}, []int{3, 4, 3, 5, 3, 3, 5, 4, 4}, []int{3, 7, 8})
	}
	columns := []int{6, 8, 4, 8, 3, 4, 5, 5, 5, 24}
	return fitTableColumns(width, columns, []int{3, 4, 3, 5, 3, 3, 5, 4, 4, 12}, []int{9, 1, 3, 5, 7, 8})
}

func requestTableLabels(columns []int) []string {
	labels := []string{"st", "rt", "http", "time", "io", "cred", "try", "lat", "tok"}
	if len(columns) > len(labels) {
		labels = append(labels, "model")
	}
	return labels
}

func fitTableColumns(width int, columns, minimums, growOrder []int) []int {
	out := append([]int(nil), columns...)
	if len(minimums) != len(out) {
		minimums = make([]int, len(out))
		for i := range minimums {
			minimums[i] = 1
		}
	}
	available := width - (len(out) - 1)
	if available <= 0 {
		return out
	}
	total := 0
	for _, column := range out {
		total += column
	}
	for available < total {
		changed := false
		for i := range out {
			if total <= available {
				break
			}
			if out[i] > minimums[i] {
				out[i]--
				total--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if available > total {
		grow := available - total
		for grow > 0 {
			for _, i := range growOrder {
				if grow == 0 {
					break
				}
				if i < 0 || i >= len(out) {
					continue
				}
				out[i]++
				grow--
			}
		}
	}
	return out
}

func compactRequestTableDetail(row management.RequestSummary) string {
	model := row.ResolvedModelID
	if model == "" {
		model = row.ModelID
	}
	provider := row.ResolvedProviderID
	if provider == "" {
		provider = row.ProviderInstanceID
	}
	if model == "" {
		return safeWrappedRequestDisplay(provider)
	}
	if provider == "" {
		return safeWrappedRequestDisplay(model)
	}
	return safeWrappedRequestDisplay(provider) + "/" + safeWrappedRequestDisplay(model)
}

func fitPlainCellFirstLine(value string, width int) string {
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

func wrappedPlainTableRow(cells []string, columns []int) string {
	if len(cells) == 0 || len(columns) == 0 {
		return ""
	}
	cellLines := make([][]string, 0, len(cells))
	rowHeight := 1
	for i := 0; i < len(cells) && i < len(columns); i++ {
		lines := wrapPlainTableCell(cells[i], columns[i])
		if len(lines) > rowHeight {
			rowHeight = len(lines)
		}
		cellLines = append(cellLines, lines)
	}
	out := make([]string, 0, rowHeight)
	for lineIndex := 0; lineIndex < rowHeight; lineIndex++ {
		parts := make([]string, 0, len(cellLines))
		for columnIndex, lines := range cellLines {
			value := ""
			if lineIndex < len(lines) {
				value = lines[lineIndex]
			}
			parts = append(parts, padPlainCell(value, columns[columnIndex]))
		}
		out = append(out, strings.TrimRight(strings.Join(parts, " "), " "))
	}
	return strings.Join(out, "\n")
}

func wrapPlainTableCell(value string, width int) []string {
	value = strings.Join(strings.Fields(safeWrappedChromeDisplay(value)), " ")
	if value == "" {
		value = "none"
	}
	if width <= 0 {
		return []string{value}
	}
	chunks := wrapDisplayChunks(value, width)
	if len(chunks) == 0 {
		return []string{""}
	}
	return chunks
}

func padPlainCell(value string, width int) string {
	if width <= 0 {
		return value
	}
	valueWidth := ansi.StringWidth(value)
	if valueWidth < width {
		return value + strings.Repeat(" ", width-valueWidth)
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
