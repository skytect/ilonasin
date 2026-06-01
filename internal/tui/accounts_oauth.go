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
	for i, row := range m.oauthRows {
		cursor := " "
		if i == m.oauthSelected {
			cursor = ">"
		}
		state := "enabled"
		if row.Disabled {
			state = "disabled"
		}
		expires := "none"
		if row.ExpiresAt != nil {
			expires = formatTime(*row.ExpiresAt)
		}
		refresh := safeRefreshFailureClass(row.RefreshFailureClass)
		if refresh == "" {
			refresh = "none"
		}
		refreshDescription := safeRefreshFailureDescriptionDisplay(row.RefreshFailureDescription)
		if refreshDescription != "" {
			refresh = refresh + " " + refreshDescription
		}
		fmt.Fprintf(b, "%s %d %s oauth account %s plan %s expires %s refresh %s %s\n",
			cursor, row.ID, safeDisplay(row.ProviderInstanceID), safeDisplay(row.AccountDisplayLabel),
			safeDisplay(row.PlanLabel), expires, refresh, state)
	}
	b.WriteString("\nProvider accounts\n")
	if len(m.accountRows) == 0 {
		b.WriteString("No provider accounts.\n")
	}
	for _, row := range m.accountRows {
		fmt.Fprintf(b, "- %s credential %d %s plan %s\n",
			safeDisplay(row.ProviderInstanceID), row.CredentialID,
			safeDisplay(row.DisplayLabel), safeDisplay(row.PlanLabel))
	}
}
