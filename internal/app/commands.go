package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/management"
	"ilonasin/internal/provider"
	"ilonasin/internal/server"
	"ilonasin/internal/tui"
)

func Serve(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, false)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	mgmt, err := startManagementServer(context.Background(), rt.HomeDir, rt.ConfigPath, rt.Config.Paths.Database, rt.Store)
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
	srv := &http.Server{
		Addr:              rt.Config.Server.Bind,
		Handler:           server.New(rt.Registry, auth, upstreams, upstreams, chatAdapters(nil, rt.Logger), modelDiscoverers(nil, rt.Logger), rt.Store, rt.Store).WithLogger(rt.Logger).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(opts.Stdout, "ilonasin serving on %s\n", rt.Config.Server.Bind)
	return srv.ListenAndServe()
}

func Manage(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, false)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	rt.Logger.InfoContext(context.Background(), "manage starting",
		slog.String("event", "app_command_start"),
		slog.String("command", "manage"),
	)
	refresher := provider.NewHTTPOAuthRefresher(nil)
	refresher.Logger = rt.Logger
	login := provider.NewHTTPOAuthDeviceLogin(nil)
	login.Logger = rt.Logger
	upstreams := &credentials.UpstreamService{
		Registry:       rt.Registry,
		Repo:           rt.Store,
		OAuthRefresher: refresher,
		OAuthLogin:     login,
		Logger:         rt.Logger,
	}
	tokenClient := management.NewUnixLocalTokenClient(management.SocketPath(rt.HomeDir, rt.ConfigPath, rt.Config.Paths.Database))
	if _, err := tokenClient.ListLocalTokens(context.Background()); err != nil {
		return err
	}
	return tui.Run(rt.Config, rt.Registry, tokenClient, upstreams, upstreams, rt.Store, rt.Store, rt.Store, rt.Logger)
}
