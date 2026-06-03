package tui

import (
	"fmt"
	"math"
	"strings"
)

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

func meterRow(label, bar, value string, width int, trailing ...string) string {
	label = safeMetricLabel(label)
	value = safeChromeDisplay(value)
	parts := make([]string, 0, 3+len(trailing))
	if label != "" {
		parts = append(parts, mutedStyle.Render(label))
	}
	if bar != "" {
		parts = append(parts, bar)
	}
	if value != "" {
		parts = append(parts, valueStyle.Render(value))
	}
	for _, item := range trailing {
		if strings.TrimSpace(item) != "" {
			parts = append(parts, item)
		}
	}
	return wrapTargetedLines(width, metricLine(parts...))
}

func percentText(value float64) string {
	return fmt.Sprintf("%5.1f%%", boundedTUIFloat(value, 0, 100))
}

func usageGaugeBlock(label string, used, remaining float64, resetLabel string, barWidth, lineWidth int) string {
	label = safeFullWrappedDisplay(label)
	if label == "" {
		label = "window"
	}
	if barWidth <= 0 {
		barWidth = 22
	}
	if barWidth < 16 {
		line := windowStyle.Render(label) + " " +
			balancedUsageBar(used, remaining, barWidth) + " " +
			valueStyle.Render(compactPercentText(used)+"/"+compactPercentText(remaining))
		if resetLabel != "" {
			line += "  " + mutedStyle.Render(compactResetTimeOnly(resetLabel))
		}
		return wrapTargetedLines(lineWidth, line)
	}
	if barWidth < 30 {
		line := windowStyle.Render(label) + " " +
			balancedUsageBar(used, remaining, barWidth) + " " +
			valueStyle.Render(compactPercentText(used)+" used") + " " +
			mutedStyle.Render(compactPercentText(remaining)+" left")
		if resetLabel != "" {
			line += "  " + mutedStyle.Render(compactResetTimeOnly(resetLabel))
		}
		return wrapTargetedLines(lineWidth, line)
	}
	line := windowStyle.Render(label) + " " +
		balancedUsageBar(used, remaining, barWidth) + " " +
		valueStyle.Render(percentText(used)+" used") + " " +
		mutedStyle.Render(percentText(remaining)+" left")
	if resetLabel != "" {
		line += "  " + mutedStyle.Render(resetLabel)
	}
	return wrapTargetedLines(lineWidth, line)
}

func poolGaugeBlock(label string, usedPoints, remainingPoints, capacityPoints float64, accountCount, staleCount int, resetLabel string, barWidth, lineWidth int) string {
	label = safeFullWrappedDisplay(label)
	if label == "" {
		label = "window"
	}
	if barWidth <= 0 {
		barWidth = 22
	}
	if capacityPoints < 0 {
		capacityPoints = 0
	}
	usedPoints = boundedTUIFloat(usedPoints, 0, capacityPoints)
	remainingPoints = boundedTUIFloat(remainingPoints, 0, capacityPoints)
	usedPercent := 0.0
	remainingPercent := 0.0
	if capacityPoints > 0 {
		usedPercent = (usedPoints / capacityPoints) * 100
		remainingPercent = (remainingPoints / capacityPoints) * 100
	}
	if barWidth < 16 {
		line := windowStyle.Render(label) + " " +
			balancedUsageBar(usedPercent, remainingPercent, barWidth) + " " +
			valueStyle.Render(fmt.Sprintf("sum used %.0fpp", usedPoints)) + " " +
			mutedStyle.Render(fmt.Sprintf("sum left %.0fpp", remainingPoints)) + " " +
			mutedStyle.Render(fmt.Sprintf("capacity %.0fpp", capacityPoints)) + " " +
			mutedStyle.Render(fmt.Sprintf("acct %d stale %d", accountCount, staleCount))
		if resetLabel != "" {
			line += "  " + mutedStyle.Render(compactResetText(resetLabel))
		}
		return wrapTargetedLines(lineWidth, line)
	}
	if barWidth < 30 {
		line := windowStyle.Render(label) + " " +
			balancedUsageBar(usedPercent, remainingPercent, barWidth) + " " +
			valueStyle.Render(fmt.Sprintf("sum used %.0fpp", usedPoints)) + " " +
			mutedStyle.Render(fmt.Sprintf("sum left %.0fpp", remainingPoints)) + " " +
			mutedStyle.Render(fmt.Sprintf("capacity %.0fpp", capacityPoints)) + " " +
			mutedStyle.Render(fmt.Sprintf("acct %d stale %d", accountCount, staleCount)) + " " +
			statusBadge(remainingRiskLabel(remainingPercent))
		if resetLabel != "" {
			line += "  " + mutedStyle.Render(compactResetTimeOnly(resetLabel))
		}
		return wrapTargetedLines(lineWidth, line)
	}
	line := windowStyle.Render(label) + " " +
		balancedUsageBar(usedPercent, remainingPercent, barWidth) + " " +
		valueStyle.Render(fmt.Sprintf("sum used %.1fpp", usedPoints)) + " " +
		mutedStyle.Render(fmt.Sprintf("sum left %.1fpp", remainingPoints)) + " " +
		mutedStyle.Render(fmt.Sprintf("capacity %.1fpp", capacityPoints)) + " " +
		mutedStyle.Render(fmt.Sprintf("accounts %d stale %d", accountCount, staleCount)) + " " +
		statusBadge(remainingRiskLabel(remainingPercent))
	if resetLabel != "" {
		line += "  " + mutedStyle.Render(resetLabel)
	}
	return wrapTargetedLines(lineWidth, line)
}

func balancedUsageBar(used, remaining float64, width int) string {
	if width <= 0 {
		width = 16
	}
	used = boundedTUIFloat(used, 0, 100)
	remaining = boundedTUIFloat(remaining, 0, 100)
	total := used + remaining
	usedShare := 0.0
	if total > 0 {
		usedShare = used / total
	}
	usedCells := int(math.Round(usedShare * float64(width)))
	if usedCells < 0 {
		usedCells = 0
	}
	if usedCells > width {
		usedCells = width
	}
	remainingCells := width - usedCells
	usedStyle := goodBarStyle
	if used >= 85 {
		usedStyle = badBarStyle
	} else if used >= 65 {
		usedStyle = warnBarStyle
	}
	remainingStyle := goodBarStyle
	if remaining <= 15 {
		remainingStyle = badBarStyle
	} else if remaining <= 35 {
		remainingStyle = warnBarStyle
	}
	return usedStyle.Render(strings.Repeat("█", usedCells)) +
		remainingStyle.Render(strings.Repeat("░", remainingCells))
}

func compactPercentText(value float64) string {
	return fmt.Sprintf("%.0f%%", boundedTUIFloat(value, 0, 100))
}

func compactResetText(value string) string {
	parts := strings.Fields(value)
	for i, part := range parts {
		if part == "in" && i+1 < len(parts) {
			return strings.Join(parts[:i+2], " ")
		}
		if part == "ago" && i > 0 {
			start := i - 1
			if start > 0 {
				start--
			}
			return strings.Join(parts[start:i+1], " ")
		}
		if part == "now" {
			if i > 0 {
				return strings.Join(parts[i-1:i+1], " ")
			}
			return part
		}
	}
	if len(parts) >= 2 {
		return parts[0] + " " + parts[len(parts)-1]
	}
	return value
}

func compactResetTimeOnly(value string) string {
	value = compactResetText(value)
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) >= 2 && parts[len(parts)-1] == "ago" {
		return strings.Join(parts[len(parts)-2:], " ")
	}
	if len(parts) >= 2 && parts[len(parts)-2] == "in" {
		return strings.Join(parts[len(parts)-2:], " ")
	}
	return parts[len(parts)-1]
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

func remainingRiskLabel(remaining float64) string {
	remaining = boundedTUIFloat(remaining, 0, 100)
	switch {
	case remaining <= 15:
		return "error"
	case remaining <= 35:
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
