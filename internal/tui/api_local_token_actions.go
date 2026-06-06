package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

type localTokenCreatedMsg struct {
	metadata management.LocalToken
	err      error
}

type localTokenDisabledMsg struct {
	id  int64
	err error
}

func (m *Model) clearReveal() {
	m.revealTokenID = 0
	m.revealTokenPrefix = ""
	m.revealTokenLast4 = ""
}

func (m *Model) selectNextLocalToken() {
	if m.selected < len(m.tokenRows)-1 {
		m.selected++
	}
}

func (m *Model) selectPreviousLocalToken() {
	if m.selected > 0 {
		m.selected--
	}
}

func (m Model) createLocalToken() (tea.Model, tea.Cmd) {
	if m.mutationInFlight {
		return m, nil
	}
	m.clearReveal()
	if m.tokens == nil {
		m.err = "local token management is unavailable"
		return m, nil
	}
	return m.startMutation(m.createLocalTokenCmd())
}

func (m Model) disableSelectedLocalToken() (tea.Model, tea.Cmd) {
	if m.mutationInFlight {
		return m, nil
	}
	m.clearReveal()
	if m.tokens == nil {
		m.err = "local token management is unavailable"
		return m, nil
	}
	if len(m.tokenRows) == 0 {
		return m, nil
	}
	id := m.tokenRows[m.selected].ID
	return m.startMutation(m.disableLocalTokenCmd(id))
}

func (m Model) createLocalTokenCmd() tea.Cmd {
	return func() tea.Msg {
		created, err := m.tokens.CreateLocalToken(context.Background(), management.CreateLocalTokenRequest{Label: "local client"})
		return localTokenCreatedMsg{metadata: created.Metadata, err: err}
	}
}

func (m Model) disableLocalTokenCmd(id int64) tea.Cmd {
	return func() tea.Msg {
		_, err := m.tokens.DisableLocalToken(context.Background(), management.DisableLocalTokenRequest{ID: id})
		return localTokenDisabledMsg{id: id, err: err}
	}
}
