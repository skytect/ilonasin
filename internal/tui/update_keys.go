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
		return m.selectTabAction(tabOverview)
	case "2":
		return m.selectTabAction(tabAccounts)
	case "3":
		return m.selectTabAction(tabObservability)
	case "4":
		return m.selectTabAction(tabHelp)
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

func (m Model) updateAccountKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch key.String() {
	case "n":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.createLocalToken()
		return next, cmd, true
	case "d":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.disableSelectedLocalToken()
		return next, cmd, true
	case "x":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.disableUpstreamCredentialAction()
		return next, cmd, true
	case "a":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.startAPIKeyInput()
		return next, cmd, true
	case "l":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.startOAuthLoginAction()
		return next, cmd, true
	case "r":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.refreshSelectedOAuthCredentialAction()
		return next, cmd, true
	case "o":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.cycleOAuthSelectionAction()
		return next, cmd, true
	case "f":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.enableFallbackPolicyAction()
		return next, cmd, true
	case "F":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.disableFallbackPolicyAction()
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) updateObservabilityKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch key.String() {
	case "p":
		if m.activeTab != tabObservability {
			return m, nil, true
		}
		next, cmd := m.pruneTelemetryAction()
		return next, cmd, true
	case "u":
		if m.activeTab != tabObservability {
			return m, nil, true
		}
		next, cmd := m.refreshSubscriptionUsageAction()
		return next, cmd, true
	}
	return m, nil, false
}
