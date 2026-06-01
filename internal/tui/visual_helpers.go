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

func accountIdentityField(label, fallback string) string {
	identity := accountIdentity(label, fallback)
	field := "identity"
	if looksLikeEmail(identity) {
		field = "email"
	}
	return labelStyle.Render(field) + " " + valueStyle.Render(identity)
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
	label = safeDisplay(label)
	value = safeDisplay(value)
	if label == "" {
		label = "metric"
	}
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
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

func percentText(value float64) string {
	return fmt.Sprintf("%5.1f%%", boundedTUIFloat(value, 0, 100))
}

func usageGauge(label string, used, remaining float64, resetLabel string, width int) string {
	label = safeDisplay(label)
	if label == "" {
		label = "window"
	}
	if width <= 0 {
		width = 18
	}
	remaining = boundedTUIFloat(remaining, 0, 100)
	head := labelStyle.Render(label) + " " + valueStyle.Render("used "+percentText(used)) + " " + mutedStyle.Render("left "+percentText(remaining))
	bar := percentBar(used, width)
	if resetLabel != "" {
		return head + "\n" + bar + " " + mutedStyle.Render(resetLabel)
	}
	return head + "\n" + bar
}

func poolGauge(label string, averageUsed, minimumRemaining float64, resetLabel string, width int) string {
	label = safeDisplay(label)
	if label == "" {
		label = "window"
	}
	if width <= 0 {
		width = 18
	}
	head := labelStyle.Render(label) + " " + valueStyle.Render("avg "+percentText(averageUsed)) + " " + mutedStyle.Render("min left "+percentText(minimumRemaining))
	bar := percentBar(averageUsed, width)
	if resetLabel != "" {
		return head + "\n" + bar + " " + mutedStyle.Render(resetLabel)
	}
	return head + "\n" + bar
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

func renderAccentCard(width int, accent lipgloss.Color, lines ...string) string {
	style := cardStyle.BorderForeground(accent)
	innerWidth := width - style.GetHorizontalFrameSize()
	if innerWidth < 8 {
		innerWidth = 8
	}
	if innerWidth > 92 {
		innerWidth = 92
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
