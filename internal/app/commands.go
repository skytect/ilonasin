package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
	"ilonasin/internal/server"
	"ilonasin/internal/tui"
)

func Serve(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, false)
	if err != nil {
		return err
	}
	defer rt.Store.Close()

	auth := credentials.Service{Repo: rt.Store}
	upstreams := &credentials.UpstreamService{
		Registry:       rt.Registry,
		Repo:           rt.Store,
		OAuthRefresher: provider.NewHTTPOAuthRefresher(nil),
	}
	srv := &http.Server{
		Addr:              rt.Config.Server.Bind,
		Handler:           server.New(rt.Registry, auth, upstreams, upstreams, chatAdapters(nil), modelDiscoverers(nil), rt.Store, rt.Store).Handler(),
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
	defer rt.Store.Close()
	upstreams := &credentials.UpstreamService{
		Registry:       rt.Registry,
		Repo:           rt.Store,
		OAuthRefresher: provider.NewHTTPOAuthRefresher(nil),
		OAuthLogin:     provider.NewHTTPOAuthDeviceLogin(nil),
	}
	return tui.Run(rt.Config, rt.Registry, credentials.Service{Repo: rt.Store}, upstreams, upstreams, rt.Store, rt.Store, rt.Store)
}
