// internal/core/run.go
package core

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"runtime"
	"time"

	"khemstrix-agent/internal/config"
	"khemstrix-agent/internal/store"
	"khemstrix-agent/internal/wsclient"
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

// buildWSURL turns the REST server URL already used for Register/Heartbeat
// (e.g. "http://192.168.1.10:8000") into the ws(s):// URL for the
// persistent /agent/ws route on that same host.
func buildWSURL(server string) (string, error) {
	u, err := url.Parse(server)
	if err != nil {
		return "", fmt.Errorf("parsing server URL %q: %w", server, err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already correct, leave as-is
	default:
		return "", fmt.Errorf("server URL %q has unrecognized scheme %q", server, u.Scheme)
	}
	u.Path = "/agent/ws"
	return u.String(), nil
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
		// A saved config.json always wins over a --server flag here. The
		// background service is normally launched with no CLI arguments at
		// all (see main.go), so if flags.Server is non-empty and disagrees
		// with the saved config, that's almost certainly an accidental or
		// stale flag from manual/debug invocation — not an intentional
		// re-point of a running agent. Silently honoring it would let a
		// single mistyped flag redirect a production agent's event stream
		// to the wrong server. We only log the mismatch and keep cfg.Server
		// as-is; anyone who genuinely wants to re-point an agent should
		// edit config.json directly (or re-run registration).
		if flags.Server != "" && flags.Server != cfg.Server {
			log.Printf("ignoring --server=%s: saved config.json already targets %s and takes precedence", flags.Server, cfg.Server)
		}
	}

	log.Printf("starting heartbeat loop for agent_id=%s targeting %s", cfg.AgentID, cfg.Server)

	// --- WebSocket streaming path (durable outbox + persistent connection) ---
	// This runs alongside the existing heartbeat loop below, not instead of
	// it — heartbeat still owns liveness/IP-MAC refresh over REST; the
	// WS client owns event delivery. A failure here is logged, not fatal:
	// the agent should keep doing heartbeats even if streaming can't start.
	st, err := store.Open()
	if err != nil {
		log.Printf("wsclient: could not open durable outbox, event streaming disabled this run: %v", err)
	} else {
		wsURL, err := buildWSURL(cfg.Server)
		if err != nil {
			log.Printf("wsclient: could not build WebSocket URL, event streaming disabled this run: %v", err)
			_ = st.Close()
		} else {
			wsc, err := wsclient.New(wsURL, cfg.AgentID, cfg.APIKey, st)
			if err != nil {
				log.Printf("wsclient: could not construct client, event streaming disabled this run: %v", err)
				_ = st.Close()
			} else {
				log.Printf("wsclient: starting, targeting %s", wsURL)
				go wsc.Run(ctx)
				go func() {
					<-ctx.Done()
					_ = st.Close()
				}()
				// Note: st is intentionally not closed here — it's owned by
				// wsc for the remaining lifetime of this run, and needs to
				// stay open until ctx is cancelled and wsc.Run returns.
			}
		}
	}

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
