package tui

import tea "github.com/charmbracelet/bubbletea"

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
