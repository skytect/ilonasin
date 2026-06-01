package tui

import (
	"fmt"
	"strconv"
	"strings"

	"ilonasin/internal/management"
)

func (m Model) writeLocalTokens(b *strings.Builder) {
	now := m.nowTime()
	width := m.viewWidth()
	enabled, disabled := localTokenStateCounts(m.tokenRows)
	b.WriteString(renderSectionBanner(width, "Local API tokens",
		fmt.Sprintf("enabled %d", enabled),
		fmt.Sprintf("disabled %d", disabled),
	))
	b.WriteByte('\n')
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
		line := metricLine(
			cardTitleStyle.Render(cursor+" "+strconv.FormatInt(token.ID, 10)+" "+safeDisplay(token.Label)),
			statusBadge(state),
			fragmentChip("token", token.TokenPrefix, token.TokenLast4),
			timeChip("created", now, token.CreatedAt),
		)
		if token.DisabledAt != nil {
			line = metricLine(line, optionalTimeChip("disabled", now, token.DisabledAt))
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if m.revealTokenID != 0 {
		fmt.Fprintf(b, "\n%s %s %s\n",
			goodBadgeStyle.Render("created"),
			strconv.FormatInt(m.revealTokenID, 10),
			fragmentChip("token", m.revealTokenPrefix, m.revealTokenLast4))
	}
}

func localTokenStateCounts(rows []management.LocalToken) (int, int) {
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
