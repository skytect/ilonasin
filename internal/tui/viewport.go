package tui

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m Model) renderViewport(body string) string {
	width := m.viewWidth()
	height := m.viewportHeight()
	lines := splitBodyLines(body)
	offset := m.scrollOffsets[m.validActiveTab()]
	maxOffset := maxInt(0, len(lines)-height)
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		index := offset + i
		line := ""
		if index < len(lines) {
			line = lines[index]
		}
		out = append(out, clipPlainLine(line, width))
	}
	return strings.Join(out, "\n")
}

func splitBodyLines(body string) []string {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return []string{""}
	}
	return strings.Split(body, "\n")
}

func (m Model) viewWidth() int {
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
		return tabOverview
	}
	return m.activeTab
}

func (m Model) activeScrollMax() int {
	return m.scrollMax(m.validActiveTab())
}

func (m Model) scrollMax(tab tuiTab) int {
	lines := splitBodyLines(m.tabBody(tab))
	return maxInt(0, len(lines)-m.viewportHeight())
}

func (m *Model) scrollActive(delta int) {
	m.setActiveScroll(m.scrollOffsets[m.validActiveTab()] + delta)
}

func (m *Model) setActiveScroll(offset int) {
	tab := m.validActiveTab()
	maxOffset := m.scrollMax(tab)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	m.scrollOffsets[tab] = offset
}

func (m *Model) clampScrolls() {
	if m.activeTab < 0 || m.activeTab >= tuiTabCount {
		m.activeTab = tabOverview
	}
	for _, tab := range tuiTabs {
		maxOffset := m.scrollMax(tab.id)
		if m.scrollOffsets[tab.id] > maxOffset {
			m.scrollOffsets[tab.id] = maxOffset
		}
		if m.scrollOffsets[tab.id] < 0 {
			m.scrollOffsets[tab.id] = 0
		}
	}
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
