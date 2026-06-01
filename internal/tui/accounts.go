package tui

import (
	"fmt"
	"strconv"
	"strings"
)

func (m Model) writeAccounts(b *strings.Builder) {
	b.WriteString("Local API tokens\n")
	if len(m.tokenRows) == 0 {
		b.WriteString("No local API tokens.\n")
	}
	for i, token := range m.tokenRows {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		state := "enabled"
		if token.Disabled {
			state = "disabled"
		}
		fmt.Fprintf(b, "%s %d %s %s...%s %s\n", cursor, token.ID, safeDisplay(token.Label),
			safeTokenFragmentDisplay(token.TokenPrefix, 8), safeTokenFragmentDisplay(token.TokenLast4, 4), state)
	}
	if m.revealTokenID != 0 {
		fmt.Fprintf(b, "\nNew token %s created: %s...%s\n",
			strconv.FormatInt(m.revealTokenID, 10),
			safeTokenFragmentDisplay(m.revealTokenPrefix, 8), safeTokenFragmentDisplay(m.revealTokenLast4, 4))
	}
	if m.apiKeyMode {
		fmt.Fprintf(b, "\nAdding API key for %s: %s\n", m.apiKeyProvider, strings.Repeat("*", len(m.apiKeyInput)))
	}
	b.WriteString("\nUpstream credentials\n")
	if len(m.credentials) == 0 {
		b.WriteString("No upstream credentials.\n")
	}
	for _, cred := range m.credentials {
		state := "enabled"
		if cred.Disabled {
			state = "disabled"
		}
		fmt.Fprintf(b, "- %d %s %s %s...%s group %s %s\n", cred.ID, safeDisplay(cred.ProviderInstanceID),
			safeDisplay(cred.Label), safeDisplay(cred.SecretPrefix), safeDisplay(cred.SecretLast4),
			safeDisplay(cred.FallbackGroup), state)
	}
	m.writeFallbackPolicies(b)
	m.writeOAuth(b)
}

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

func (m Model) writeFallbackPolicies(b *strings.Builder) {
	if m.upstreams == nil && m.snapshot == nil {
		return
	}
	b.WriteString("\nCredential groups\n")
	if len(m.fallbackPolicies) == 0 {
		b.WriteString("No credential group metadata.\n")
	}
	for _, row := range m.fallbackPolicies {
		state := "disabled"
		if row.Enabled {
			state = "enabled"
		}
		fmt.Fprintf(b, "- %s %s %s credentials %d\n",
			safeDisplay(row.ProviderInstanceID), safeDisplay(row.GroupLabel), state, row.CredentialCount)
	}
}
