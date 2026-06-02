package tui

import (
	"fmt"
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
	b.WriteString(m.renderDashboard())
	b.WriteByte('\n')
	b.WriteString(clipPlainLine(m.footerLine(width), width))
	b.WriteByte('\n')
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
		return "New token " + strconv.FormatInt(m.revealTokenID, 10) + " metadata is visible on api."
	}
	if m.apiKeyMode {
		return "Adding API key for " + safeDisplay(m.apiKeyProvider) + ": " + strings.Repeat("*", len(m.apiKeyInput))
	}
	if m.oauthChallenge != nil {
		return "OAuth login for " + safeDisplay(m.oauthChallenge.ProviderInstanceID) + " is visible on providers."
	}
	return ""
}

func (m Model) footerLine(width int) string {
	hints := []keyHint{
		{"tab", "section"},
		{"1-4", "jump"},
		{"[/]", "pane"},
		{"j/k", "move"},
		{"pg", "page"},
	}
	switch m.activeTab {
	case tabAPI:
		hints = append(hints,
			keyHint{"n", "new token"},
			keyHint{"d", "disable"},
		)
	case tabProviders:
		hints = append(hints,
			keyHint{"a", "add key"},
			keyHint{"x", "disable"},
			keyHint{"l", "login"},
			keyHint{"o/r", "oauth"},
			keyHint{"f/F", "fallback"},
		)
	case tabUsage:
		hints = append(hints, keyHint{"u", "refresh"})
	case tabLogs:
		hints = append(hints, keyHint{"p", "prune"})
	}
	hints = append(hints,
		keyHint{"home/end", "jump"},
		keyHint{"q", "quit"},
	)
	return renderKeyMap(width, hints)
}
