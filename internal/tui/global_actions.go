package tui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) quitAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	m.cancelOAuthLogin()
	return m, tea.Quit
}

func (m Model) cancelVisibleAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	m.oauthChallenge = nil
	m.cancelOAuthLogin()
	return m, nil
}

func (m Model) clearRevealAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	return m, nil
}
