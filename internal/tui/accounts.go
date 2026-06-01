package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeAccounts(b *strings.Builder) {
	m.writeLocalTokens(b)
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
