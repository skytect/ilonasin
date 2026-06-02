package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case subscriptionUsageAutoRefreshTickMsg:
		cmds := []tea.Cmd{subscriptionUsageAutoRefreshTickCmd(subscriptionUsageAutoRefreshInterval)}
		if !m.subscriptionRefreshInFlight && m.subscriptionUsageIsStale(m.nowTime()) {
			m.subscriptionRefreshInFlight = true
			cmds = append(cmds, m.refreshSubscriptionUsageCmd(false))
		}
		return m, tea.Batch(cmds...)
	case subscriptionUsageRefreshedMsg:
		m.subscriptionRefreshInFlight = false
		if msg.err != nil {
			m.logError(context.Background(), "tui_subscription_usage_refresh_failed", msg.err)
			if msg.manual {
				m.err = "subscription usage refresh failed"
			}
			return m, nil
		}
		m.applySubscriptionUsage(msg.response)
		_ = m.reload()
		return m, nil
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
			if !m.scrollPaneAtViewPosition(msg.X, msg.Y, -3) {
				m.scrollFocusedPane(-3)
			}
		case tea.MouseWheelDown:
			if !m.scrollPaneAtViewPosition(msg.X, msg.Y, 3) {
				m.scrollFocusedPane(3)
			}
		case tea.MouseLeft:
			if msg.Action != tea.MouseActionPress {
				return m, nil
			}
			if tab, ok := m.tabAtViewPosition(msg.X, msg.Y); ok {
				m.activeTab = tab
				m.clampScrolls()
				return m, nil
			}
			m.focusPaneAtViewPosition(msg.X, msg.Y)
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		return m.updateKey(key)
	}
	return m, nil
}
