package config

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
)

// Config is what gets saved to disk after a successful registration.
type Config struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
	Server  string `json:"server"`
}

// Flags are the CLI arguments passed in on first-run.
type Flags struct {
	Server string
	Token  string
}

func ParseFlags() Flags {
	server := flag.String("server", "", "EDR server URL, e.g. http://192.168.1.10:8000")
	token := flag.String("token", "", "One-time enrollment token")
	flag.Parse()
	return Flags{Server: *server, Token: *token}
}

// configPath returns the OS-appropriate location for the persisted config —
// matches the C:\ProgramData\khemstrix-agent\config.json plan from the design doc.
func configPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "khemstrix-agent", "config.json")
	}
	return "/etc/khemstrix-agent/config.json"
}

// Load returns an error if no config exists yet — that error IS the signal
// main.go uses to decide "this is a fresh install, go register."
func Load() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600) // 0600 — api_key lives in this file, keep it locked down
}
