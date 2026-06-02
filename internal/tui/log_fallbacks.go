package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	cards := make([]string, 0, len(m.fallbackRows))
	for _, row := range m.fallbackRows {
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)+"/"+safeDisplay(row.ModelID)) + " " + statusBadge("warning"),
			metricLine(
				timeChip("at", now, row.OccurredAt),
				metricChip("reason", row.Reason),
			),
			mutedStyle.Render(credentialDisplay(row.FromCredentialID, row.FromCredentialLabel)),
			valueStyle.Render("->"),
			mutedStyle.Render(credentialDisplay(row.ToCredentialID, row.ToCredentialLabel)),
		}
		cards = append(cards, renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("214"), lines...))
	}
	if len(cards) > 0 {
		b.WriteString(renderMetricCardGrid(width, cards))
		b.WriteByte('\n')
	}
}
