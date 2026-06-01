package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

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
		m.clearAPIKeyInput()
		if apiKey == "" {
			m.err = "API key is required"
			return m, nil
		}
		if m.upstreams == nil {
			m.err = "upstream credential management is unavailable"
			return m, nil
		}
		created, err := m.upstreams.AddUpstreamAPIKey(context.Background(), management.AddUpstreamAPIKeyRequest{
			ProviderInstanceID: providerID,
			Label:              "api key",
			APIKey:             apiKey,
		})
		if err != nil {
			m.logError(context.Background(), "tui_upstream_credential_create_failed", err, slog.String("provider_instance", providerID))
			m.err = err.Error()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_upstream_credential_created",
			slog.String("provider_instance", providerID),
			slog.Int64("credential_id", created.Credential.ID),
		)
		_ = m.reload()
		return m, nil
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
	instance, ok := firstAPIKeyProvider(m.registry)
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

func firstAPIKeyProvider(registry provider.Registry) (provider.Instance, bool) {
	for _, instance := range registry.List() {
		if instance.APIKey && !instance.Placeholder {
			return instance, true
		}
	}
	return provider.Instance{}, false
}
