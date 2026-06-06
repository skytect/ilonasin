package tui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) startMutation(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if cmd == nil || m.mutationInFlight {
		return m, nil
	}
	m.mutationInFlight = true
	return m, cmd
}

func (m Model) completeMutation() Model {
	m.mutationInFlight = false
	return m
}
