// internal/core/run.go
package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"khemstrix-agent/internal/config"
)

const heartbeatInterval = 30 * time.Second

// RegisterAndSaveConfig performs first-time registration against server
// using the given enrollment token, and persists the resulting agent_id/
// api_key to config.json. This is called from main.go, in the foreground,
// before the background service is installed and started — the service
// itself is always launched with no CLI arguments (see the comment on
// svcConfig.Arguments in cmd/agent/main.go), so it can never perform
// first-time registration on its own. config.json must already exist and
// be correct by the time the service process starts.
func RegisterAndSaveConfig(server, token string) (*config.Config, error) {
	hostname, _ := os.Hostname()
	netInfo := GetPrimaryInterface()

	resp, err := Register(server, RegisterRequest{
		Hostname:        hostname,
		OS:              runtime.GOOS,
		EnrollmentToken: token,
		IPAddress:       netInfo.IPAddress,
		MACAddress:      netInfo.MACAddress,
	})
	if err != nil {
		return nil, err
	}

	cfg := &config.Config{
		AgentID: resp.AgentID,
		APIKey:  resp.APIKey,
		Server:  server,
	}
	if err := config.Save(cfg); err != nil {
		return nil, fmt.Errorf("registered, but failed to save local config: %w", err)
	}
	log.Printf("registered successfully — agent_id=%s", cfg.AgentID)

	_ = config.SaveState(config.State{LastSuccessAt: time.Now().UTC().Format(time.RFC3339)})

	return cfg, nil
}

func Run(ctx context.Context, flags config.Flags) error {
	cfg, err := config.Load()

	if err != nil {
		// --- Fresh Registration Path ---
		// In normal operation this is unreachable for the real background
		// service, since config.json should already exist by the time it
		// starts (see main.go's autoInstallAndStart). Kept as a fallback
		// for anyone invoking the binary directly with flags outside the
		// service wrapper.
		if flags.Server == "" || flags.Token == "" {
			return fmt.Errorf("no local config found, and --server/--token were not provided — run once manually with both flags to register")
		}

		cfg, err = RegisterAndSaveConfig(flags.Server, flags.Token)
		if err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}

	} else {
		// --- Existing Config Path ---
		if flags.Server != "" && flags.Server != cfg.Server {
			log.Printf("server address override detected: switching from %s to %s", cfg.Server, flags.Server)
			cfg.Server = flags.Server
			if err := config.Save(cfg); err != nil {
				log.Printf("warning: failed to save updated server URL to config: %v", err)
			}
		}
	}

	log.Printf("starting heartbeat loop for agent_id=%s targeting %s", cfg.AgentID, cfg.Server)

	sendHeartbeat := func() {
		netInfo := GetPrimaryInterface()
		err := Heartbeat(cfg.Server, HeartbeatRequest{
			AgentID:    cfg.AgentID,
			APIKey:     cfg.APIKey,
			IPAddress:  netInfo.IPAddress,
			MACAddress: netInfo.MACAddress,
		})

		if err != nil {
			log.Printf("heartbeat failed (will retry next cycle): %v", err)
			prev, _ := config.LoadState()
			lastSuccess := ""
			if prev != nil {
				lastSuccess = prev.LastSuccessAt
			}
			_ = config.SaveState(config.State{
				LastSuccessAt: lastSuccess,
				LastError:     err.Error(),
			})
		} else {
			_ = config.SaveState(config.State{
				LastSuccessAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
	}

	sendHeartbeat()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("heartbeat loop stopping (shutdown requested)")
			return nil

		case <-ticker.C:
			sendHeartbeat()
		}
	}
}
