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
	for _, row := range m.fallbackRows {
		b.WriteString(fallbackSummaryRow(row, now, width))
		b.WriteString("\n\n")
	}
}

func fallbackSummaryRow(row management.FallbackSummary, now time.Time, width int) string {
	return wrappedMetricBlock(width,
		statusBadge("warning"),
		cardTitleStyle.Render(safeFullWrappedDisplay(row.ProviderInstanceID)),
		wrappedMetricChip("model", row.ModelID),
		timeChip("at", now, row.OccurredAt),
		wrappedMetricChip("reason", row.Reason),
		mutedStyle.Render(labeledFullCredentialDisplay("from", row.FromCredentialID, row.FromCredentialLabel)),
		mutedStyle.Render(labeledFullCredentialDisplay("to", row.ToCredentialID, row.ToCredentialLabel)),
	)
}

func labeledFullCredentialDisplay(label string, id int64, value string) string {
	label = safeMetricLabel(label)
	if label == "" {
		label = "credential"
	}
	return label + " " + fullCredentialDisplay(id, value)
}
