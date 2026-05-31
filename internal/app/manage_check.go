package app

import (
	"bytes"
	"context"
	"fmt"

	"ilonasin/internal/credentials"
	"ilonasin/internal/provider"
	"ilonasin/internal/tui"
)

func ManageCheck(opts Options) error {
	rt, err := bootstrap(context.Background(), opts, true)
	if err != nil {
		return err
	}
	defer rt.cleanup()
	defer rt.Store.Close()
	beforeSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	if err := exerciseLocalTokenCheck(context.Background()); err != nil {
		return err
	}
	if err := exerciseUpstreamCredentialCheck(context.Background(), rt.Registry, rt.Config, opts); err != nil {
		return err
	}
	if err := exerciseFallbackPolicyCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseModelCacheCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseObservabilityCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseOAuthCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	if err := exerciseOAuthDeviceLoginCheck(context.Background(), rt.Config); err != nil {
		return err
	}
	if err := exerciseOAuthRefreshCheck(context.Background(), rt.Config); err != nil {
		return err
	}
	if err := exerciseTelemetryPruneCheck(context.Background(), rt.Registry, rt.Config); err != nil {
		return err
	}
	var buf bytes.Buffer
	tokenService := credentials.Service{Repo: rt.Store}
	upstreams := &credentials.UpstreamService{
		Registry:       rt.Registry,
		Repo:           rt.Store,
		OAuthRefresher: provider.NewHTTPOAuthRefresher(nil),
		OAuthLogin:     provider.NewHTTPOAuthDeviceLogin(nil),
	}
	if err := tui.Check(rt.Config, rt.Registry, tokenService, upstreams, upstreams, rt.Store, rt.Store, rt.Store, &buf); err != nil {
		return err
	}
	afterSnapshot, err := selectedHomeSnapshot(context.Background(), rt.Store, rt.ConfigPath)
	if err != nil {
		return err
	}
	if afterSnapshot != beforeSnapshot {
		return fmt.Errorf("manage check mutated selected home metadata")
	}
	if opts.Stdout != nil {
		_, _ = opts.Stdout.Write(buf.Bytes())
	}
	return nil
}
