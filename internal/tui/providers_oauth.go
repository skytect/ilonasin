package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) writeOAuth(b *strings.Builder) {
	if m.oauth == nil && m.snapshot == nil {
		return
	}
	width := m.viewWidth()
	now := m.nowTime()
	b.WriteString(renderSectionBanner(width, "OAuth accounts", fmt.Sprintf("oauth %d", len(m.oauthRows)), fmt.Sprintf("accounts %d", len(m.accountRows))))
	b.WriteByte('\n')
	if m.oauthChallenge != nil {
		fmt.Fprintf(b, "%s %s %s %s\n", warnBadgeStyle.Render("login"), metricChip("provider", m.oauthChallenge.ProviderInstanceID),
			metricChip("code", m.oauthChallenge.UserCode), safeDisplay(m.oauthChallenge.VerificationURL))
	}
	if len(m.oauthRows) == 0 {
		b.WriteString("No OAuth accounts.\n")
	}
	cards := make([]string, 0, len(m.oauthRows))
	for i, row := range m.oauthRows {
		cursor := " "
		if i == m.oauthSelected {
			cursor = ">"
		}
		state := "enabled"
		if row.Disabled {
			state = "disabled"
		}
		expires := optionalTimeChip("exp", now, row.ExpiresAt)
		if expires == "" {
			expires = metricChip("exp", "none")
		}
		refresh := safeRefreshFailureClass(row.RefreshFailureClass)
		if refresh == "" {
			refresh = "none"
		}
		refreshDescription := safeRefreshFailureDescriptionDisplay(row.RefreshFailureDescription)
		title := cursor + " " + accountIdentity(row.AccountDisplayLabel, "OAuth account")
		lines := []string{
			cardTitleStyle.Render(title) + " " + statusBadge(state),
			highlightedIdentity(row.AccountDisplayLabel, "OAuth account"),
			accountMeta(
				fmt.Sprintf("credential %d", row.ID),
				safeDisplay(row.ProviderInstanceID),
				accountMetaField("plan", row.PlanLabel),
				expires,
				metricChip("refresh", refresh),
			),
		}
		if refreshDescription != "" {
			lines = append(lines, mutedStyle.Render(refreshDescription))
		}
		cards = append(cards, renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("110"), lines...))
	}
	if len(cards) > 0 {
		b.WriteString(renderMetricCardGrid(width, cards))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(renderSectionBanner(width, "Provider accounts", fmt.Sprintf("accounts %d", len(m.accountRows))))
	b.WriteByte('\n')
	if len(m.accountRows) == 0 {
		b.WriteString("No provider accounts.\n")
	}
	accountCards := make([]string, 0, len(m.accountRows))
	for _, row := range m.accountRows {
		lines := []string{
			cardTitleStyle.Render(accountIdentity(row.DisplayLabel, "provider account")),
			highlightedIdentity(row.DisplayLabel, "provider account"),
			accountMeta(
				safeDisplay(row.ProviderInstanceID),
				fmt.Sprintf("credential %d", row.CredentialID),
				accountMetaField("plan", row.PlanLabel),
			),
		}
		accountCards = append(accountCards, renderMetricAccentCard(metricCardWidth(width), lipgloss.Color("24"), lines...))
	}
	if len(accountCards) > 0 {
		b.WriteString(renderMetricCardGrid(width, accountCards))
		b.WriteByte('\n')
	}
}
