package tui

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func splitBodyLines(body string) []string {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return []string{""}
	}
	return strings.Split(body, "\n")
}

func (m Model) viewWidth() int {
	if m.renderWidth > 0 {
		return m.renderWidth
	}
	if m.width > 0 {
		return m.width
	}
	if width, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && width > 0 {
		return width
	}
	return 100
}

func (m Model) viewHeight() int {
	if m.height > 0 {
		return m.height
	}
	if height, err := strconv.Atoi(os.Getenv("LINES")); err == nil && height > 0 {
		return height
	}
	return 30
}

func (m Model) viewportHeight() int {
	reserved := 3
	if m.statusLine() != "" {
		reserved++
	}
	height := m.viewHeight() - reserved
	if height < 1 {
		return 1
	}
	return height
}

func (m Model) validActiveTab() tuiTab {
	if m.activeTab < 0 || m.activeTab >= tuiTabCount {
		return tabAPI
	}
	return m.activeTab
}

func (m *Model) clampScrolls() {
	if m.activeTab < 0 || m.activeTab >= tuiTabCount {
		m.activeTab = tabAPI
	}
	m.clampPaneState()
}

func clipPlainLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	if ansi.StringWidth(line) <= width {
		return line
	}
	if width <= 3 {
		return ansi.Truncate(line, width, "")
	}
	return ansi.Truncate(line, width, "...")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
