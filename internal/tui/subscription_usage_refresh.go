package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

const subscriptionUsageAutoRefreshInterval = time.Minute

type subscriptionUsageAutoRefreshTickMsg struct{}

type subscriptionUsageRefreshedMsg struct {
	response management.SubscriptionUsageResponse
	err      error
	manual   bool
}

func subscriptionUsageAutoRefreshTickCmd(after time.Duration) tea.Cmd {
	return func() tea.Msg {
		if after > 0 {
			time.Sleep(after)
		}
		return subscriptionUsageAutoRefreshTickMsg{}
	}
}

func (m Model) refreshSubscriptionUsageCmd(manual bool) tea.Cmd {
	return func() tea.Msg {
		if m.subscriptionUsage == nil {
			return subscriptionUsageRefreshedMsg{manual: manual}
		}
		resp, err := m.subscriptionUsage.RefreshSubscriptionUsage(context.Background())
		return subscriptionUsageRefreshedMsg{response: resp, err: err, manual: manual}
	}
}

func (m Model) subscriptionUsageIsStale(now time.Time) bool {
	if len(m.oauthRows) == 0 {
		return false
	}
	if len(m.subscriptionRows) == 0 && len(m.subscriptionPools) == 0 {
		return true
	}
	observed := m.subscriptionObservedAt
	if observed.IsZero() {
		return true
	}
	return !observed.Add(subscriptionUsageAutoRefreshInterval).After(now.UTC())
}
