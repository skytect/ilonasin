package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func plainTableHeader(labels []string, columns []int) string {
	capacity := len(labels)
	if len(columns) < capacity {
		capacity = len(columns)
	}
	cells := make([]string, 0, capacity)
	for i := 0; i < len(labels) && i < len(columns); i++ {
		cells = append(cells, fitPlainCellFirstLine(labels[i], columns[i]))
	}
	return mutedStyle.Render(strings.Join(cells, " "))
}

func plainTableSeparator(width int, columns []int) string {
	if width <= 0 {
		return ""
	}
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		if column < 1 {
			column = 1
		}
		parts = append(parts, strings.Repeat("-", column))
	}
	return mutedStyle.Render(strings.Join(parts, " "))
}

func fitPlainCellFirstLine(value string, width int) string {
	value = strings.Join(strings.Fields(safeWrappedChromeDisplay(value)), " ")
	if width <= 0 {
		return value
	}
	chunks := wrapDisplayChunks(value, width)
	if len(chunks) > 0 {
		value = chunks[0]
	}
	valueWidth := ansi.StringWidth(value)
	if valueWidth < width {
		value += strings.Repeat(" ", width-valueWidth)
	}
	return value
}

func wrappedPlainTableRow(cells []string, columns []int) string {
	if len(cells) == 0 || len(columns) == 0 {
		return ""
	}
	cellLines := make([][]string, 0, len(cells))
	rowHeight := 1
	for i := 0; i < len(cells) && i < len(columns); i++ {
		lines := wrapPlainTableCell(cells[i], columns[i])
		if len(lines) > rowHeight {
			rowHeight = len(lines)
		}
		cellLines = append(cellLines, lines)
	}
	out := make([]string, 0, rowHeight)
	for lineIndex := 0; lineIndex < rowHeight; lineIndex++ {
		parts := make([]string, 0, len(cellLines))
		for columnIndex, lines := range cellLines {
			value := ""
			if lineIndex < len(lines) {
				value = lines[lineIndex]
			}
			parts = append(parts, padPlainCell(value, columns[columnIndex]))
		}
		out = append(out, strings.TrimRight(strings.Join(parts, " "), " "))
	}
	return strings.Join(out, "\n")
}

func wrapPlainTableCell(value string, width int) []string {
	value = strings.Join(strings.Fields(safeWrappedChromeDisplay(value)), " ")
	if value == "" {
		value = "none"
	}
	if width <= 0 {
		return []string{value}
	}
	chunks := wrapDisplayChunks(value, width)
	if len(chunks) == 0 {
		return []string{""}
	}
	return chunks
}

func padPlainCell(value string, width int) string {
	if width <= 0 {
		return value
	}
	valueWidth := ansi.StringWidth(value)
	if valueWidth < width {
		return value + strings.Repeat(" ", width-valueWidth)
	}
	return value
}
