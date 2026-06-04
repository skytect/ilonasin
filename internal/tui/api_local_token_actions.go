package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/management"
)

func (m *Model) clearReveal() {
	m.revealTokenID = 0
	m.revealTokenPrefix = ""
	m.revealTokenLast4 = ""
}

func (m *Model) selectNextLocalToken() {
	if m.selected < len(m.tokenRows)-1 {
		m.selected++
	}
}

func (m *Model) selectPreviousLocalToken() {
	if m.selected > 0 {
		m.selected--
	}
}

func (m Model) createLocalToken() (tea.Model, tea.Cmd) {
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
	next, cmd := m.startSnapshotRefresh(false)
	return next, cmd
}

func (m Model) disableSelectedLocalToken() (tea.Model, tea.Cmd) {
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
	next, cmd := m.startSnapshotRefresh(false)
	return next, cmd
}
