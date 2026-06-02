package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeFallbacks(b *strings.Builder) {
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(renderSectionBanner(width, "Fallback metadata", fmt.Sprintf("events %d", len(m.fallbackRows))))
	b.WriteByte('\n')
	if len(m.fallbackRows) == 0 {
		b.WriteString(renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("214"),
			cardTitleStyle.Render("fallback ledger")+" "+statusBadge("enabled"),
			metricLine(metricChip("events", "0"), metricChip("visibility", "metadata-only")),
			metricLine(metricChip("reason", "none"), metricChip("credential", "redacted")),
		))
		b.WriteByte('\n')
	}
	for _, row := range m.fallbackRows {
		b.WriteString(fallbackSummaryRow(row, now))
		b.WriteByte('\n')
	}
}

func fallbackSummaryRow(row management.FallbackSummary, now time.Time) string {
	return strings.Join([]string{
		metricLine(
			statusBadge("warning"),
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)),
			metricChip("model", row.ModelID),
			timeChip("at", now, row.OccurredAt),
			metricChip("reason", row.Reason),
		),
		metricLine(
			metricChip("from", credentialDisplay(row.FromCredentialID, row.FromCredentialLabel)),
			metricChip("to", credentialDisplay(row.ToCredentialID, row.ToCredentialLabel)),
		),
	}, "\n")
}
