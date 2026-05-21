package state

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

type Config struct {
	Telemetry    TelemetryConfig    `toml:"telemetry"`
	Marketplaces MarketplacesConfig `toml:"marketplaces"`
}

type TelemetryConfig struct {
	Enabled bool `toml:"enabled"`
}

type MarketplacesConfig struct {
	Default string `toml:"default"`
}

func LoadConfig(ctx context.Context, home string) (Config, error) {
	path := filepath.Join(home, "config.toml")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return defaultConfig(), nil
		}
		return Config{}, errpkg.Wrap("E_CONFIG_READ", err, "cannot read "+path)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errpkg.Wrap("E_CONFIG_PARSE", err, "invalid config.toml")
	}

	applyDefaults(&cfg)
	return cfg, nil
}

func defaultConfig() Config {
	c := Config{}
	applyDefaults(&c)
	return c
}

func applyDefaults(c *Config) {
	if c.Marketplaces.Default == "" {
		c.Marketplaces.Default = "official-obey"
	}
}
