package tui

import (
	"fmt"
	"strings"
)

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
