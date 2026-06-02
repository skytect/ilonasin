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
	return bootstrapCore(ctx, opts, false)
}

func bootstrapCore(ctx context.Context, opts Options, createDefaultConfig bool) (*coreRuntime, error) {
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
	cfg, cfgPath, err := loadConfig(opts.ConfigPath, homeDir, createDefaultConfig)
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
	logger, logCloser, err := logging.Setup(loggingOptions(cfg), opts.Stderr)
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
	ioLogger, err := logging.SetupIO(loggingIOOptions(cfg))
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
	registry, err := provider.NewRegistry(providerRegistryConfig(cfg))
	if err != nil {
		cleanup()
		return nil, err
	}
	return &coreRuntime{HomeDir: homeDir, ConfigPath: cfgPath, Config: cfg, Registry: registry, Logger: logger, IOLogger: ioLogger, cleanup: cleanup}, nil
}

func loggingOptions(cfg config.Config) logging.Options {
	return logging.Options{
		Level:   cfg.Logging.Level,
		Format:  cfg.Logging.Format,
		Outputs: append([]string(nil), cfg.Logging.Outputs...),
		LogDir:  cfg.Paths.LogDir,
	}
}

func loggingIOOptions(cfg config.Config) logging.IOOptions {
	return logging.IOOptions{
		Capture: cfg.Logging.CaptureIO,
		LogDir:  cfg.Paths.LogDir,
	}
}

func providerRegistryConfig(cfg config.Config) provider.RegistryConfig {
	providers := make(map[string]provider.ProviderConfig, len(cfg.Providers))
	for id, row := range cfg.Providers {
		providers[id] = provider.ProviderConfig{
			Type:       row.Type,
			BaseURL:    row.BaseURL,
			AuthIssuer: row.AuthIssuer,
		}
	}
	return provider.RegistryConfig{Providers: providers}
}

func subscriptionKeepaliveSettingsFromConfig(cfg config.SubscriptionKeepaliveConfig) subscriptionKeepaliveSettings {
	return subscriptionKeepaliveSettings{
		Enabled:           cfg.Enabled,
		ScheduleTimes:     config.SubscriptionKeepaliveScheduleTimes(cfg.ScheduleTimes),
		Model:             cfg.Model,
		MaxOutputTokens:   cfg.MaxOutputTokens,
		OutputCapVerified: config.SubscriptionKeepaliveOutputCapVerified(cfg),
	}
}

func loadConfig(path, homeDir string, createDefault bool) (config.Config, string, error) {
	if createDefault {
		return config.LoadOrCreate(path, homeDir, path != "")
	}
	return config.Load(path, homeDir)
}
