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
	if line := routingPolicyLine(m.routingPolicy, width); line != "" {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if len(m.credentialPoolGroups) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("220"), "credential groups",
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

func routingPolicyLine(policy management.RoutingPolicyStatus, width int) string {
	if policy.Scope == "" {
		return ""
	}
	body := "no"
	if policy.ExposesBodyValues {
		body = "yes"
	}
	return wrappedMetricLine(width,
		statusBadge("routing"),
		machineChip("scope", routingPolicyDisplay(policy.Scope)),
		machineChip("pool", routingPolicyDisplay(policy.Pooling)),
		machineChip("affinity", routingPolicyDisplay(policy.Affinity)),
		machineChip("cache", routingPolicyDisplay(policy.Cache)),
		machineChip("pressure", routingPolicyDisplay(policy.Pressure)),
		machineChip("tie", routingPolicyDisplay(policy.TieBreaker)),
		machineChip("quota", routingPolicyDisplay(policy.Quota)),
		machineChip("fallback", routingPolicyDisplay(policy.Fallback)),
		machineChip("body", body),
	)
}

func routingPolicyDisplay(value string) string {
	switch value {
	case "safe-client-signal-or-local-token-route":
		return "safe-signal-or-local-route"
	case "token-scoped-cursor":
		return "local-cursor"
	case "safe-prompt-cache-key-preferred":
		return "cache-key-preferred"
	default:
		return value
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
