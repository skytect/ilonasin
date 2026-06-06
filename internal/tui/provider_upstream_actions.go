package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

type upstreamCredentialDisabledMsg struct {
	credential management.UpstreamCredential
	err        error
}

func (m Model) disableUpstreamCredentialAction() (tea.Model, tea.Cmd) {
	if m.mutationInFlight {
		return m, nil
	}
	m.clearReveal()
	credential, ok := m.firstEnabledUpstreamCredential()
	if !ok {
		return m, nil
	}
	return m.startMutation(m.disableUpstreamCredentialCmd(credential))
}

func (m Model) firstEnabledUpstreamCredential() (management.UpstreamCredential, bool) {
	if m.upstreams == nil {
		return management.UpstreamCredential{}, false
	}
	for _, cred := range m.credentials {
		if !cred.Disabled {
			return cred, true
		}
	}
	return management.UpstreamCredential{}, false
}

func (m Model) disableUpstreamCredentialCmd(credential management.UpstreamCredential) tea.Cmd {
	return func() tea.Msg {
		_, err := m.upstreams.DisableUpstreamCredential(context.Background(), management.DisableUpstreamCredentialRequest{ID: credential.ID})
		return upstreamCredentialDisabledMsg{credential: credential, err: err}
	}
}
