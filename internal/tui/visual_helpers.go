package tui

import (
	"fmt"
	"math"
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
	innerWidth := width - 4
	if innerWidth < 8 {
		innerWidth = 8
	}
	if innerWidth > 92 {
		innerWidth = 92
	}
	bodyLines := make([]string, 0, len(lines))
	for _, line := range lines {
		bodyLines = append(bodyLines, ansi.Truncate(strings.TrimRight(line, "\n"), innerWidth, "..."))
	}
	body := strings.TrimRight(strings.Join(bodyLines, "\n"), "\n")
	return cardStyle.Render(body)
}
