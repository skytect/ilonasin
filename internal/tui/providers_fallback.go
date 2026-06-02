package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeFallbackPolicies(b *strings.Builder) {
	if m.upstreams == nil && m.snapshot == nil {
		return
	}
	width := m.viewWidth()
	b.WriteString(renderSectionBanner(width, "Credential groups", fmt.Sprintf("groups %d", len(m.fallbackPolicies))))
	b.WriteByte('\n')
	if len(m.fallbackPolicies) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "credential groups",
			metricLine(metricChip("groups", "0"), metricChip("policy", "default")),
			metricLine(metricChip("fallback", "same-provider"), metricChip("scope", "metadata")),
		))
		b.WriteByte('\n')
		return
	}
	for _, row := range m.fallbackPolicies {
		b.WriteString(fallbackPolicyRow(row))
		b.WriteByte('\n')
	}
}

func fallbackPolicyRow(row management.FallbackPolicy) string {
	state := "disabled"
	if row.Enabled {
		state = "enabled"
	}
	explicit := "default"
	if row.Explicit {
		explicit = "explicit"
	}
	return metricLine(
		statusBadge(state),
		cardTitleStyle.Render(safeDisplay(row.ProviderInstanceID)+" "+safeDisplay(row.GroupLabel)),
		metricChip("kind", row.CredentialKind),
		metricChip("credentials", fmt.Sprintf("%d", row.CredentialCount)),
		metricChip("policy", explicit),
	)
}
