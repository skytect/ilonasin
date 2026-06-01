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
