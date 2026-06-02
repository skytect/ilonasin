package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"ilonasin/internal/management"
)

func (m Model) writeLocalTokens(b *strings.Builder) {
	now := m.nowTime()
	width := m.viewWidth()
	enabled, disabled := localTokenStateCounts(m.tokenRows)
	b.WriteString(renderSectionBanner(width, "Local API tokens",
		"local-api",
		fmt.Sprintf("enabled %d", enabled),
		fmt.Sprintf("disabled %d", disabled),
	))
	b.WriteByte('\n')
	if len(m.tokenRows) == 0 {
		b.WriteString(renderEmptyMetricCard(width, lipgloss.Color("110"), "downstream tokens",
			metricLine(metricChip("enabled", "0"), metricChip("disabled", "0")),
			metricLine(metricChip("scope", "local-api"), metricChip("upstream", "providers")),
		))
		b.WriteByte('\n')
	}
	for i, token := range m.tokenRows {
		b.WriteString(localTokenRow(token, i == m.selected, now, width))
		b.WriteByte('\n')
	}
	if m.revealTokenID != 0 {
		fmt.Fprintf(b, "\n%s %s %s\n",
			goodBadgeStyle.Render("created"),
			strconv.FormatInt(m.revealTokenID, 10),
			fragmentChip("token", m.revealTokenPrefix, m.revealTokenLast4))
	}
}

func localTokenRow(token management.LocalToken, selected bool, now time.Time, width int) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	state := "enabled"
	if token.Disabled {
		state = "disabled"
	}
	head := metricLine(
		statusBadge(state),
		cardTitleStyle.Render(cursor+" "+strconv.FormatInt(token.ID, 10)+" "+safeDisplay(token.Label)),
		fragmentChip("token", token.TokenPrefix, token.TokenLast4),
	)
	meta := metricLine(timeChip("created", now, token.CreatedAt), optionalTimeChip("disabled", now, token.DisabledAt))
	if meta == "" {
		return head
	}
	if width >= 96 {
		return metricLine(head, meta)
	}
	return strings.Join([]string{head, meta}, "\n")
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
