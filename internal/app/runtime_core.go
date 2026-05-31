package app

import (
	"context"
	"io"
	"log/slog"
	"os"

	"ilonasin/internal/config"
	"ilonasin/internal/home"
	"ilonasin/internal/logging"
	"ilonasin/internal/provider"
)

type Options struct {
	ConfigPath string
	Stdout     io.Writer
	Stderr     io.Writer
}

type coreRuntime struct {
	HomeDir    string
	ConfigPath string
	Config     config.Config
	Registry   provider.Registry
	Logger     *slog.Logger
	IOLogger   *logging.IOLogger
	cleanup    func()
}

func bootstrapClient(ctx context.Context, opts Options) (*coreRuntime, error) {
	return bootstrapCore(ctx, opts)
}

func bootstrapCore(ctx context.Context, opts Options) (*coreRuntime, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	envHome := os.Getenv(home.EnvName)
	cleanup := func() {}
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
	logger, logCloser, err := logging.Setup(cfg, opts.Stderr)
	if err != nil {
		cleanup()
		return nil, err
	}
	cleanup = func(previous func()) func() {
		return func() {
			_ = logCloser.Close()
			previous()
		}
	}(cleanup)
	ioLogger, err := logging.SetupIO(cfg)
	if err != nil {
		cleanup()
		return nil, err
	}
	if ioLogger != nil {
		cleanup = func(previous func()) func() {
			return func() {
				_ = ioLogger.Close()
				previous()
			}
		}(cleanup)
	}
	logger.InfoContext(ctx, "application bootstrap complete",
		slog.String("event", "app_bootstrap"),
		slog.String("home_dir", homeDir),
		slog.String("config_file", cfgPath),
		slog.String("log_output", "configured"),
	)
	registry, err := provider.NewRegistry(cfg)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &coreRuntime{HomeDir: homeDir, ConfigPath: cfgPath, Config: cfg, Registry: registry, Logger: logger, IOLogger: ioLogger, cleanup: cleanup}, nil
}
