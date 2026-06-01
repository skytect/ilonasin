package tui

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

func (m Model) pruneTelemetryAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if err := m.pruneTelemetry(); err != nil {
		m.logError(context.Background(), "tui_telemetry_prune_failed", err)
		m.err = "telemetry prune failed"
		return m, nil
	}
	_ = m.reload()
	return m, nil
}

func (m Model) refreshSubscriptionUsageAction() (tea.Model, tea.Cmd) {
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
	return m, nil
}

func (m *Model) pruneTelemetry() error {
	if m.pruner == nil {
		return nil
	}
	cutoff := m.nowTime().Add(-30 * 24 * time.Hour).UTC()
	resp, err := m.pruner.PruneTelemetry(context.Background(), management.PruneTelemetryRequest{Cutoff: cutoff})
	if err != nil {
		return err
	}
	result := resp.Result
	m.pruneResult = &result
	m.logInfo(context.Background(), "tui_telemetry_pruned",
		slog.Int("requests", result.Requests),
		slog.Int("streams", result.Streams),
		slog.Int("fallbacks", result.Fallbacks),
		slog.Int("health", result.Health),
	)
	return nil
}
