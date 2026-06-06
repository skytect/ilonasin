package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

type telemetryPrunedMsg struct {
	result management.PruneResult
	err    error
}

func (m Model) updateUsageLogKey(key tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch key.String() {
	case "p":
		if m.activeTab != tabLogs {
			return m, nil, true
		}
		next, cmd := m.pruneTelemetryAction()
		return next, cmd, true
	case "u":
		if m.activeTab != tabUsage {
			return m, nil, true
		}
		next, cmd := m.refreshSubscriptionUsageAction()
		return next, cmd, true
	}
	return m, nil, false
}

func (m Model) pruneTelemetryAction() (tea.Model, tea.Cmd) {
	if m.mutationInFlight {
		return m, nil
	}
	m.clearReveal()
	if m.pruner == nil {
		return m, nil
	}
	return m.startMutation(m.pruneTelemetryCmd())
}

func (m Model) refreshSubscriptionUsageAction() (tea.Model, tea.Cmd) {
	m.clearReveal()
	if m.subscriptionUsage == nil || m.subscriptionRefreshInFlight {
		return m, nil
	}
	m.subscriptionRefreshInFlight = true
	return m, m.refreshSubscriptionUsageCmd(true)
}

func (m Model) pruneTelemetryCmd() tea.Cmd {
	cutoff := m.nowTime().Add(-30 * 24 * time.Hour).UTC()
	return func() tea.Msg {
		resp, err := m.pruner.PruneTelemetry(context.Background(), management.PruneTelemetryRequest{Cutoff: cutoff})
		return telemetryPrunedMsg{result: resp.Result, err: err}
	}
}
