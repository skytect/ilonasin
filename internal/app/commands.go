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
	"ilonasin/internal/server"
	"ilonasin/internal/tui"
)

func Serve(opts Options) error {
	rt, err := bootstrap(context.Background(), opts)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	captureUpstreamIO := rt.IOLogger != nil && logging.DebugEnabled(rt.Config.Logging.Level)
	mgmt, err := startManagementServer(context.Background(), rt.HomeDir, rt.ConfigPath, rt.Config.Paths.Database, rt.Registry, rt.Store, rt.Config.SubscriptionKeepalive, rt.IOLogger, captureUpstreamIO, rt.Logger)
	if err != nil {
		return err
	}
	defer mgmt.Close(context.Background())
	rt.Logger.InfoContext(context.Background(), "serve starting",
		slog.String("event", "app_command_start"),
		slog.String("command", "serve"),
		slog.String("bind", rt.Config.Server.Bind),
	)

	auth := credentials.Service{Repo: rt.Store}
	upstreams := &credentials.UpstreamService{
		Registry:       rt.Registry,
		Repo:           rt.Store,
		OAuthRefresher: provider.NewHTTPOAuthRefresher(nil),
		Logger:         rt.Logger,
	}
	refresher := provider.NewHTTPOAuthRefresher(nil)
	refresher.Logger = rt.Logger
	upstreams.OAuthRefresher = refresher
	codexAdapter := provider.NewHTTPChatAdapter(nil)
	codexAdapter.Logger = rt.Logger
	codexAdapter.IOLogger = rt.IOLogger
	codexAdapter.CaptureUpstreamIO = captureUpstreamIO
	adapters := chatAdapters(nil, rt.IOLogger, captureUpstreamIO, rt.Logger)
	adapters["codex"] = codexAdapter
	stopKeepalive := startSubscriptionKeepalive(context.Background(), rt.Config.SubscriptionKeepalive, rt.Registry, upstreams, codexAdapter, codexAdapter, rt.Logger)
	defer stopKeepalive()
	srv := &http.Server{
		Addr:              rt.Config.Server.Bind,
		Handler:           server.New(rt.Registry, auth, upstreams, upstreams, adapters, modelDiscoverers(nil, rt.IOLogger, captureUpstreamIO, rt.Logger), rt.Store, rt.Store).WithLogger(rt.Logger).WithIOLogger(rt.IOLogger).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(opts.Stdout, "ilonasin serving on %s\n", rt.Config.Server.Bind)
	return srv.ListenAndServe()
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
	return tui.Run(rt.Config, managementClient, managementClient, managementClient, managementClient, managementClient, rt.Logger)
}
