package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
	"ilonasin/internal/server"
	"ilonasin/internal/tui"
)

const (
	gracefulShutdownTimeout = 20 * time.Second
	publicReadHeaderTimeout = 5 * time.Second
	publicIdleTimeout       = 120 * time.Second
	publicMaxHeaderBytes    = 1 << 20
)

func Serve(opts Options) error {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	rt, err := bootstrap(ctx, opts)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	if err := refreshIOConfiguredSecrets(ctx, rt.IOLogger, rt.Store); err != nil {
		return err
	}
	captureUpstreamIO := rt.IOLogger != nil
	secretRefresh := ioSecretRefreshHook(ctx, rt.IOLogger, rt.Store)
	keepalive := subscriptionKeepaliveSettingsFromConfig(rt.Config.SubscriptionKeepalive)
	auth := credentials.Service{Repo: rt.Store}
	if rt.IOLogger != nil {
		auth.EphemeralSecretAdded = rt.IOLogger.AddEphemeralSecret
	}
	refresher := provider.NewHTTPOAuthRefresher(nil)
	refresher.Logger = rt.Logger
	login := provider.NewHTTPOAuthDeviceLogin(nil)
	login.Logger = rt.Logger
	upstreams := &credentials.UpstreamService{
		Registry:       credentialsProviderRegistry(rt.Registry),
		Repo:           rt.Store,
		OAuthRefresher: credentialsOAuthRefresher(refresher),
		OAuthLogin:     credentialsOAuthDeviceLogin(login),
		Logger:         rt.Logger,
		SecretsChanged: secretRefresh,
	}
	mgmt, err := startManagementServer(ctx, rt.HomeDir, rt.ConfigPath, rt.Config.Paths.Database, rt.Config.Server.Bind, ioRetentionStatusFromConfig(rt.Config.Logging), rt.Registry, rt.Store, upstreams, keepalive, rt.IOLogger, captureUpstreamIO, rt.Logger)
	if err != nil {
		return err
	}
	mgmtClosed := false
	closeManagement := func(ctx context.Context) {
		if mgmtClosed {
			return
		}
		mgmt.Close(ctx)
		mgmtClosed = true
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		closeManagement(closeCtx)
	}()
	rt.Logger.InfoContext(ctx, "serve starting",
		slog.String("event", "app_command_start"),
		slog.String("command", "serve"),
		slog.String("bind", rt.Config.Server.Bind),
	)

	codexVersionResolver := provider.NewCachedCodexClientVersionResolver(nil)
	codexAdapter := provider.NewHTTPChatAdapter(nil)
	codexAdapter.Logger = rt.Logger
	codexAdapter.IOLogger = rt.IOLogger
	codexAdapter.CaptureUpstreamIO = captureUpstreamIO
	codexAdapter.CodexVersionResolver = codexVersionResolver
	adapters := chatAdapters(nil, rt.IOLogger, captureUpstreamIO, rt.Logger)
	adapters["codex"] = codexAdapter
	modelAdapters := modelDiscoverers(nil, rt.IOLogger, captureUpstreamIO, rt.Logger)
	modelAdapters["codex"] = codexAdapter
	responseAdapters := responsesAdapters(nil, rt.IOLogger, captureUpstreamIO, rt.Logger)
	responseAdapters["codex"] = codexAdapter
	stopKeepalive := startSubscriptionKeepalive(ctx, keepalive, keepaliveProviderRegistryFromProvider(rt.Registry), upstreams, keepaliveUsageClientFromProvider(codexAdapter), keepaliveChatClientFromProvider(codexAdapter), rt.Logger)
	keepaliveStopped := false
	stopKeepaliveOnce := func() {
		if keepaliveStopped {
			return
		}
		stopKeepalive()
		keepaliveStopped = true
	}
	defer stopKeepaliveOnce()
	srvHandler := server.New(rt.Registry, auth, upstreams, upstreams, adapters, modelAdapters, rt.Store, rt.Store).
		WithLogger(rt.Logger).
		WithIOLogger(rt.IOLogger).
		WithResponsesAdapters(responseAdapters).
		Handler()
	srv := &http.Server{
		Addr:              rt.Config.Server.Bind,
		Handler:           srvHandler,
		ReadHeaderTimeout: publicReadHeaderTimeout,
		IdleTimeout:       publicIdleTimeout,
		MaxHeaderBytes:    publicMaxHeaderBytes,
	}
	fmt.Fprintf(opts.Stdout, "ilonasin serving on %s\n", rt.Config.Server.Bind)
	errc := make(chan error, 1)
	go func() {
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		stopSignals()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()
		shutdownErrc := make(chan error, 1)
		go func() {
			shutdownErrc <- srv.Shutdown(shutdownCtx)
		}()
		stopKeepaliveOnce()
		closeManagement(shutdownCtx)
		shutdownErr := <-shutdownErrc
		if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
			return shutdownErr
		}
		if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func Manage(opts Options) error {
	rt, err := bootstrapClient(context.Background(), opts)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	rt.Logger.InfoContext(context.Background(), "manage starting",
		slog.String("event", "app_command_start"),
		slog.String("command", "manage"),
	)
	managementClient := management.NewUnixClient(management.SocketPath(rt.HomeDir, rt.ConfigPath, rt.Config.Paths.Database))
	if _, err := managementClient.LoadManagementSnapshot(context.Background()); err != nil {
		return err
	}
	return tui.Run(managementClient, managementClient, managementClient, managementClient, managementClient, rt.Logger)
}
