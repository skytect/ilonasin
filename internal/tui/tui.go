package tui

import (
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"ilonasin/internal/config"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
)

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
