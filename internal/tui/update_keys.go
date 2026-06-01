package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m.createLocalToken()
	case "d":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.disableSelectedLocalToken()
	case "x":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.disableUpstreamCredentialAction()
	case "a":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.startAPIKeyInput()
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
		return m.enableFallbackPolicyAction()
	case "F":
		if m.activeTab != tabAccounts {
			return m, nil
		}
		return m.disableFallbackPolicyAction()
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
	return m, nil
}
