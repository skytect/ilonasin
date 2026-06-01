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
	case "n":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.createLocalToken()
	case "d":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.disableSelectedLocalToken()
	case "x":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.disableUpstreamCredentialAction()
	case "a":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.startAPIKeyInput()
	case "p":
		if m.activeTab != tabObservability {
			return m, nil
		}
		return m.pruneTelemetryAction()
	case "u":
		if m.activeTab != tabObservability {
			return m, nil
		}
		return m.refreshSubscriptionUsageAction()
	case "l":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.startOAuthLoginAction()
	case "r":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.refreshSelectedOAuthCredentialAction()
	case "o":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.cycleOAuthSelectionAction()
	case "f":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.enableFallbackPolicyAction()
	case "F":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.disableFallbackPolicyAction()
	case "esc":
		return m.cancelVisibleAction()
	case "enter":
		return m.clearRevealAction()
	case "down", "j":
		return m.downAction()
	case "up", "k":
		return m.upAction()
	}
	return m, nil
}
