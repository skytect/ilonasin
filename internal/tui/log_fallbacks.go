package tui

import (
	"fmt"
	"strings"
	"time"

	"ilonasin/internal/management"
)

func (m Model) writeFallbacks(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(renderSectionBanner(width, "Fallback metadata", fmt.Sprintf("events %d", len(m.fallbackRows))))
	b.WriteByte('\n')
	if len(m.fallbackRows) == 0 {
		b.WriteString(metricLine(
			statusBadge("enabled"),
			cardTitleStyle.Render("fallback ledger"),
			metricChip("events", "0"),
			metricChip("visibility", "metadata-only"),
			metricChip("reason", "none"),
			metricChip("credential", "redacted"),
		))
		b.WriteByte('\n')
	}
	if len(m.fallbackRows) > 0 {
		b.WriteString(fallbackTableHeader(width))
		b.WriteByte('\n')
		if separator := fallbackTableSeparator(width); separator != "" {
			b.WriteString(separator)
			b.WriteByte('\n')
		}
	}
	for index, row := range m.fallbackRows {
		if index > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fallbackSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

func fallbackSummaryRow(row management.FallbackSummary, now time.Time, width int) string {
	lines := []string{
		fallbackTableRow(row, now, width),
		logDetailRows(fallbackDetailFields(row), width),
	}
	return wrapTargetedLinesPreserveBlank(width, lines...)
}

func fallbackDetailFields(row management.FallbackSummary) []logDetailField {
	return []logDetailField{
		{label: "route", value: fallbackRouteDisplay(row)},
		{label: "reason", value: row.Reason},
		{label: "from", value: fullCredentialDisplay(row.FromCredentialID, row.FromCredentialLabel)},
		{label: "to", value: fullCredentialDisplay(row.ToCredentialID, row.ToCredentialLabel)},
	}
}

func fallbackRouteDisplay(row management.FallbackSummary) string {
	provider := safeWrappedRequestDisplay(row.ProviderInstanceID)
	model := safeWrappedRequestDisplay(row.ModelID)
	if model == "" {
		return provider
	}
	if provider == "" {
		return model
	}
	return provider + "/" + model
}

func fallbackTableHeader(width int) string {
	columns := fallbackTableColumns(width)
	labels := []string{"st", "time", "from", "to", "route"}
	cells := make([]string, 0, len(columns))
	for i, column := range columns {
		cells = append(cells, fitPlainCellFirstLine(labels[i], column))
	}
	return mutedStyle.Render(strings.Join(cells, " "))
}

func fallbackTableSeparator(width int) string {
	if width <= 0 {
		return ""
	}
	columns := fallbackTableColumns(width)
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		if column < 1 {
			column = 1
		}
		parts = append(parts, strings.Repeat("-", column))
	}
	return mutedStyle.Render(strings.Join(parts, " "))
}

func fallbackTableRow(row management.FallbackSummary, now time.Time, width int) string {
	columns := fallbackTableColumns(width)
	detail := safeWrappedRequestDisplay(row.ProviderInstanceID)
	if row.ModelID != "" {
		detail += "/" + safeWrappedRequestDisplay(row.ModelID)
	}
	cells := []string{
		"warn",
		formatRelativeTimeNoClock(now, row.OccurredAt),
		compactCredentialID(row.FromCredentialID),
		compactCredentialID(row.ToCredentialID),
		detail,
	}
	return wrappedPlainTableRow(cells, columns)
}

func fallbackTableColumns(width int) []int {
	columns := []int{6, 10, 8, 8, 24}
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
			for _, i := range []int{4, 1, 2, 3} {
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
