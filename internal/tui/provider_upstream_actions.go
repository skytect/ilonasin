package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

func (m Model) disableUpstreamCredentialAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if err := m.disableFirstUpstreamCredential(); err != nil {
		m.logError(context.Background(), "tui_upstream_credential_disable_failed", err)
		m.err = err.Error()
		return m, nil
	}
	_ = m.reload()
	return m, nil
}

func (m *Model) disableFirstUpstreamCredential() error {
	if m.upstreams == nil {
		return nil
	}
	for _, cred := range m.credentials {
		if !cred.Disabled {
			if _, err := m.upstreams.DisableUpstreamCredential(context.Background(), management.DisableUpstreamCredentialRequest{ID: cred.ID}); err != nil {
				return err
			}
			m.logInfo(context.Background(), "tui_upstream_credential_disabled",
				slog.String("provider_instance", cred.ProviderInstanceID),
				slog.Int64("credential_id", cred.ID),
			)
			return nil
		}
	}
	return nil
}
