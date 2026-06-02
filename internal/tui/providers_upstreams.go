package tui

import (
	"fmt"
	"strings"

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
	cards := make([]string, 0, len(m.credentials))
	for _, cred := range m.credentials {
		state := "enabled"
		accent := lipgloss.Color("42")
		if cred.Disabled {
			state = "disabled"
			accent = lipgloss.Color("160")
		}
		lines := []string{
			cardTitleStyle.Render(fmt.Sprintf("%d %s", cred.ID, safeDisplay(cred.Label))) + " " + statusBadge(state),
			metricLine(
				metricChip("provider", cred.ProviderInstanceID),
				metricChip("kind", cred.Kind),
				metricChip("group", cred.FallbackGroup),
			),
			metricLine(
				fragmentChip("key", cred.SecretPrefix, cred.SecretLast4),
				timeChip("created", m.nowTime(), cred.CreatedAt),
			),
		}
		if cred.DisabledAt != nil {
			lines = append(lines, optionalTimeChip("disabled", m.nowTime(), cred.DisabledAt))
		}
		cards = append(cards, renderMetricAccentCard(metricCardWidth(width), accent, lines...))
	}
	b.WriteString(renderMetricCardGrid(width, cards))
	b.WriteByte('\n')
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
