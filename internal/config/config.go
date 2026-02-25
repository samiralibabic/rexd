package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Limits   LimitsConfig   `toml:"limits"`
	Security SecurityConfig `toml:"security"`
	Audit    AuditConfig    `toml:"audit"`
}

type ServerConfig struct {
	Stdio      bool   `toml:"stdio"`
	HTTPListen string `toml:"http_listen"`
	HTTPPath   string `toml:"http_path"`
	WSPath     string `toml:"ws_path"`
	LogLevel   string `toml:"log_level"`
}

type LimitsConfig struct {
	DefaultTimeoutMs      int `toml:"default_timeout_ms"`
	HardTimeoutMs         int `toml:"hard_timeout_ms"`
	MaxOutputBytes        int `toml:"max_output_bytes"`
	MaxFileReadBytes      int `toml:"max_file_read_bytes"`
	MaxProcessesPerSess   int `toml:"max_processes_per_session"`
	MaxConcurrentSessions int `toml:"max_concurrent_sessions"`
}

type SecurityConfig struct {
	AllowShell  bool          `toml:"allow_shell"`
	AllowedRoot []AllowedRoot `toml:"allowed_roots"`
}

type AllowedRoot struct {
	Path string `toml:"path"`
}

type AuditConfig struct {
	Enabled bool   `toml:"enabled"`
	Path    string `toml:"path"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Stdio:      true,
			HTTPListen: "",
			HTTPPath:   "/rpc",
			WSPath:     "/ws",
			LogLevel:   "info",
		},
		Limits: LimitsConfig{
			DefaultTimeoutMs:      30000,
			HardTimeoutMs:         300000,
			MaxOutputBytes:        1048576,
			MaxFileReadBytes:      1048576,
			MaxProcessesPerSess:   8,
			MaxConcurrentSessions: 16,
		},
		Security: SecurityConfig{
			AllowShell: true,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, err
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func AllowedRoots(cfg Config) []string {
	roots := make([]string, 0, len(cfg.Security.AllowedRoot))
	for _, r := range cfg.Security.AllowedRoot {
		if r.Path != "" {
			roots = append(roots, r.Path)
		}
	}
	return roots
}
