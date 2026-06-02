package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeUpstreamCredentials(b *strings.Builder) {
	width := m.viewWidth()
	if m.apiKeyMode {
		fmt.Fprintf(b, "%s %s %s\n", warnBadgeStyle.Render("adding"), metricChip("provider", m.apiKeyProvider), strings.Repeat("*", len(m.apiKeyInput)))
	}
	enabled, disabled := upstreamCredentialStateCounts(m.credentials)
	b.WriteString(renderSectionBanner(width, "Upstream API keys",
		fmt.Sprintf("enabled %d", enabled),
		fmt.Sprintf("disabled %d", disabled),
	))
	b.WriteByte('\n')
	if len(m.credentials) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "upstream credentials",
			metricLine(metricChip("enabled", "0"), metricChip("disabled", "0")),
			metricLine(metricChip("scope", "provider-auth"), metricChip("local", "api-tab")),
		))
		b.WriteByte('\n')
		return
	}
	for _, cred := range m.credentials {
		b.WriteString(upstreamCredentialRow(cred, m.nowTime(), width))
		b.WriteByte('\n')
	}
}

func upstreamCredentialStateCounts(rows []management.UpstreamCredential) (int, int) {
	enabled := 0
	disabled := 0
	for _, row := range rows {
		if row.Disabled {
			disabled++
		} else {
			enabled++
		}
	}
	return enabled, disabled
}

func upstreamCredentialRow(cred management.UpstreamCredential, now time.Time, width int) string {
	state := "enabled"
	if cred.Disabled {
		state = "disabled"
	}
	head := metricLine(
		statusBadge(state),
		cardTitleStyle.Render(fmt.Sprintf("%d %s", cred.ID, safeDisplay(cred.Label))),
		metricChip("provider", cred.ProviderInstanceID),
		metricChip("group", cred.FallbackGroup),
		machineChip("kind", cred.Kind),
		fragmentChip("key", cred.SecretPrefix, cred.SecretLast4),
	)
	meta := metricLine(timeChip("created", now, cred.CreatedAt), optionalTimeChip("disabled", now, cred.DisabledAt))
	if meta == "" {
		return head
	}
	if width >= 96 {
		return metricLine(head, meta)
	}
	return strings.Join([]string{head, meta}, "\n")
}
