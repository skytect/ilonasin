package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

type upstreamAPIKeyAddedMsg struct {
	providerID string
	created    management.AddUpstreamAPIKeyResponse
	err        error
}

func (m Model) updateAPIKeyInput(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.clearAPIKeyInput()
		if key.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyEnter:
		apiKey := m.apiKeyInput
		providerID := m.apiKeyProvider
		if apiKey == "" {
			m.err = "API key is required"
			return m, nil
		}
		if m.upstreams == nil {
			m.err = "upstream credential management is unavailable"
			return m, nil
		}
		if m.mutationInFlight {
			return m, nil
		}
		m.clearAPIKeyInput()
		return m.startMutation(m.addUpstreamAPIKeyCmd(providerID, apiKey))
	case tea.KeyBackspace:
		if len(m.apiKeyInput) > 0 {
			m.apiKeyInput = m.apiKeyInput[:len(m.apiKeyInput)-1]
		}
		return m, nil
	case tea.KeyRunes:
		m.apiKeyInput += string(key.Runes)
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) startAPIKeyInput() (tea.Model, tea.Cmd) {
	m.clearReveal()
	instance, ok := firstAPIKeyProvider(m.providers)
	if !ok {
		m.err = "no API-key provider instance is configured"
		return m, nil
	}
	m.apiKeyMode = true
	m.apiKeyProvider = instance.ID
	m.apiKeyInput = ""
	return m, nil
}

func (m *Model) clearAPIKeyInput() {
	m.apiKeyMode = false
	m.apiKeyProvider = ""
	m.apiKeyInput = ""
}

func (m Model) addUpstreamAPIKeyCmd(providerID string, apiKey string) tea.Cmd {
	return func() tea.Msg {
		created, err := m.upstreams.AddUpstreamAPIKey(context.Background(), management.AddUpstreamAPIKeyRequest{
			ProviderInstanceID: providerID,
			Label:              "api key",
			APIKey:             apiKey,
		})
		return upstreamAPIKeyAddedMsg{providerID: providerID, created: created, err: err}
	}
}

func firstAPIKeyProvider(providers []management.ProviderInstance) (management.ProviderInstance, bool) {
	for _, instance := range providers {
		if instance.APIKey {
			return instance, true
		}
	}
	return management.ProviderInstance{}, false
}
