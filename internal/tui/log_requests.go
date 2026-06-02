package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

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
	for _, row := range m.requestRows {
		b.WriteString(requestSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

func requestSummaryRow(row management.RequestSummary, nowTime time.Time, width int) string {
	state := statusState(row.HTTPStatus, row.ErrorClass)
	head := wrapRequestMetricLine(width,
		statusBadge(state),
		endpointMetricChip("route", row.Endpoint),
		cardTitleStyle.Render(wrappedRequestModelDisplay(row, width)),
		metricChip("status", fmt.Sprintf("%d", row.HTTPStatus)),
		timeChip("at", nowTime, row.StartedAt),
		streamChip(row.Stream),
	)
	tokens := wrapRequestMetricLine(width,
		compactTokenMixLine(row.PromptTokens, row.CompletionTokens, row.ReasoningTokens, row.CacheHitTokens, row.CacheMissTokens, row.CacheWriteTokens, width),
		metricChip("total", compactInt(row.TotalTokens)),
		compactRateBars(width, rateMetric{"hit", row.CacheHitRate * 100}),
	)
	route := wrapRequestMetricLine(width,
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
	return wrapRequestMetricLine(width, parts...)
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
		return strings.Join(wrapRequestDisplayChunks(requested+" -> "+resolved, width), "\n")
	}
	return strings.Join(wrapRequestDisplayChunks(resolved, width), "\n")
}

func safeWrappedRequestDisplay(value string) string {
	return safeWrappedRequestDisplayWithPattern(value, unsafeDisplayPattern)
}

func safeWrappedRequestDisplayWithPattern(value string, unsafe *regexp.Regexp) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if unsafe.MatchString(value) {
		return "[redacted]"
	}
	return value
}

func wrapRequestDisplayChunks(value string, width int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if width <= 0 || ansi.StringWidth(value) <= width {
		return []string{value}
	}
	if width < 1 {
		width = 1
	}
	chunks := []string{}
	var b strings.Builder
	for _, r := range value {
		candidate := b.String() + string(r)
		if b.Len() > 0 && ansi.StringWidth(candidate) > width {
			chunks = append(chunks, b.String())
			b.Reset()
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
}

func wrapRequestMetricLine(width int, parts ...string) string {
	lines := make([]string, 0, 1)
	current := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if current == "" {
			current = part
			continue
		}
		candidate := current + "  " + part
		if width > 0 && ansi.StringWidth(candidate) > width {
			lines = append(lines, current)
			current = part
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}
