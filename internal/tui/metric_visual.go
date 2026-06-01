package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func metricCardWidth(width int) int {
	if width >= 110 {
		return (width - 2) / 2
	}
	return width
}

func renderMetricCardGrid(width int, cards []string) string {
	if len(cards) == 0 {
		return ""
	}
	if width < 110 || len(cards) == 1 {
		return strings.Join(cards, "\n")
	}
	gap := 2
	joined := make([]string, 0, (len(cards)+1)/2)
	for i := 0; i < len(cards); i += 2 {
		if i+1 >= len(cards) {
			joined = append(joined, cards[i])
			continue
		}
		line := lipgloss.JoinHorizontal(lipgloss.Top, cards[i], strings.Repeat(" ", gap), cards[i+1])
		if lipgloss.Width(line) > width {
			joined = append(joined, cards[i], cards[i+1])
			continue
		}
		joined = append(joined, line)
	}
	return strings.Join(joined, "\n")
}

func renderMetricAccentCard(width int, accent lipgloss.Color, lines ...string) string {
	style := cardStyle.MarginBottom(0).BorderForeground(accent)
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

func narrowMetrics(width int) bool {
	return width < 80
}

func metricBarWidth(width int) int {
	switch {
	case width < 60:
		return 10
	case width < 90:
		return 14
	default:
		return 18
	}
}

func msText(label string, value int64) string {
	return fmt.Sprintf("%s %s", label, compactDurationMS(value))
}

func tpsText(label string, value float64) string {
	return fmt.Sprintf("%s %.1f/s", label, boundedTUIFloat(value, 0, 9999))
}

func compactInt(value int) string {
	return compactInt64(int64(value))
}

func compactInt64(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	switch {
	case value >= 1_000_000_000:
		return fmt.Sprintf("%s%.1fb", sign, float64(value)/1_000_000_000)
	case value >= 1_000_000:
		return fmt.Sprintf("%s%.1fm", sign, float64(value)/1_000_000)
	case value >= 10_000:
		return fmt.Sprintf("%s%.1fk", sign, float64(value)/1_000)
	default:
		return fmt.Sprintf("%s%d", sign, value)
	}
}

func compactDurationMS(value int64) string {
	if value <= 0 {
		return "0ms"
	}
	if value >= 60_000 {
		return fmt.Sprintf("%.1fm", float64(value)/60_000)
	}
	if value >= 1_000 {
		return fmt.Sprintf("%.1fs", float64(value)/1_000)
	}
	return fmt.Sprintf("%dms", value)
}

func statusState(httpStatus int, errorClass string) string {
	switch {
	case errorClass != "":
		return "error"
	case httpStatus >= 500 || httpStatus == 429:
		return "error"
	case httpStatus >= 400:
		return "warning"
	default:
		return "fresh"
	}
}

func eventState(eventClass, errorClass string, httpStatus int) string {
	if errorClass != "" || httpStatus >= 400 {
		return "error"
	}
	switch safeDisplay(eventClass) {
	case "upstream_success", "success":
		return "fresh"
	default:
		return "warning"
	}
}

func latencyState(ms int64) string {
	switch {
	case ms >= 10_000:
		return "error"
	case ms >= 3_000:
		return "warning"
	default:
		return "fresh"
	}
}

func percentGaugeLine(label string, value float64, width int) string {
	return metricLine(
		mutedStyle.Render(safeMetricLabel(label)),
		percentBar(value, metricBarWidth(width)),
		valueStyle.Render(percentText(value)),
	)
}

func compactPercentMetric(label string, value float64) string {
	return metricChip(label, compactPercentText(value))
}

func endpointMetricChip(label, value string) string {
	label = safeMetricLabel(label)
	value = safeEndpointDisplay(value)
	if label == "" {
		label = "route"
	}
	if value == "" {
		value = "none"
	}
	return chipStyle.Render(label + " " + value)
}
