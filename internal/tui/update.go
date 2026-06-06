package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case snapshotAutoRefreshTickMsg:
		cmds := []tea.Cmd{snapshotAutoRefreshTickCmd(snapshotAutoRefreshInterval)}
		if !m.snapshotRefreshInFlight {
			m.snapshotRefreshInFlight = true
			cmds = append(cmds, m.refreshSnapshotCmd(true))
		}
		return m, tea.Batch(cmds...)
	case snapshotRefreshedMsg:
		m.snapshotRefreshInFlight = false
		pendingForeground := m.snapshotForegroundPending
		m.snapshotForegroundPending = false
		if msg.err != nil {
			if msg.background {
				m.logError(context.Background(), "tui_snapshot_refresh_failed", msg.err)
				if pendingForeground {
					next, cmd := m.startSnapshotRefresh(false)
					return next, cmd
				}
				return m, nil
			}
			m.err = "snapshot refresh failed"
			m.logError(context.Background(), "tui_snapshot_refresh_failed", msg.err)
			if pendingForeground {
				next, cmd := m.startSnapshotRefresh(false)
				return next, cmd
			}
			return m, nil
		}
		if msg.background && pendingForeground {
			next, cmd := m.startSnapshotRefresh(false)
			return next, cmd
		}
		m.applySnapshot(msg.snapshot, snapshotApplyOptions{applySubscriptionUsage: !msg.background})
		if pendingForeground {
			next, cmd := m.startSnapshotRefresh(false)
			return next, cmd
		}
		return m, nil
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
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
	case localTokenCreatedMsg:
		m = m.completeMutation()
		if msg.err != nil {
			m.logError(context.Background(), "tui_local_token_create_failed", msg.err)
			m.err = msg.err.Error()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_local_token_created", slog.Int64("local_id", msg.metadata.ID))
		m.revealTokenID = msg.metadata.ID
		m.revealTokenPrefix = msg.metadata.TokenPrefix
		m.revealTokenLast4 = msg.metadata.TokenLast4
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
	case localTokenDisabledMsg:
		m = m.completeMutation()
		if msg.err != nil {
			m.logError(context.Background(), "tui_local_token_disable_failed", msg.err, slog.Int64("local_id", msg.id))
			m.err = msg.err.Error()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_local_token_disabled", slog.Int64("local_id", msg.id))
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
	case upstreamAPIKeyAddedMsg:
		m = m.completeMutation()
		if msg.err != nil {
			m.logError(context.Background(), "tui_upstream_credential_create_failed", msg.err, slog.String("provider_instance", msg.providerID))
			m.err = msg.err.Error()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_upstream_credential_created",
			slog.String("provider_instance", msg.providerID),
			slog.Int64("credential_id", msg.created.Credential.ID),
		)
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
	case upstreamCredentialDisabledMsg:
		m = m.completeMutation()
		if msg.err != nil {
			m.logError(context.Background(), "tui_upstream_credential_disable_failed", msg.err)
			m.err = msg.err.Error()
			return m, nil
		}
		m.logInfo(context.Background(), "tui_upstream_credential_disabled",
			slog.String("provider_instance", msg.credential.ProviderInstanceID),
			slog.Int64("credential_id", msg.credential.ID),
		)
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
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
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
	case oauthCredentialRefreshedMsg:
		m = m.completeMutation()
		if msg.err != nil {
			m.logError(context.Background(), "tui_oauth_refresh_failed", msg.err)
			m.err = "OAuth refresh failed"
			next, cmd := m.startSnapshotRefresh(false)
			return next, cmd
		}
		m.logInfo(context.Background(), "tui_oauth_refreshed",
			slog.String("provider_instance", msg.row.ProviderInstanceID),
			slog.Int64("credential_id", msg.row.ID),
		)
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
	case telemetryPrunedMsg:
		m = m.completeMutation()
		if msg.err != nil {
			m.logError(context.Background(), "tui_telemetry_prune_failed", msg.err)
			m.err = "telemetry prune failed"
			return m, nil
		}
		result := msg.result
		m.pruneResult = &result
		m.logInfo(context.Background(), "tui_telemetry_pruned",
			slog.Int("requests", result.Requests),
			slog.Int("streams", result.Streams),
			slog.Int("fallbacks", result.Fallbacks),
			slog.Int("health", result.Health),
		)
		next, cmd := m.startSnapshotRefresh(false)
		return next, cmd
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
