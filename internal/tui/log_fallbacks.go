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
			b.WriteString("\n\n")
		}
		b.WriteString(fallbackSummaryRow(row, now, width))
		b.WriteByte('\n')
	}
}

func fallbackSummaryRow(row management.FallbackSummary, now time.Time, width int) string {
	lines := []string{
		fallbackTableRow(row, now, width),
		wrappedDisplayField("route", safeFullWrappedDisplay(row.ProviderInstanceID)+"/"+safeFullWrappedDisplay(row.ModelID), width),
		wrappedDisplayField("reason", safeFullWrappedDisplay(row.Reason), width),
		requestDetailLine(width, "credentials",
			mutedStyle.Render(labeledFullCredentialDisplay("from", row.FromCredentialID, row.FromCredentialLabel)),
			mutedStyle.Render(labeledFullCredentialDisplay("to", row.ToCredentialID, row.ToCredentialLabel)),
		),
	}
	return wrapTargetedLinesPreserveBlank(width, lines...)
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
	detail := safeFullWrappedDisplay(row.ProviderInstanceID)
	if row.ModelID != "" {
		detail += "/" + safeFullWrappedDisplay(row.ModelID)
	}
	cells := []string{
		"warn",
		formatRelativeTimeNoClock(now, row.OccurredAt),
		compactCredentialID(row.FromCredentialID),
		compactCredentialID(row.ToCredentialID),
		detail,
	}
	for i := range cells {
		cells[i] = fitPlainCellFirstLine(cells[i], columns[i])
	}
	return strings.Join(cells, " ")
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

func labeledFullCredentialDisplay(label string, id int64, value string) string {
	label = safeMetricLabel(label)
	if label == "" {
		label = "credential"
	}
	return label + " " + fullCredentialDisplay(id, value)
}
