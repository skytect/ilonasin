package tui

import (
	"context"
	"errors"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

type oauthLoginStartedMsg struct {
	challenge credentials.OAuthDeviceLoginChallenge
	err       error
}

type oauthLoginCompletedMsg struct {
	err error
}

func oauthChallengeFromManagement(row management.OAuthDeviceLoginChallenge) credentials.OAuthDeviceLoginChallenge {
	return credentials.OAuthDeviceLoginChallenge{
		ProviderInstanceID: row.ProviderInstanceID,
		VerificationURL:    row.VerificationURL,
		UserCode:           row.UserCode,
		Handle:             row.Handle,
	}
}

func (m Model) startOAuthLoginCmd(ctx context.Context, providerInstanceID string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.oauth.StartOAuthDeviceLogin(ctx, management.StartOAuthDeviceLoginRequest{ProviderInstanceID: providerInstanceID})
		return oauthLoginStartedMsg{challenge: oauthChallengeFromManagement(resp.Challenge), err: err}
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

func (m *Model) refreshSelectedOAuthCredential() error {
	if m.oauth == nil || len(m.oauthRows) == 0 {
		return nil
	}
	if m.oauthSelected < 0 || m.oauthSelected >= len(m.oauthRows) {
		return nil
	}
	row := m.oauthRows[m.oauthSelected]
	if row.Disabled {
		return nil
	}
	if _, err := m.oauth.RefreshOAuthCredential(context.Background(), management.RefreshOAuthCredentialRequest{ID: row.ID}); err != nil {
		return err
	}
	m.logInfo(context.Background(), "tui_oauth_refreshed",
		slog.String("provider_instance", row.ProviderInstanceID),
		slog.Int64("credential_id", row.ID),
	)
	return nil
}

func firstOAuthLoginProvider(registry provider.Registry) (string, bool) {
	for _, instance := range registry.List() {
		if instance.Type == "codex" && instance.OAuth {
			return instance.ID, true
		}
	}
	return "", false
}

func oauthLoginErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var loginErr provider.OAuthDeviceLoginError
	if errors.As(err, &loginErr) && loginErr.Class != "" {
		if loginErr.EventID != "" {
			return "OAuth login failed: " + loginErr.Class + " event_id=" + loginErr.EventID
		}
		return "OAuth login failed: " + loginErr.Class
	}
	if errors.Is(err, context.Canceled) {
		return "OAuth login failed: oauth_login_canceled"
	}
	if errors.Is(err, credentials.ErrNoEligibleCredential) {
		return "OAuth login failed: oauth_login_expired"
	}
	return "OAuth login failed: " + safeErrorMessage(err.Error())
}
