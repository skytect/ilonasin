package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/logging"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
	"ilonasin/internal/storage/sqlite"
)

type managementRuntime struct {
	socketPath string
	owner      management.SocketOwner
	server     *http.Server
}

type ioRetentionStatus struct {
	maxBytes int64
	maxFiles int
}

func startManagementServer(ctx context.Context, homeDir, configPath, databasePath, bind string, ioRetention ioRetentionStatus, registry provider.Registry, store *sqlite.Store, upstreams *credentials.UpstreamService, keepalive subscriptionKeepaliveSettings, ioLogger *logging.IOLogger, captureUpstreamIO bool, loggers ...*slog.Logger) (managementRuntime, error) {
	logger := firstSlogLogger(loggers)
	usageClient := provider.NewHTTPChatAdapter(provider.NewOutboundHTTPClient(90 * time.Second))
	usageClient.Logger = logger
	usageClient.IOLogger = ioLogger
	usageClient.CaptureUpstreamIO = captureUpstreamIO
	tokens := credentials.Service{Repo: store}
	if ioLogger != nil {
		tokens.EphemeralSecretAdded = ioLogger.AddEphemeralSecret
	}
	return startManagementServerWithService(ctx, homeDir, configPath, databasePath, management.Service{
		Runtime: management.RuntimeStatus{
			Bind:       bind,
			CaptureIO:  ioLogger != nil,
			IOMaxBytes: ioRetention.maxBytes,
			IOMaxFiles: ioRetention.maxFiles,
		},
		Tokens:            tokens,
		Providers:         managementProviderInstances(registry),
		Upstreams:         upstreams,
		UpstreamMutations: upstreams,
		OAuth:             upstreams,
		OAuthMutations:    upstreams,
		OAuthResolver:     upstreams,
		SubscriptionUsage: store,
		UsageClient:       subscriptionUsageProviderAdapter{client: usageClient},
		Keepalive:         managementKeepaliveSettings(keepalive),
		ModelCache:        store,
		Observability:     store,
		Pruner:            store,
		Now:               time.Now,
	})
}

func managementProviderInstances(registry provider.Registry) []management.ProviderInstance {
	rows := registry.List()
	out := make([]management.ProviderInstance, 0, len(rows))
	for _, row := range rows {
		out = append(out, management.ProviderInstance{
			ID:             row.ID,
			Type:           row.Type,
			BaseURL:        row.BaseURL,
			AuthIssuer:     row.AuthIssuer,
			AuthStyle:      row.AuthStyle,
			APIKey:         row.APIKey,
			OAuth:          row.OAuth,
			OAuthRefresh:   row.OAuthRefresh,
			Chat:           row.Chat,
			ModelDiscovery: row.ModelDiscovery,
		})
	}
	return out
}

func managementKeepaliveSettings(keepalive subscriptionKeepaliveSettings) management.SubscriptionKeepaliveSettings {
	return management.SubscriptionKeepaliveSettings{
		Enabled:           keepalive.Enabled,
		OutputCapVerified: keepalive.OutputCapVerified,
		ScheduleTimes:     append([]string(nil), keepalive.ScheduleTimes...),
	}
}

func startManagementServerWithService(ctx context.Context, homeDir, configPath, databasePath string, service management.HandlerService) (managementRuntime, error) {
	socketPath := management.SocketPath(homeDir, configPath, databasePath)
	listener, owner, err := management.PrepareUnixListener(ctx, socketPath)
	if err != nil {
		return managementRuntime{}, err
	}
	srv := &http.Server{
		Handler:           management.Handler(service),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(listener)
	}()
	select {
	case err := <-errc:
		management.CleanupSocket(owner)
		if err == nil || err == http.ErrServerClosed {
			return managementRuntime{}, fmt.Errorf("management server stopped")
		}
		return managementRuntime{}, err
	default:
	}
	return managementRuntime{socketPath: socketPath, owner: owner, server: srv}, nil
}

func firstSlogLogger(loggers []*slog.Logger) *slog.Logger {
	for _, logger := range loggers {
		if logger != nil {
			return logger
		}
	}
	return nil
}

func (m managementRuntime) Close(ctx context.Context) {
	if m.server != nil {
		_ = m.server.Shutdown(ctx)
	}
	management.CleanupSocket(m.owner)
}
