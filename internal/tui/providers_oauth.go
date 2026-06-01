package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeOAuth(b *strings.Builder) {
	if m.oauth == nil && m.snapshot == nil {
		return
	}
	b.WriteString("\nOAuth accounts\n")
	if m.oauthChallenge != nil {
		fmt.Fprintf(b, "Login %s at %s code %s\n", safeDisplay(m.oauthChallenge.ProviderInstanceID),
			safeDisplay(m.oauthChallenge.VerificationURL), safeDisplay(m.oauthChallenge.UserCode))
	}
	if len(m.oauthRows) == 0 {
		b.WriteString("No OAuth accounts.\n")
	}
	width := m.viewWidth()
	now := m.nowTime()
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
		b.WriteString(renderCard(width, lines...))
		b.WriteByte('\n')
	}
	b.WriteString("\nProvider accounts\n")
	if len(m.accountRows) == 0 {
		b.WriteString("No provider accounts.\n")
	}
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
		b.WriteString(renderCard(width, lines...))
		b.WriteByte('\n')
	}
}
