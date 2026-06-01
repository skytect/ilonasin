package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) writeFallbackPolicies(b *strings.Builder) {
	if m.upstreams == nil && m.snapshot == nil {
		return
	}
	width := m.viewWidth()
	b.WriteString("\n")
	b.WriteString(renderSectionBanner(width, "Credential groups", fmt.Sprintf("groups %d", len(m.fallbackPolicies))))
	b.WriteByte('\n')
	if len(m.fallbackPolicies) == 0 {
		b.WriteString("No credential group metadata.\n")
		return
	}
	cards := make([]string, 0, len(m.fallbackPolicies))
	for _, row := range m.fallbackPolicies {
		state := "disabled"
		accent := lipgloss.Color("160")
		if row.Enabled {
			state = "enabled"
			accent = lipgloss.Color("42")
		}
		explicit := "default"
		if row.Explicit {
			explicit = "explicit"
		}
		lines := []string{
			cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)+" "+safeDisplay(row.GroupLabel)) + " " + statusBadge(state),
			metricLine(
				metricChip("kind", row.CredentialKind),
				metricChip("credentials", fmt.Sprintf("%d", row.CredentialCount)),
				metricChip("policy", explicit),
			),
		}
		cards = append(cards, renderMetricAccentCard(metricCardWidth(width), accent, lines...))
	}
	b.WriteString(renderMetricCardGrid(width, cards))
	b.WriteByte('\n')
}
