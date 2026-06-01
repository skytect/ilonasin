package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"ilonasin/internal/home"
)

type Config struct {
	Server                ServerConfig                `toml:"server"`
	Paths                 PathsConfig                 `toml:"paths"`
	Logging               LoggingConfig               `toml:"logging"`
	SubscriptionKeepalive SubscriptionKeepaliveConfig `toml:"subscription_keepalive"`
	Providers             map[string]ProviderConfig   `toml:"providers"`
}

type ServerConfig struct {
	Bind string `toml:"bind"`
}

type PathsConfig struct {
	DataDir  string `toml:"data_dir"`
	Database string `toml:"database"`
	LogDir   string `toml:"log_dir"`
	CacheDir string `toml:"cache_dir"`
}

type LoggingConfig struct {
	Level     string   `toml:"level"`
	Format    string   `toml:"format"`
	Outputs   []string `toml:"outputs"`
	CaptureIO bool     `toml:"capture_io"`
}

type SubscriptionKeepaliveConfig struct {
	Enabled         bool     `toml:"enabled"`
	Timezone        string   `toml:"timezone"`
	ScheduleTimes   []string `toml:"schedule_times"`
	Model           string   `toml:"model"`
	MaxOutputTokens int      `toml:"max_output_tokens"`
}

type ProviderConfig struct {
	Type                string `toml:"type"`
	BaseURL             string `toml:"base_url"`
	AuthIssuer          string `toml:"auth_issuer"`
	CodexAccountPooling bool   `toml:"codex_account_pooling"`
}

func Default(homeDir string) Config {
	return Config{
		Server: ServerConfig{Bind: "127.0.0.1:11435"},
		Paths: PathsConfig{
			DataDir:  homeDir,
			Database: filepath.Join(homeDir, "ilonasin.sqlite"),
			LogDir:   filepath.Join(homeDir, "logs"),
			CacheDir: filepath.Join(homeDir, "cache"),
		},
		Logging: LoggingConfig{
			Level:   "info",
			Format:  "json",
			Outputs: []string{"file"},
		},
		SubscriptionKeepalive: SubscriptionKeepaliveConfig{
			Timezone:        "local",
			ScheduleTimes:   []string{"07:00", "12:00", "17:00", "22:00"},
			MaxOutputTokens: 1,
		},
		Providers: map[string]ProviderConfig{
			"deepseek":   {Type: "deepseek"},
			"openrouter": {Type: "openrouter"},
			"codex":      {Type: "codex"},
		},
	}
}

func LoadOrCreate(path, homeDir string, explicit bool) (Config, string, error) {
	if path == "" {
		path = filepath.Join(homeDir, "config.toml")
	}
	path = canonicalPath(home.ExpandPath(path, homeDir))

	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, "", err
		}
		if explicit {
			return Config{}, "", fmt.Errorf("config file does not exist: %s", path)
		}
		cfg := Default(homeDir)
		if err := writeDefault(path, cfg); err != nil {
			return Config{}, "", err
		}
		return cfg, path, nil
	}

	var cfg Config
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, "", err
	}
	if meta.IsDefined("logging", "outputs") && len(cfg.Logging.Outputs) == 0 {
		return Config{}, "", fmt.Errorf("logging outputs must not be empty")
	}
	cfg.applyDefaults(homeDir)
	cfg.Paths.DataDir = canonicalPath(home.ExpandPath(cfg.Paths.DataDir, homeDir))
	cfg.Paths.Database = canonicalPath(home.ExpandPath(cfg.Paths.Database, homeDir))
	cfg.Paths.LogDir = canonicalPath(home.ExpandPath(cfg.Paths.LogDir, homeDir))
	cfg.Paths.CacheDir = canonicalPath(home.ExpandPath(cfg.Paths.CacheDir, homeDir))
	return cfg, path, nil
}

func (c *Config) applyDefaults(homeDir string) {
	def := Default(homeDir)
	if c.Server.Bind == "" {
		c.Server.Bind = def.Server.Bind
	}
	if c.Paths.DataDir == "" {
		c.Paths.DataDir = def.Paths.DataDir
	}
	if c.Paths.Database == "" {
		c.Paths.Database = def.Paths.Database
	}
	if c.Paths.LogDir == "" {
		c.Paths.LogDir = def.Paths.LogDir
	}
	if c.Paths.CacheDir == "" {
		c.Paths.CacheDir = def.Paths.CacheDir
	}
	if c.Logging.Level == "" {
		c.Logging.Level = def.Logging.Level
	}
	if c.Logging.Format == "" {
		c.Logging.Format = def.Logging.Format
	}
	if len(c.Logging.Outputs) == 0 {
		c.Logging.Outputs = append([]string(nil), def.Logging.Outputs...)
	}
	if c.SubscriptionKeepalive.Timezone == "" {
		c.SubscriptionKeepalive.Timezone = def.SubscriptionKeepalive.Timezone
	}
	if len(c.SubscriptionKeepalive.ScheduleTimes) == 0 {
		c.SubscriptionKeepalive.ScheduleTimes = append([]string(nil), def.SubscriptionKeepalive.ScheduleTimes...)
	}
	if c.SubscriptionKeepalive.MaxOutputTokens == 0 {
		c.SubscriptionKeepalive.MaxOutputTokens = def.SubscriptionKeepalive.MaxOutputTokens
	}
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}
}

func writeDefault(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	if err := enc.Encode(cfg); err != nil {
		return err
	}
	home.SecureFile(path)
	return nil
}

func canonicalPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	eval, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = eval
	}
	return filepath.Clean(path)
}
