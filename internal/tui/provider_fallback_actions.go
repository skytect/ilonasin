package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
)

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
		if instance.APIKey {
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

func fallbackPolicyEnabled(rows []management.FallbackPolicy, providerInstanceID, credentialKind, groupLabel string) bool {
	for _, row := range rows {
		if row.ProviderInstanceID == providerInstanceID && row.CredentialKind == credentialKind && row.GroupLabel == groupLabel {
			return row.Enabled
		}
	}
	return false
}
