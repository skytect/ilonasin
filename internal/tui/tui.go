package tui

import (
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"ilonasin/internal/management"
)

func Run(snapshot management.SnapshotClient, tokens management.LocalTokenClient, upstreams management.UpstreamCredentialClient, oauth management.OAuthClient, pruner management.TelemetryPruneClient, loggers ...*slog.Logger) error {
	if snapshot == nil {
		return fmt.Errorf("management snapshot client is required")
	}
	var subscriptionUsage management.SubscriptionUsageClient
	if client, ok := snapshot.(management.SubscriptionUsageClient); ok {
		subscriptionUsage = client
	}
	model := NewModel(tokens, upstreams, oauth, pruner, subscriptionUsage, nil, loggers...)
	model.snapshot = snapshot
	if err := model.reload(); err != nil {
		return err
	}
	lipgloss.SetColorProfile(termenv.ANSI256)
	_, err := tea.NewProgram(model, tea.WithMouseCellMotion()).Run()
	return err
}
