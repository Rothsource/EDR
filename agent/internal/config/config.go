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

// State tracks whether the agent is actually talking to the server right
// now — separate from Config, which only holds registration credentials.
// This is what lets an install script (or a future "status" check) tell
// the difference between "the OS thinks the service is running" and "the
// agent has actually successfully checked in," since those can disagree —
// heartbeat logs are invisible once running as a real background service.
type State struct {
	LastSuccessAt string `json:"last_success_at,omitempty"` // RFC3339, empty if never succeeded
	LastError     string `json:"last_error,omitempty"`      // empty if the last attempt succeeded
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

// statePath lives alongside configPath, same directory, different file —
// config.json holds secrets (api_key), state.json doesn't, so they get
// different file permissions below.
func statePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "khemstrix-agent", "state.json")
	}
	return "/etc/khemstrix-agent/state.json"
}

// Dir returns the directory that holds config.json and state.json, so other
// packages (e.g. the SQLite durable outbox) can put their own files
// alongside them without duplicating the OS-specific path logic that lives
// in configPath/statePath.
func Dir() string {
	return filepath.Dir(configPath())
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

// SaveState is called after every registration/heartbeat attempt (success
// or failure) so an install script or status check can verify real
// connectivity, not just "the process is alive."
func SaveState(s State) error {
	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644) // world-readable — no secrets in here, just timestamps
}

// LoadState returns an error if no state has been written yet (e.g. the
// agent hasn't started or hasn't attempted a check-in). Callers should
// treat that as "no data yet," not as a fatal problem.
func LoadState() (*State, error) {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
