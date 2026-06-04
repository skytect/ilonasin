package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

const (
	snapshotAutoRefreshInterval = 5 * time.Second
	snapshotRefreshTimeout      = 3 * time.Second
)

type snapshotAutoRefreshTickMsg struct{}

type snapshotRefreshedMsg struct {
	snapshot   management.ManagementSnapshotResponse
	err        error
	background bool
}

func snapshotAutoRefreshTickCmd(after time.Duration) tea.Cmd {
	return func() tea.Msg {
		if after > 0 {
			time.Sleep(after)
		}
		return snapshotAutoRefreshTickMsg{}
	}
}

func (m Model) refreshSnapshotCmd(background bool) tea.Cmd {
	return func() tea.Msg {
		if m.snapshot == nil {
			return snapshotRefreshedMsg{err: errMissingSnapshotClient(), background: background}
		}
		ctx, cancel := context.WithTimeout(context.Background(), snapshotRefreshTimeout)
		defer cancel()
		snapshot, err := m.snapshot.LoadManagementSnapshot(ctx)
		return snapshotRefreshedMsg{snapshot: snapshot, err: err, background: background}
	}
}

func (m Model) startSnapshotRefresh(background bool) (Model, tea.Cmd) {
	if m.snapshotRefreshInFlight {
		if !background {
			m.snapshotForegroundPending = true
		}
		return m, nil
	}
	m.snapshotRefreshInFlight = true
	if !background {
		m.snapshotForegroundPending = false
	}
	return m, m.refreshSnapshotCmd(background)
}
