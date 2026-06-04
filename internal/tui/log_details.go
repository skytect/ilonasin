package tui

import (
	"strings"
)

type logDetailField struct {
	label string
	value string
}

func logDetailRows(fields []logDetailField, width int) string {
	if len(fields) == 0 {
		return ""
	}
	labelWidth := logDetailLabelWidth(fields)
	valueWidth := width - labelWidth - 3
	if valueWidth < 8 {
		valueWidth = maxInt(1, width)
		labelWidth = 0
	}
	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		label := safeMetricLabel(field.label)
		value := strings.Join(strings.Fields(safeWrappedChromeDisplay(field.value)), " ")
		if value == "" {
			value = "none"
		}
		valueLines := wrapPlainTableCell(value, valueWidth)
		if labelWidth == 0 {
			for _, valueLine := range valueLines {
				lines = append(lines, mutedStyle.Render(label)+" "+valueLine)
			}
			continue
		}
		for index, valueLine := range valueLines {
			labelCell := ""
			if index == 0 {
				labelCell = padPlainCell(label, labelWidth)
			} else {
				labelCell = strings.Repeat(" ", labelWidth)
			}
			lines = append(lines, mutedStyle.Render(labelCell)+mutedStyle.Render(" | ")+valueLine)
		}
	}
	return strings.Join(lines, "\n")
}

func logDetailLabelWidth(fields []logDetailField) int {
	width := 0
	for _, field := range fields {
		label := safeMetricLabel(field.label)
		if len(label) > width {
			width = len(label)
		}
	}
	if width < 5 {
		return 5
	}
	return width
}
