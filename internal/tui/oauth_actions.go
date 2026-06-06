package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

type oauthLoginStartedMsg struct {
	challenge management.OAuthDeviceLoginChallenge
	err       error
}

type oauthLoginCompletedMsg struct {
	err error
}

type oauthCredentialRefreshedMsg struct {
	row management.OAuthCredential
	err error
}

func (m Model) startOAuthLoginCmd(ctx context.Context, providerInstanceID string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.oauth.StartOAuthDeviceLogin(ctx, management.StartOAuthDeviceLoginRequest{ProviderInstanceID: providerInstanceID})
		return oauthLoginStartedMsg{challenge: resp.Challenge, err: err}
	}
}

func (m Model) completeOAuthLoginCmd(handle string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.oauthCtx
		if ctx == nil {
			ctx = context.Background()
		}
		_, err := m.oauth.CompleteOAuthDeviceLogin(ctx, management.CompleteOAuthDeviceLoginRequest{Handle: handle})
		return oauthLoginCompletedMsg{err: err}
	}
}

func (m *Model) cancelOAuthLogin() {
	if m.oauthCancel != nil {
		m.oauthCancel()
		m.oauthCancel = nil
	}
	m.oauthCtx = nil
}

func (m Model) startOAuthLoginAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	m.cancelOAuthLogin()
	providerID, ok := firstOAuthLoginProvider(m.providers)
	if !ok || m.oauth == nil {
		m.logInfo(context.Background(), "tui_oauth_login_unavailable")
		m.err = "OAuth login failed"
		return m, nil
	}
	loginCtx, cancel := context.WithCancel(context.Background())
	m.oauthCtx = loginCtx
	m.oauthCancel = cancel
	return m, m.startOAuthLoginCmd(loginCtx, providerID)
}

func (m Model) refreshSelectedOAuthCredentialAction() (tea.Model, tea.Cmd) {
	if m.mutationInFlight {
		return m, nil
	}
	m.clearReveal()
	row, ok := m.selectedOAuthCredential()
	if !ok {
		return m, nil
	}
	return m.startMutation(m.refreshOAuthCredentialCmd(row))
}

func (m Model) cycleOAuthSelectionAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if len(m.oauthRows) > 0 {
		m.oauthSelected = (m.oauthSelected + 1) % len(m.oauthRows)
	}
	return m, nil
}

func (m Model) selectedOAuthCredential() (management.OAuthCredential, bool) {
	if m.oauth == nil || len(m.oauthRows) == 0 {
		return management.OAuthCredential{}, false
	}
	if m.oauthSelected < 0 || m.oauthSelected >= len(m.oauthRows) {
		return management.OAuthCredential{}, false
	}
	row := m.oauthRows[m.oauthSelected]
	if row.Disabled {
		return management.OAuthCredential{}, false
	}
	return row, true
}

func (m Model) refreshOAuthCredentialCmd(row management.OAuthCredential) tea.Cmd {
	return func() tea.Msg {
		_, err := m.oauth.RefreshOAuthCredential(context.Background(), management.RefreshOAuthCredentialRequest{ID: row.ID})
		return oauthCredentialRefreshedMsg{row: row, err: err}
	}
}

func firstOAuthLoginProvider(providers []management.ProviderInstance) (string, bool) {
	for _, instance := range providers {
		if management.SupportsCodexOAuth(instance) {
			return instance.ID, true
		}
	}
	return "", false
}

func oauthLoginErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var loginErr management.ClientError
	if errors.As(err, &loginErr) && loginErr.Class != "" {
		if loginErr.EventID != "" {
			return "OAuth login failed: " + loginErr.Class + " event_id=" + loginErr.EventID
		}
		return "OAuth login failed: " + loginErr.Class
	}
	if errors.Is(err, context.Canceled) {
		return "OAuth login failed: oauth_login_canceled"
	}
	return "OAuth login failed: " + safeErrorMessage(err.Error())
}
