package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"ilonasin/internal/config"
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

func startManagementServer(ctx context.Context, homeDir, configPath, databasePath, bind string, registry provider.Registry, store *sqlite.Store, keepalive config.SubscriptionKeepaliveConfig, ioLogger *logging.IOLogger, captureUpstreamIO bool, secretRefresh func(context.Context, ...string), loggers ...*slog.Logger) (managementRuntime, error) {
	logger := firstSlogLogger(loggers)
	refresher := provider.NewHTTPOAuthRefresher(nil)
	refresher.Logger = logger
	login := provider.NewHTTPOAuthDeviceLogin(nil)
	login.Logger = logger
	usageClient := provider.NewHTTPChatAdapter(nil)
	usageClient.Logger = logger
	usageClient.IOLogger = ioLogger
	usageClient.CaptureUpstreamIO = captureUpstreamIO
	upstreams := &credentials.UpstreamService{
		Registry:       registry,
		Repo:           store,
		OAuthRefresher: refresher,
		OAuthLogin:     login,
		Logger:         logger,
		SecretsChanged: secretRefresh,
	}
	tokens := credentials.Service{Repo: store}
	if ioLogger != nil {
		tokens.EphemeralSecretAdded = ioLogger.AddEphemeralSecret
	}
	return startManagementServerWithService(ctx, homeDir, configPath, databasePath, management.Service{
		Runtime: management.RuntimeStatus{
			Bind:      bind,
			CaptureIO: ioLogger != nil,
		},
		Tokens:            tokens,
		Registry:          registry,
		Upstreams:         upstreams,
		UpstreamMutations: upstreams,
		OAuth:             upstreams,
		OAuthMutations:    upstreams,
		OAuthResolver:     upstreams,
		SubscriptionUsage: store,
		UsageClient:       usageClient,
		Keepalive:         managementKeepaliveSettings(keepalive),
		ModelCache:        store,
		Observability:     store,
		Pruner:            store,
	})
}

func managementKeepaliveSettings(keepalive config.SubscriptionKeepaliveConfig) management.SubscriptionKeepaliveSettings {
	return management.SubscriptionKeepaliveSettings{
		Enabled:           keepalive.Enabled,
		OutputCapVerified: config.SubscriptionKeepaliveOutputCapVerified(keepalive),
		ScheduleTimes:     config.SubscriptionKeepaliveScheduleTimes(keepalive.ScheduleTimes),
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
