package app

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"ilonasin/internal/config"
	"ilonasin/internal/home"
	"ilonasin/internal/provider"
	"ilonasin/internal/storage/sqlite"
)

type Options struct {
	ConfigPath string
	Stdout     io.Writer
	Stderr     io.Writer
}

type runtime struct {
	HomeDir    string
	ConfigPath string
	Config     config.Config
	Registry   provider.Registry
	Store      *sqlite.Store
	cleanup    func()
}

func bootstrap(ctx context.Context, opts Options, checkSafeHome bool) (*runtime, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	envHome := os.Getenv(home.EnvName)
	cleanup := func() {}
	if checkSafeHome && envHome == "" {
		tmp, err := os.MkdirTemp("", "ilonasin-check-*")
		if err != nil {
			return nil, err
		}
		envHome = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	}
	homeDir, err := home.Resolve(envHome)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := home.Ensure(homeDir); err != nil {
		cleanup()
		return nil, err
	}
	cfg, cfgPath, err := config.LoadOrCreate(opts.ConfigPath, homeDir, opts.ConfigPath != "")
	if err != nil {
		cleanup()
		return nil, err
	}
	for _, dir := range []string{cfg.Paths.DataDir, cfg.Paths.LogDir, cfg.Paths.CacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			cleanup()
			return nil, err
		}
	}
	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		cleanup()
		return nil, err
	}
	store, err := sqlite.Open(ctx, filepath.Clean(cfg.Paths.Database))
	if err != nil {
		cleanup()
		return nil, err
	}
	return &runtime{HomeDir: homeDir, ConfigPath: cfgPath, Config: cfg, Registry: registry, Store: store, cleanup: cleanup}, nil
}
