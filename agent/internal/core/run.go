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

func Run(ctx context.Context, flags config.Flags) error {
	cfg, err := config.Load()
	if err != nil {
		if flags.Server == "" || flags.Token == "" {
			return fmt.Errorf("no local config found, and --server/--token were not provided — run once manually with both flags to register")
		}

		hostname, _ := os.Hostname()
		netInfo := GetPrimaryInterface()

		resp, err := Register(flags.Server, RegisterRequest{
			Hostname:        hostname,
			OS:              runtime.GOOS,
			EnrollmentToken: flags.Token,
			IPAddress:       netInfo.IPAddress,
			MACAddress:      netInfo.MACAddress,
		})
		if err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}

		cfg = &config.Config{
			AgentID: resp.AgentID,
			APIKey:  resp.APIKey,
			Server:  flags.Server,
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("registered, but failed to save local config: %w", err)
		}
		log.Printf("registered successfully — agent_id=%s", cfg.AgentID)

		config.SaveState(config.State{LastSuccessAt: time.Now().UTC().Format(time.RFC3339)})
	}

	log.Println("starting heartbeat loop")
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("heartbeat loop stopping (shutdown requested)")
			return nil

		case <-ticker.C:
			err := Heartbeat(cfg.Server, HeartbeatRequest{
				AgentID: cfg.AgentID,
				APIKey:  cfg.APIKey,
			})
			if err != nil {
				log.Printf("heartbeat failed (will retry next cycle): %v", err)
				prev, _ := config.LoadState()
				lastSuccess := ""
				if prev != nil {
					lastSuccess = prev.LastSuccessAt
				}
				config.SaveState(config.State{
					LastSuccessAt: lastSuccess,
					LastError:     err.Error(),
				})
			} else {
				config.SaveState(config.State{
					LastSuccessAt: time.Now().UTC().Format(time.RFC3339),
				})
			}
		}
	}
}
