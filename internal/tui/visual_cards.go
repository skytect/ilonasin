package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func renderCard(width int, lines ...string) string {
	return renderAccentCard(width, lipgloss.Color("238"), lines...)
}

func renderCompactCard(width int, lines ...string) string {
	style := cardStyle.MarginBottom(0)
	return renderCardWithStyle(style, width, lines...)
}

func renderSectionBanner(width int, title string, chips ...string) string {
	title = safeChromeDisplay(title)
	if title == "" {
		title = "section"
	}
	parts := []string{heroStyle.Render(title)}
	for _, chip := range chips {
		chip = safeChromeDisplay(chip)
		if chip != "" {
			parts = append(parts, chipStyle.Render(chip))
		}
	}
	line := strings.Join(parts, " ")
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "...")
}

func renderPaneSubhead(width int, title string, chips ...string) string {
	title = safeChromeDisplay(title)
	if title == "" {
		title = "group"
	}
	parts := []string{paneTitleStyle.Render(title)}
	for _, chip := range chips {
		chip = safeChromeDisplay(chip)
		if chip != "" {
			parts = append(parts, chipStyle.Render(chip))
		}
	}
	line := strings.Join(parts, " ")
	if width <= 0 || ansi.StringWidth(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "...")
}

func renderCardGrid(width int, cards []string) string {
	if len(cards) == 0 {
		return ""
	}
	if width < 160 || len(cards) == 1 {
		return strings.Join(cards, "\n")
	}
	gap := 2
	columnWidth := (width - gap) / 2
	if columnWidth < 72 {
		return strings.Join(cards, "\n")
	}
	rows := make([]string, 0, (len(cards)+1)/2)
	for i := 0; i < len(cards); i += 2 {
		left := cards[i]
		if i+1 >= len(cards) {
			rows = append(rows, left)
			continue
		}
		right := cards[i+1]
		joined := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
		if lipgloss.Width(joined) > width {
			rows = append(rows, left, right)
			continue
		}
		rows = append(rows, joined)
	}
	return strings.Join(rows, "\n")
}

func renderAccentCard(width int, accent lipgloss.Color, lines ...string) string {
	style := cardStyle.BorderForeground(accent)
	return renderCardWithStyle(style, width, lines...)
}

func renderEmptyMetricCard(width int, accent lipgloss.Color, title string, lines ...string) string {
	cardLines := []string{cardTitleStyle.Render(safeChromeDisplay(title)) + " " + statusBadge("disabled")}
	cardLines = append(cardLines, lines...)
	return renderMetricAccentCard(metricCardWidth(width), accent, cardLines...)
}

func renderCardWithStyle(style lipgloss.Style, width int, lines ...string) string {
	innerWidth := width - style.GetHorizontalFrameSize()
	if innerWidth < 8 {
		innerWidth = 8
	}
	if innerWidth > 148 {
		innerWidth = 148
	}
	bodyLines := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, part := range strings.Split(strings.TrimRight(line, "\n"), "\n") {
			bodyLines = append(bodyLines, ansi.Truncate(part, innerWidth, "..."))
		}
	}
	body := strings.TrimRight(strings.Join(bodyLines, "\n"), "\n")
	return style.Width(innerWidth).Render(body)
}
