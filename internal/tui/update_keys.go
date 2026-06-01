package tui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.apiKeyMode {
		return m.updateAPIKeyInput(key)
	}
	switch key.String() {
	case "tab", "right":
		return m.nextTabAction()
	case "shift+tab", "left":
		return m.previousTabAction()
	case "1":
		return m.selectTabAction(tabAPI)
	case "2":
		return m.selectTabAction(tabProviders)
	case "3":
		return m.selectTabAction(tabUsage)
	case "4":
		return m.selectTabAction(tabLogs)
	case "pgdown", "ctrl+d":
		return m.pageDownAction()
	case "pgup", "ctrl+u":
		return m.pageUpAction()
	case "home":
		return m.homeAction()
	case "end":
		return m.endAction()
	case "q", "ctrl+c":
		return m.quitAction()
	case "esc":
		return m.cancelVisibleAction()
	case "enter":
		return m.clearRevealAction()
	case "down", "j":
		return m.downAction()
	case "up", "k":
		return m.upAction()
	}
	if next, cmd, handled := m.updateAccountKey(key); handled {
		return next, cmd
	}
	if next, cmd, handled := m.updateObservabilityKey(key); handled {
		return next, cmd
	}
	return m, nil
}
