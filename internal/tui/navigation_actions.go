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
	m.scrollFocusedPane(m.paneContentHeight(m.validActiveTab(), m.validPaneFocus(m.validActiveTab())))
	return m, nil
}

func (m Model) pageUpAction() (tea.Model, tea.Cmd) {
	m.scrollFocusedPane(-m.paneContentHeight(m.validActiveTab(), m.validPaneFocus(m.validActiveTab())))
	return m, nil
}

func (m Model) homeAction() (tea.Model, tea.Cmd) {
	m.setFocusedPaneScroll(0)
	return m, nil
}

func (m Model) endAction() (tea.Model, tea.Cmd) {
	m.setFocusedPaneScroll(m.focusedPaneScrollMax())
	return m, nil
}

func (m Model) downAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if m.activeTab == tabAPI && m.validPaneFocus(tabAPI) == apiPaneTokens {
		m.selectNextLocalToken()
	} else {
		m.scrollFocusedPane(1)
	}
	return m, nil
}

func (m Model) upAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if m.activeTab == tabAPI && m.validPaneFocus(tabAPI) == apiPaneTokens {
		m.selectPreviousLocalToken()
	} else {
		m.scrollFocusedPane(-1)
	}
	return m, nil
}

func (m Model) nextPaneAction() (tea.Model, tea.Cmd) {
	m.focusNextPane()
	return m, nil
}

func (m Model) previousPaneAction() (tea.Model, tea.Cmd) {
	m.focusPreviousPane()
	return m, nil
}
