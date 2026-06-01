package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	var b strings.Builder
	width := m.viewWidth()
	header := fmt.Sprintf("ilonasin  providers %d  bind %s", len(m.cfg.Providers), m.cfg.Server.Bind)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render(clipPlainLine(header, width))
	b.WriteString(title)
	b.WriteByte('\n')
	b.WriteString(m.tabBar(width))
	b.WriteByte('\n')
	status := m.statusLine()
	if status != "" {
		b.WriteString(clipPlainLine(status, width))
		b.WriteByte('\n')
	}
	b.WriteString(m.renderViewport(m.activeTabBody()))
	b.WriteByte('\n')
	b.WriteString(clipPlainLine(m.footerLine(), width))
	b.WriteByte('\n')
	return b.String()
}

func (m Model) activeTabBody() string {
	return m.tabBody(m.activeTab)
}

func (m Model) tabBody(tab tuiTab) string {
	var b strings.Builder
	switch tab {
	case tabOverview:
		m.writeOverview(&b)
	case tabAccounts:
		m.writeAccounts(&b)
	case tabObservability:
		m.writeObservability(&b)
	case tabHelp:
		m.writeHelp(&b)
	default:
		m.writeOverview(&b)
	}
	return b.String()
}

func (m Model) tabBar(width int) string {
	active := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	parts := make([]string, 0, len(tuiTabs))
	for _, tab := range tuiTabs {
		label := " " + tab.label + " "
		if tab.id == m.activeTab {
			parts = append(parts, active.Render("["+label+"]"))
		} else {
			parts = append(parts, inactive.Render(" "+label+" "))
		}
	}
	line := strings.Join(parts, " ")
	if width > 0 && lipgloss.Width(line) > width {
		return lipgloss.NewStyle().MaxWidth(width).Render(line)
	}
	return line
}

func (m Model) statusLine() string {
	if m.err != "" {
		return "Error: " + safeErrorMessage(m.err)
	}
	if m.revealTokenID != 0 {
		return "New token " + strconv.FormatInt(m.revealTokenID, 10) + " metadata is visible on accounts."
	}
	if m.apiKeyMode {
		return "Adding API key for " + safeDisplay(m.apiKeyProvider) + ": " + strings.Repeat("*", len(m.apiKeyInput))
	}
	if m.oauthChallenge != nil {
		return "OAuth login for " + safeDisplay(m.oauthChallenge.ProviderInstanceID) + " is visible on accounts."
	}
	return ""
}

func (m Model) footerLine() string {
	switch m.activeTab {
	case tabAccounts:
		return "tab switch  up/down select  pgup/pgdown scroll  n new token  a add key  d disable token  x disable key  l login  o/r OAuth  f/F fallback  q quit"
	case tabObservability:
		return "tab switch  up/down scroll  pgup/pgdown page  home/end jump  u usage  p prune  q quit"
	case tabHelp:
		return "tab switch  up/down scroll  q quit"
	default:
		return "tab switch  up/down scroll  q quit"
	}
}

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
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
