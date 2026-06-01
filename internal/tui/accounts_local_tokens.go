package tui

import (
	"fmt"
	"strconv"
	"strings"
)

func (m Model) writeLocalTokens(b *strings.Builder) {
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
}
