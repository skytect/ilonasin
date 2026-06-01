package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

func (m Model) updateAccountKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch key.String() {
	case "n":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.createLocalToken()
		return next, cmd, true
	case "d":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.disableSelectedLocalToken()
		return next, cmd, true
	case "x":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.disableUpstreamCredentialAction()
		return next, cmd, true
	case "a":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.startAPIKeyInput()
		return next, cmd, true
	case "l":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.startOAuthLoginAction()
		return next, cmd, true
	case "r":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.refreshSelectedOAuthCredentialAction()
		return next, cmd, true
	case "o":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.cycleOAuthSelectionAction()
		return next, cmd, true
	case "f":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.enableFallbackPolicyAction()
		return next, cmd, true
	case "F":
		if m.activeTab != tabAccounts {
			return m, nil, true
		}
		next, cmd := m.disableFallbackPolicyAction()
		return next, cmd, true
	}
	return m, nil, false
}

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

func (m Model) visibleUpstreamCredentials(rows []management.UpstreamCredential) []management.UpstreamCredential {
	allowed := map[string]bool{}
	for _, instance := range m.visibleProviderRows() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = true
		}
	}
	out := make([]management.UpstreamCredential, 0, len(rows))
	for _, row := range rows {
		if allowed[row.ProviderInstanceID] {
			out = append(out, row)
		}
	}
	return out
}

func (m Model) visibleProviderRows() []provider.Instance {
	if len(m.providers) > 0 {
		return m.providers
	}
	return m.registry.List()
}
