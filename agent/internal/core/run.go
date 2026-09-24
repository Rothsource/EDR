// internal/core/run.go
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
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

// readLocalConfigVersion reads the "version" field already saved in the
// module config file, so the agent doesn't fetch a config it already has
// on first heartbeat after a restart. Missing/unreadable/malformed file
// all safely resolve to 0, which just means "fetch on next mismatch."
func readLocalConfigVersion(path string) int {
	if path == "" {
		return 0
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var v struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(b, &v) != nil {
		return 0
	}
	return v.Version
}

// atomicWriteFile matches the write-tmp/Sync/rename pattern already used
// by saveCursor (Linux) and saveBookmarkXML (Windows) elsewhere in the
// agent, so a config push can never leave a half-written file on disk.
func atomicWriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// applyConfigUpdate fetches the resolved config for this agent, writes it
// atomically, and signals the auth collector to reload — without ever
// restarting the agent process itself (Section 7B.1).
func applyConfigUpdate(cfg *config.Config, newVersion int, localVersion *int, handle *AuthCollectorHandle) {
	body, err := FetchConfig(cfg.Server, cfg.AgentID, cfg.APIKey)
	if err != nil {
		log.Printf("config: fetch failed, will retry next heartbeat: %v", err)
		return
	}
	if err := atomicWriteFile(AuthConfigPath, body); err != nil {
		log.Printf("config: write failed, will retry next heartbeat: %v", err)
		return
	}
	old := *localVersion
	*localVersion = newVersion
	log.Printf("config: applied v%d -> v%d", old, newVersion)
	if handle != nil {
		handle.Reload()
	}
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
	//
	// authHandle stays nil unless the auth collector actually starts; the
	// heartbeat loop below checks for nil before calling Reload() on it.
	var authHandle *AuthCollectorHandle

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
				authHandle = StartAuthCollector(ctx, cfg.AgentID, wsc)
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

	// localConfigVersion tracks the module config version this agent last
	// applied. Seeded from whatever's already on disk so a restart doesn't
	// re-fetch a config it already has.
	localConfigVersion := readLocalConfigVersion(AuthConfigPath)

	sendHeartbeat := func() {
		netInfo := GetPrimaryInterface()
		resp, err := Heartbeat(cfg.Server, HeartbeatRequest{
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
			return
		}

		_ = config.SaveState(config.State{
			LastSuccessAt: time.Now().UTC().Format(time.RFC3339),
		})

		// Section 7B.1 step 1: compare the version the server just
		// returned against what we last applied. AuthConfigPath is ""
		// on platforms without a collector yet (authcollector_other.go),
		// which disables this path entirely there.
		if AuthConfigPath != "" && resp.ConfigVersion != localConfigVersion {
			applyConfigUpdate(cfg, resp.ConfigVersion, &localConfigVersion, authHandle)
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
