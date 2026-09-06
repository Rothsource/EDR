package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"khemstrix-agent/internal/config"
	"khemstrix-agent/internal/core"
)

const heartbeatInterval = 30 * time.Second

func main() {
	flags := config.ParseFlags()

	cfg, err := config.Load()
	if err != nil {
		// No local config — this is a first-time install.
		if flags.Server == "" || flags.Token == "" {
			log.Fatal("first-time setup requires --server and --token")
		}
		cfg = registerAgent(flags)
	}

	runHeartbeatLoop(cfg)
}

func registerAgent(flags config.Flags) *config.Config {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("failed to read hostname: %v", err)
	}
	netInfo := core.GetPrimaryInterface()

	resp, err := core.Register(flags.Server, core.RegisterRequest{
		Hostname:        hostname,
		OS:              runtime.GOOS,
		EnrollmentToken: flags.Token,
		IPAddress:       netInfo.IPAddress,
		MACAddress:      netInfo.MACAddress,
	})
	if err != nil {
		// Registration failure = loud, fatal, exit non-zero — a human is watching this run.
		log.Fatalf("registration failed: %v", err)
	}

	cfg := &config.Config{AgentID: resp.AgentID, APIKey: resp.APIKey, Server: flags.Server}
	if err := config.Save(cfg); err != nil {
		log.Fatalf("registered, but failed to save local config: %v", err)
	}

	fmt.Printf("Registered successfully — agent_id: %s\n", cfg.AgentID)
	return cfg
}

func runHeartbeatLoop(cfg *config.Config) {
	fmt.Println("Starting heartbeat loop...")
	for {
		if err := core.Heartbeat(cfg.Server, core.HeartbeatRequest{AgentID: cfg.AgentID, APIKey: cfg.APIKey}); err != nil {
			// Heartbeat failure = quiet, log, retry next cycle, never crash the loop.
			log.Printf("heartbeat failed: %v", err)
		} else {
			log.Println("heartbeat ok")
		}
		time.Sleep(heartbeatInterval)
	}
}
