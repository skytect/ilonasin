package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

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

func (m Model) createLocalToken() (tea.Model, tea.Cmd) {
	m.clearReveal()
	created, err := m.tokens.CreateLocalToken(context.Background(), management.CreateLocalTokenRequest{Label: "local client"})
	if err != nil {
		m.logError(context.Background(), "tui_local_token_create_failed", err)
		m.err = err.Error()
		return m, nil
	}
	m.logInfo(context.Background(), "tui_local_token_created", slog.Int64("local_id", created.Metadata.ID))
	m.revealTokenID = created.Metadata.ID
	m.revealTokenPrefix = created.Metadata.TokenPrefix
	m.revealTokenLast4 = created.Metadata.TokenLast4
	_ = m.reload()
	return m, nil
}

func (m Model) disableSelectedLocalToken() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if len(m.tokenRows) == 0 {
		return m, nil
	}
	if _, err := m.tokens.DisableLocalToken(context.Background(), management.DisableLocalTokenRequest{ID: m.tokenRows[m.selected].ID}); err != nil {
		m.logError(context.Background(), "tui_local_token_disable_failed", err, slog.Int64("local_id", m.tokenRows[m.selected].ID))
		m.err = err.Error()
		return m, nil
	}
	m.logInfo(context.Background(), "tui_local_token_disabled", slog.Int64("local_id", m.tokenRows[m.selected].ID))
	_ = m.reload()
	return m, nil
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

func (m Model) enableFallbackPolicyAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if err := m.enableFirstFallbackPolicy(); err != nil {
		m.logError(context.Background(), "tui_fallback_policy_update_failed", err)
		m.err = "fallback policy update failed"
		return m, nil
	}
	_ = m.reload()
	return m, nil
}

func (m Model) disableFallbackPolicyAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if err := m.disableFirstFallbackPolicy(); err != nil {
		m.logError(context.Background(), "tui_fallback_policy_update_failed", err)
		m.err = "fallback policy update failed"
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

func (m *Model) enableFirstFallbackPolicy() error {
	if m.upstreams == nil {
		return nil
	}
	for _, row := range m.fallbackPolicies {
		if !row.Enabled {
			if _, err := m.upstreams.EnableFallbackPolicy(context.Background(), management.FallbackPolicyRequest{
				ProviderInstanceID: row.ProviderInstanceID,
				CredentialKind:     row.CredentialKind,
				GroupLabel:         row.GroupLabel,
			}); err != nil {
				return err
			}
			m.logInfo(context.Background(), "tui_fallback_policy_changed",
				slog.String("provider_instance", row.ProviderInstanceID),
				slog.String("credential_kind", row.CredentialKind),
				slog.String("group", row.GroupLabel),
				slog.Bool("enabled", true),
			)
			return nil
		}
	}
	return nil
}

func (m *Model) disableFirstFallbackPolicy() error {
	if m.upstreams == nil {
		return nil
	}
	for _, row := range m.fallbackPolicies {
		if row.Enabled {
			if _, err := m.upstreams.DisableFallbackPolicy(context.Background(), management.FallbackPolicyRequest{
				ProviderInstanceID: row.ProviderInstanceID,
				CredentialKind:     row.CredentialKind,
				GroupLabel:         row.GroupLabel,
			}); err != nil {
				return err
			}
			m.logInfo(context.Background(), "tui_fallback_policy_changed",
				slog.String("provider_instance", row.ProviderInstanceID),
				slog.String("credential_kind", row.CredentialKind),
				slog.String("group", row.GroupLabel),
				slog.Bool("enabled", false),
			)
			return nil
		}
	}
	return nil
}

func (m Model) visibleFallbackPolicies(rows []management.FallbackPolicy) []management.FallbackPolicy {
	allowed := map[string]map[string]bool{}
	for _, instance := range m.visibleProviderRows() {
		if instance.APIKey && !instance.Placeholder {
			allowed[instance.ID] = map[string]bool{credentials.CredentialKindAPIKey: true}
		}
		if instance.OAuth && instance.Type == "codex" {
			if allowed[instance.ID] == nil {
				allowed[instance.ID] = map[string]bool{}
			}
			allowed[instance.ID][credentials.CredentialKindOAuth] = true
		}
	}
	out := make([]management.FallbackPolicy, 0, len(rows))
	for _, row := range rows {
		if allowed[row.ProviderInstanceID][row.CredentialKind] && row.CredentialCount >= 2 {
			out = append(out, row)
		}
	}
	return out
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

func fallbackPolicyEnabled(rows []management.FallbackPolicy, providerInstanceID, credentialKind, groupLabel string) bool {
	for _, row := range rows {
		if row.ProviderInstanceID == providerInstanceID && row.CredentialKind == credentialKind && row.GroupLabel == groupLabel {
			return row.Enabled
		}
	}
	return false
}
