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
	Server    ServerConfig              `toml:"server"`
	Paths     PathsConfig               `toml:"paths"`
	Providers map[string]ProviderConfig `toml:"providers"`
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

type ProviderConfig struct {
	Type       string `toml:"type"`
	BaseURL    string `toml:"base_url"`
	AuthIssuer string `toml:"auth_issuer"`
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
	path = home.ExpandPath(path, homeDir)

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
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, "", err
	}
	cfg.applyDefaults(homeDir)
	cfg.Paths.DataDir = home.ExpandPath(cfg.Paths.DataDir, homeDir)
	cfg.Paths.Database = home.ExpandPath(cfg.Paths.Database, homeDir)
	cfg.Paths.LogDir = home.ExpandPath(cfg.Paths.LogDir, homeDir)
	cfg.Paths.CacheDir = home.ExpandPath(cfg.Paths.CacheDir, homeDir)
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
