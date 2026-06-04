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
		fallbackColumns := fallbackTableColumns(width)
		b.WriteString(plainTableHeader(fallbackTableLabels(), fallbackColumns))
		b.WriteByte('\n')
		if separator := plainTableSeparator(width, fallbackColumns); separator != "" {
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
	return logSummaryRow(width, fallbackTableRow(row, now, width), logDetailRows(fallbackDetailFields(row), width))
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
	return logRouteDisplay(row.ProviderInstanceID, row.ModelID)
}

func fallbackTableRow(row management.FallbackSummary, now time.Time, width int) string {
	columns := fallbackTableColumns(width)
	cells := []string{
		"warn",
		formatRelativeTimeNoClock(now, row.OccurredAt),
		compactCredentialID(row.FromCredentialID),
		compactCredentialID(row.ToCredentialID),
		fallbackRouteDisplay(row),
	}
	return wrappedPlainTableRow(cells, columns)
}

func fallbackTableLabels() []string {
	return []string{"st", "time", "from", "to", "route"}
}

func fallbackTableColumns(width int) []int {
	return fitTableColumns(width, []int{6, 10, 8, 8, 24}, []int{1, 1, 1, 1, 1}, []int{4, 1, 2, 3})
}
