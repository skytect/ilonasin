package tui

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/config"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case oauthLoginStartedMsg:
		if msg.err != nil {
			m.logError(context.Background(), "tui_oauth_login_start_failed", msg.err)
			m.err = oauthLoginErrorMessage(msg.err)
			m.oauthChallenge = nil
			m.cancelOAuthLogin()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_oauth_login_started")
		m.err = ""
		challenge := msg.challenge
		m.oauthChallenge = &challenge
		return m, m.completeOAuthLoginCmd(challenge.Handle)
	case oauthLoginCompletedMsg:
		m.oauthChallenge = nil
		m.cancelOAuthLogin()
		if msg.err != nil {
			m.logError(context.Background(), "tui_oauth_login_complete_failed", msg.err)
			m.err = oauthLoginErrorMessage(msg.err)
			return m, nil
		}
		m.logInfo(context.Background(), "tui_oauth_login_completed")
		_ = m.reload()
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScrolls()
		return m, nil
	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			m.scrollActive(-3)
		case tea.MouseWheelDown:
			m.scrollActive(3)
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.apiKeyMode {
			return m.updateAPIKeyInput(key)
		}
		switch key.String() {
		case "tab", "right":
			m.activeTab = (m.activeTab + 1) % tuiTabCount
			m.clampScrolls()
		case "shift+tab", "left":
			m.activeTab = (m.activeTab + tuiTabCount - 1) % tuiTabCount
			m.clampScrolls()
		case "1":
			m.activeTab = tabOverview
			m.clampScrolls()
		case "2":
			m.activeTab = tabAccounts
			m.clampScrolls()
		case "3":
			m.activeTab = tabObservability
			m.clampScrolls()
		case "4":
			m.activeTab = tabHelp
			m.clampScrolls()
		case "pgdown", "ctrl+d":
			m.scrollActive(m.viewportHeight())
		case "pgup", "ctrl+u":
			m.scrollActive(-m.viewportHeight())
		case "home":
			m.setActiveScroll(0)
		case "end":
			m.setActiveScroll(m.activeScrollMax())
		case "q", "ctrl+c":
			m.clearReveal()
			m.cancelOAuthLogin()
			return m, tea.Quit
		case "n":
			if m.activeTab != tabAccounts {
				return m, nil
			}
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
		case "d":
			if m.activeTab != tabAccounts {
				return m, nil
			}
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
		case "x":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if err := m.disableFirstUpstreamCredential(); err != nil {
				m.logError(context.Background(), "tui_upstream_credential_disable_failed", err)
				m.err = err.Error()
				return m, nil
			}
			_ = m.reload()
		case "a":
			if m.activeTab != tabAccounts {
				return m, nil
			}
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
		case "p":
			if m.activeTab != tabObservability {
				return m, nil
			}
			m.clearReveal()
			if err := m.pruneTelemetry(); err != nil {
				m.logError(context.Background(), "tui_telemetry_prune_failed", err)
				m.err = "telemetry prune failed"
				return m, nil
			}
			_ = m.reload()
		case "u":
			if m.activeTab != tabObservability {
				return m, nil
			}
			m.clearReveal()
			if m.subscriptionUsage == nil {
				return m, nil
			}
			resp, err := m.subscriptionUsage.RefreshSubscriptionUsage(context.Background())
			if err != nil {
				m.logError(context.Background(), "tui_subscription_usage_refresh_failed", err)
				m.err = "subscription usage refresh failed"
				return m, nil
			}
			m.applySubscriptionUsage(resp)
			_ = m.reload()
		case "l":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			m.cancelOAuthLogin()
			providerID, ok := firstOAuthLoginProvider(m.registry)
			if !ok || m.oauth == nil {
				m.logInfo(context.Background(), "tui_oauth_login_unavailable")
				m.err = "OAuth login failed"
				return m, nil
			}
			loginCtx, cancel := context.WithCancel(context.Background())
			m.oauthCtx = loginCtx
			m.oauthCancel = cancel
			return m, m.startOAuthLoginCmd(loginCtx, providerID)
		case "r":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if err := m.refreshSelectedOAuthCredential(); err != nil {
				m.logError(context.Background(), "tui_oauth_refresh_failed", err)
				m.err = "OAuth refresh failed"
				_ = m.reload()
				return m, nil
			}
			_ = m.reload()
		case "o":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if len(m.oauthRows) > 0 {
				m.oauthSelected = (m.oauthSelected + 1) % len(m.oauthRows)
			}
		case "f":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if err := m.enableFirstFallbackPolicy(); err != nil {
				m.logError(context.Background(), "tui_fallback_policy_update_failed", err)
				m.err = "fallback policy update failed"
				return m, nil
			}
			_ = m.reload()
		case "F":
			if m.activeTab != tabAccounts {
				return m, nil
			}
			m.clearReveal()
			if err := m.disableFirstFallbackPolicy(); err != nil {
				m.logError(context.Background(), "tui_fallback_policy_update_failed", err)
				m.err = "fallback policy update failed"
				return m, nil
			}
			_ = m.reload()
		case "esc":
			m.clearReveal()
			m.oauthChallenge = nil
			m.cancelOAuthLogin()
		case "enter":
			m.clearReveal()
		case "down", "j":
			m.clearReveal()
			if m.activeTab == tabAccounts {
				m.selectNextLocalToken()
			} else {
				m.scrollActive(1)
			}
		case "up", "k":
			m.clearReveal()
			if m.activeTab == tabAccounts {
				m.selectPreviousLocalToken()
			} else {
				m.scrollActive(-1)
			}
		}
	}
	return m, nil
}

func Run(cfg config.Config, registry provider.Registry, snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, loggers ...*slog.Logger) error {
	if snapshot == nil {
		return fmt.Errorf("management snapshot client is required")
	}
	var subscriptionUsage management.SubscriptionUsageClient
	if client, ok := snapshot.(management.SubscriptionUsageClient); ok {
		subscriptionUsage = client
	}
	model := NewModel(cfg, registry, tokens, upstreams, oauth, pruner, subscriptionUsage, nil, loggers...)
	model.snapshot = snapshot
	if err := model.reload(); err != nil {
		return err
	}
	_, err := tea.NewProgram(model, tea.WithMouseCellMotion()).Run()
	return err
}
