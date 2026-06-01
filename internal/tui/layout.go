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
