package tui

import (
	"fmt"
	"math"
	"net/mail"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1).
			MarginBottom(1)
	cardTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("219"))
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	labelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	valueStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	chipStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Padding(0, 1)
	goodBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("42")).Padding(0, 1)
	warnBadgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("214")).Padding(0, 1)
	badBadgeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("160")).Padding(0, 1)
	goodBarStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnBarStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	badBarStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	emptyBarStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	heroStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Padding(0, 1)
	identityStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1)
	windowStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("110")).Padding(0, 1)
)

func accountIdentity(label, fallback string) string {
	out := safeAccountDisplay(label)
	if out == "" || out == "[redacted]" {
		out = fallback
	}
	if out == "" {
		out = "account"
	}
	return out
}

func accountMeta(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, mutedStyle.Render(part))
	}
	return strings.Join(out, "  ")
}

func accountMetaField(label, value string) string {
	value = safeDisplay(value)
	if value == "" {
		value = "none"
	}
	return label + " " + value
}

func highlightedIdentity(label, fallback string) string {
	identity := safeAccountDisplay(label)
	if identity == "" {
		return warnBadgeStyle.Render("email") + " " + mutedStyle.Render("not captured")
	}
	if identity == "[redacted]" {
		return warnBadgeStyle.Render("identity") + " " + mutedStyle.Render("redacted")
	}
	field := "identity"
	if looksLikeEmail(identity) {
		field = "email"
	}
	return identityStyle.Render(field) + " " + valueStyle.Bold(true).Render(identity)
}

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(value)
	return err == nil && addr.Address == value
}

func statusBadge(state string) string {
	state = strings.TrimSpace(state)
	switch state {
	case "fresh", "enabled", "pooled":
		return goodBadgeStyle.Render(state)
	case "stale", "warning":
		return warnBadgeStyle.Render(state)
	case "disabled", "error":
		return badBadgeStyle.Render(state)
	default:
		return chipStyle.Render(safeDisplay(state))
	}
}

func metricChip(label, value string) string {
	label = safeMetricLabel(label)
	value = safeDisplay(value)
	if label == "" {
		label = "metric"
	}
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
}

func safeMetricLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_':
			out.WriteRune(r)
		}
	}
	return out.String()
}

func percentBar(value float64, width int) string {
	if width <= 0 {
		width = 16
	}
	value = boundedTUIFloat(value, 0, 100)
	filled := int(math.Round((value / 100) * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	fill := strings.Repeat("█", filled)
	empty := strings.Repeat("░", width-filled)
	style := goodBarStyle
	if value >= 85 {
		style = badBarStyle
	} else if value >= 65 {
		style = warnBarStyle
	}
	return style.Render(fill) + emptyBarStyle.Render(empty)
}

func remainingBar(value float64, width int) string {
	if width <= 0 {
		width = 16
	}
	value = boundedTUIFloat(value, 0, 100)
	filled := int(math.Round((value / 100) * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	fill := strings.Repeat("█", filled)
	empty := strings.Repeat("░", width-filled)
	style := goodBarStyle
	if value <= 15 {
		style = badBarStyle
	} else if value <= 35 {
		style = warnBarStyle
	}
	return style.Render(fill) + emptyBarStyle.Render(empty)
}

func percentText(value float64) string {
	return fmt.Sprintf("%5.1f%%", boundedTUIFloat(value, 0, 100))
}

func usageGaugeBlock(label string, used, remaining float64, resetLabel string, width int) string {
	label = safeDisplay(label)
	if label == "" {
		label = "window"
	}
	if width <= 0 {
		width = 22
	}
	status := riskLabel(used)
	head := windowStyle.Render(label) + " " +
		valueStyle.Render("used "+percentText(used)) + " " +
		mutedStyle.Render("left "+percentText(remaining)) + " " +
		statusBadge(status)
	lines := []string{
		head,
		percentBar(used, width) + " " + mutedStyle.Render("account usage"),
		remainingBar(remaining, width) + " " + mutedStyle.Render("account remaining"),
	}
	if resetLabel != "" {
		lines = append(lines, mutedStyle.Render(resetLabel))
	}
	return strings.Join(lines, "\n")
}

func poolGaugeBlock(label string, averageUsed, remainingPoints, capacityPoints float64, resetLabel string, width int) string {
	label = safeDisplay(label)
	if label == "" {
		label = "window"
	}
	if width <= 0 {
		width = 22
	}
	if capacityPoints < 0 {
		capacityPoints = 0
	}
	remainingPoints = boundedTUIFloat(remainingPoints, 0, capacityPoints)
	remainingPercent := 0.0
	if capacityPoints > 0 {
		remainingPercent = (remainingPoints / capacityPoints) * 100
	}
	head := windowStyle.Render(label) + " " +
		valueStyle.Render(fmt.Sprintf("remaining %.1f/%.1f account-points", remainingPoints, capacityPoints)) + " " +
		statusBadge(poolRiskLabel(averageUsed, remainingPercent))
	lines := []string{
		head,
		remainingBar(remainingPercent, width) + " " + mutedStyle.Render("pool remaining"),
		percentBar(averageUsed, width) + " " + mutedStyle.Render("avg used "+percentText(averageUsed)),
	}
	if resetLabel != "" {
		lines = append(lines, mutedStyle.Render(resetLabel))
	}
	return strings.Join(lines, "\n")
}

func riskLabel(used float64) string {
	used = boundedTUIFloat(used, 0, 100)
	switch {
	case used >= 85:
		return "error"
	case used >= 65:
		return "warning"
	default:
		return "fresh"
	}
}

func poolRiskLabel(averageUsed, remaining float64) string {
	averageUsed = boundedTUIFloat(averageUsed, 0, 100)
	remaining = boundedTUIFloat(remaining, 0, 100)
	switch {
	case averageUsed >= 85 || remaining <= 15:
		return "error"
	case averageUsed >= 65 || remaining <= 35:
		return "warning"
	default:
		return "fresh"
	}
}

func boundedTUIFloat(value, min, max float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func renderCard(width int, lines ...string) string {
	return renderAccentCard(width, lipgloss.Color("238"), lines...)
}

func renderSectionBanner(width int, title string, chips ...string) string {
	title = safeDisplay(title)
	if title == "" {
		title = "section"
	}
	parts := []string{heroStyle.Render(title)}
	for _, chip := range chips {
		chip = strings.TrimSpace(chip)
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
