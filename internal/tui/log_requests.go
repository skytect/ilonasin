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
	if len(m.requestRows) > 0 {
		b.WriteString(requestOverviewBlock(m.requestRows, m.runtime.CaptureIO, width))
		b.WriteByte('\n')
		requestColumns := requestTableColumns(width)
		b.WriteString(plainTableHeader(requestTableLabels(requestColumns), requestColumns))
		b.WriteByte('\n')
		if separator := plainTableSeparator(width, requestColumns); separator != "" {
			b.WriteString(separator)
			b.WriteByte('\n')
		}
	}
	for index, row := range m.requestRows {
		if index > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(requestSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

type requestOverview struct {
	OK                int
	Warning           int
	Error             int
	Chat              int
	Responses         int
	Messages          int
	CountTokens       int
	Unknown           int
	PromptTokens      int
	CompletionTokens  int
	ReasoningTokens   int
	CacheHitTokens    int
	CacheMissTokens   int
	CacheWriteTokens  int
	TotalTokens       int
	AverageLatencyMS  int64
	AverageTTFTMS     int64
	StreamCount       int
	TotalRequestCount int
}

func requestOverviewBlock(rows []management.RequestSummary, captureIO bool, width int) string {
	overview := requestOverviewFromRows(rows)
	if overview.TotalRequestCount == 0 {
		return ""
	}
	return strings.Join([]string{
		wrappedMetricLine(width,
			statusBadge(logOverviewState(overview)),
			metricChip("recent", compactInt(overview.TotalRequestCount)),
			metricChip("ok", compactInt(overview.OK)),
			metricChip("warn", compactInt(overview.Warning)),
			metricChip("err", compactInt(overview.Error)),
			metricChip("io", ioCaptureMode(captureIO)),
		),
		wrappedMetricLine(width,
			mutedStyle.Render("routes"),
			metricChip("chat", compactInt(overview.Chat)),
			metricChip("resp", compactInt(overview.Responses)),
			metricChip("msg", compactInt(overview.Messages)),
			metricChip("count", compactInt(overview.CountTokens)),
			metricChip("unknown", compactInt(overview.Unknown)),
			metricChip("sse", compactInt(overview.StreamCount)),
		),
		compactTokenMixLine(overview.PromptTokens, overview.CompletionTokens, overview.ReasoningTokens, overview.CacheHitTokens, overview.CacheMissTokens, overview.CacheWriteTokens, width),
		wrappedMetricLine(width,
			mutedStyle.Render("latency"),
			durationBar("avg", overview.AverageLatencyMS, 10_000, compactMetricBarWidth(width)),
			durationBar("ttft", overview.AverageTTFTMS, 5_000, compactMetricBarWidth(width)),
			metricChip("tokens", compactInt(overview.TotalTokens)),
		),
	}, "\n")
}

func requestOverviewFromRows(rows []management.RequestSummary) requestOverview {
	var overview requestOverview
	latencyTotal := int64(0)
	ttftTotal := int64(0)
	for _, row := range rows {
		overview.TotalRequestCount++
		switch statusState(row.HTTPStatus, row.ErrorClass) {
		case "error":
			overview.Error++
		case "warning":
			overview.Warning++
		default:
			overview.OK++
		}
		switch shortEndpointDisplay(row.Endpoint) {
		case "chat":
			overview.Chat++
		case "resp":
			overview.Responses++
		case "msg":
			overview.Messages++
		case "count":
			overview.CountTokens++
		default:
			overview.Unknown++
		}
		if row.Stream {
			overview.StreamCount++
		}
		overview.PromptTokens += row.PromptTokens
		overview.CompletionTokens += row.CompletionTokens
		overview.ReasoningTokens += row.ReasoningTokens
		overview.CacheHitTokens += row.CacheHitTokens
		overview.CacheMissTokens += row.CacheMissTokens
		overview.CacheWriteTokens += row.CacheWriteTokens
		overview.TotalTokens += row.TotalTokens
		latencyTotal += row.TotalLatencyMS
		ttftTotal += row.TimeToFirstTokenMS
	}
	if overview.TotalRequestCount > 0 {
		overview.AverageLatencyMS = latencyTotal / int64(overview.TotalRequestCount)
		overview.AverageTTFTMS = ttftTotal / int64(overview.TotalRequestCount)
	}
	return overview
}

func logOverviewState(overview requestOverview) string {
	switch {
	case overview.Error > 0:
		return "error"
	case overview.Warning > 0:
		return "warning"
	default:
		return "fresh"
	}
}

func requestSummaryRow(row management.RequestSummary, nowTime time.Time, width int) string {
	state := statusState(row.HTTPStatus, row.ErrorClass)
	head := requestTableRow(row, nowTime, state, width)
	lines := []string{head, requestDetailRows(row, width)}
	return wrapTargetedLinesPreserveBlank(width, lines...)
}

func requestDetailRows(row management.RequestSummary, width int) string {
	return logDetailRows(requestDetailFields(row), width)
}

func requestDetailFields(row management.RequestSummary) []logDetailField {
	fields := []logDetailField{
		{label: "route", value: requestModelRoute(row)},
		{label: "cred", value: fullCredentialDisplay(row.CredentialID, row.CredentialLabel)},
		{label: "tokens", value: requestTokenDetail(row)},
		{label: "timing", value: requestTimingDetail(row)},
		{label: "tries", value: requestAttemptDetail(row)},
		{label: "inputs", value: requestInputDetail(row)},
	}
	if row.ErrorClass != "" {
		fields = append(fields, logDetailField{label: "error", value: row.ErrorClass})
	}
	if row.FallbackReason != "" {
		fields = append(fields, logDetailField{label: "fallback", value: row.FallbackReason})
	}
	if endpoint := safeEndpointDisplay(row.Endpoint); endpoint != "" {
		fields = append(fields, logDetailField{label: "endpoint", value: endpoint})
	}
	if row.RequestedServiceTier != "" {
		fields = append(fields, logDetailField{label: "tier", value: row.RequestedServiceTier})
	}
	if row.EffectiveServiceTier != "" && row.EffectiveServiceTier != row.RequestedServiceTier {
		fields = append(fields, logDetailField{label: "effective", value: row.EffectiveServiceTier})
	}
	if row.ReasoningEffort != "" {
		fields = append(fields, logDetailField{label: "reasoning", value: row.ReasoningEffort})
	}
	if row.ThinkingType != "" {
		fields = append(fields, logDetailField{label: "thinking", value: row.ThinkingType})
	}
	return fields
}

func requestTokenDetail(row management.RequestSummary) string {
	parts := []string{
		"in " + compactInt(row.PromptTokens),
		"out " + compactInt(row.CompletionTokens),
		"reason " + compactInt(row.ReasoningTokens),
		"cache-hit " + compactInt(row.CacheHitTokens),
		"cache-miss " + compactInt(row.CacheMissTokens),
		"cache-write " + compactInt(row.CacheWriteTokens),
		"total " + compactInt(row.TotalTokens),
		"hit " + compactPercentText(row.CacheHitRate*100),
	}
	return strings.Join(parts, "  ")
}

func requestTimingDetail(row management.RequestSummary) string {
	return strings.Join([]string{
		msText("lat", row.TotalLatencyMS),
		msText("up", row.UpstreamLatencyMS),
		msText("ttft", row.TimeToFirstTokenMS),
		tpsText("tps", row.OutputTokensPerSecondTotal),
	}, "  ")
}

func requestAttemptDetail(row management.RequestSummary) string {
	return fmt.Sprintf("attempts %d  auth %d  fallbacks %d", row.AttemptCount, row.AuthRetryCount, row.FallbackCount)
}

func requestInputDetail(row management.RequestSummary) string {
	return fmt.Sprintf("messages %d  tools %d  images %d", row.MessageCount, row.ToolCount, row.ImageCount)
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

func compactRequestTableDetail(row management.RequestSummary) string {
	model := row.ResolvedModelID
	if model == "" {
		model = row.ModelID
	}
	provider := row.ResolvedProviderID
	if provider == "" {
		provider = row.ProviderInstanceID
	}
	return requestRouteDisplay(provider, model)
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
	requested := requestRouteDisplay(requestedProvider, requestedModel)
	resolved := requestRouteDisplay(resolvedProvider, resolvedModel)
	if requested != resolved {
		return requested + " -> " + resolved
	}
	return resolved
}

func requestRouteDisplay(provider, model string) string {
	provider = safeWrappedRequestDisplay(provider)
	model = safeWrappedRequestDisplay(model)
	if model == "" {
		return provider
	}
	if provider == "" {
		return model
	}
	return provider + "/" + model
}

func safeWrappedRequestDisplay(value string) string {
	return safeFullWrappedDisplay(value)
}
