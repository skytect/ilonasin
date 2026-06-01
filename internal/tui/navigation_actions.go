package tui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) nextTabAction() (tea.Model, tea.Cmd) {
	m.activeTab = (m.activeTab + 1) % tuiTabCount
	m.clampScrolls()
	return m, nil
}

func (m Model) previousTabAction() (tea.Model, tea.Cmd) {
	m.activeTab = (m.activeTab + tuiTabCount - 1) % tuiTabCount
	m.clampScrolls()
	return m, nil
}

func (m Model) selectTabAction(tab tuiTab) (tea.Model, tea.Cmd) {
	m.activeTab = tab
	m.clampScrolls()
	return m, nil
}

func (m Model) pageDownAction() (tea.Model, tea.Cmd) {
	m.scrollActive(m.viewportHeight())
	return m, nil
}

func (m Model) pageUpAction() (tea.Model, tea.Cmd) {
	m.scrollActive(-m.viewportHeight())
	return m, nil
}

func (m Model) homeAction() (tea.Model, tea.Cmd) {
	m.setActiveScroll(0)
	return m, nil
}

func (m Model) endAction() (tea.Model, tea.Cmd) {
	m.setActiveScroll(m.activeScrollMax())
	return m, nil
}

func (m Model) downAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if m.activeTab == tabAccounts {
		m.selectNextLocalToken()
	} else {
		m.scrollActive(1)
	}
	return m, nil
}

func (m Model) upAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if m.activeTab == tabAccounts {
		m.selectPreviousLocalToken()
	} else {
		m.scrollActive(-1)
	}
	return m, nil
}
