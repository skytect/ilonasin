package tui

import "strings"

func (m Model) writeAccounts(b *strings.Builder) {
	m.writeLocalTokens(b)
	m.writeUpstreamCredentials(b)
	m.writeFallbackPolicies(b)
	m.writeOAuth(b)
}
