package tui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) updateControlSectionKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch key.String() {
	case "n":
		if m.activeTab != tabAPI {
			return m, nil, true
		}
		m.paneFocus[tabAPI] = apiPaneTokens
		next, cmd := m.createLocalToken()
		return next, cmd, true
	case "d":
		if m.activeTab != tabAPI {
			return m, nil, true
		}
		m.paneFocus[tabAPI] = apiPaneTokens
		next, cmd := m.disableSelectedLocalToken()
		return next, cmd, true
	case "x":
		if m.activeTab != tabProviders {
			return m, nil, true
		}
		m.paneFocus[tabProviders] = providersPaneCredentials
		next, cmd := m.disableUpstreamCredentialAction()
		return next, cmd, true
	case "a":
		if m.activeTab != tabProviders {
			return m, nil, true
		}
		m.paneFocus[tabProviders] = providersPaneCredentials
		next, cmd := m.startAPIKeyInput()
		return next, cmd, true
	case "l":
		if m.activeTab != tabProviders {
			return m, nil, true
		}
		m.paneFocus[tabProviders] = providersPaneOAuth
		next, cmd := m.startOAuthLoginAction()
		return next, cmd, true
	case "r":
		if m.activeTab != tabProviders {
			return m, nil, true
		}
		m.paneFocus[tabProviders] = providersPaneOAuth
		next, cmd := m.refreshSelectedOAuthCredentialAction()
		return next, cmd, true
	case "o":
		if m.activeTab != tabProviders {
			return m, nil, true
		}
		m.paneFocus[tabProviders] = providersPaneOAuth
		next, cmd := m.cycleOAuthSelectionAction()
		return next, cmd, true
	}
	return m, nil, false
}
