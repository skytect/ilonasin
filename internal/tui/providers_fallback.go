package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeCredentialPoolGroups(b *strings.Builder) {
	if m.upstreams == nil && m.snapshot == nil {
		return
	}
	width := m.viewWidth()
	b.WriteString(renderSectionBanner(width, "Credential pool groups", fmt.Sprintf("groups %d", len(m.credentialPoolGroups))))
	b.WriteByte('\n')
	if len(m.credentialPoolGroups) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "credential groups",
			metricLine(metricChip("groups", "0"), metricChip("pool", "same-provider")),
			metricLine(metricChip("scope", "metadata"), metricChip("min creds", "2")),
		))
		b.WriteByte('\n')
		return
	}
	for _, row := range m.credentialPoolGroups {
		b.WriteString(credentialPoolGroupRow(row))
		b.WriteByte('\n')
	}
}

func credentialPoolGroupRow(row management.CredentialPoolGroup) string {
	return metricLine(
		statusBadge("pool"),
		cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)+" "+safeDisplay(row.GroupLabel)),
		metricChip("kind", row.CredentialKind),
		metricChip("creds", fmt.Sprintf("%d", row.CredentialCount)),
		metricChip("scope", "same-provider"),
	)
}
