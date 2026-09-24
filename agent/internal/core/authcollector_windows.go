//go:build windows

package core

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"khemstrix-agent/internal/modules/auth"
	"khemstrix-agent/internal/wsclient"
)

const authCollectorBackoff = 5 * time.Second

// programDataDir returns the real, expanded agent data folder.
// window.go's `%ProgramData%\...` constants are literal strings that Go
// does not expand, so this file never relies on them.
func programDataDir() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "khemstrix-agent")
}

// AuthConfigPath is where applyConfigUpdate (run.go) writes the pushed
// config and where ReadSecurityLog reads it back.
var AuthConfigPath = filepath.Join(programDataDir(), "config", "auth.json")

var authBookmarkPath = filepath.Join(programDataDir(), "auth_bookmark.xml")

type AuthCollectorHandle struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// Reload cancels the current Security-log subscription. The loop in
// StartAuthCollector then resubscribes, re-reading auth.json and resuming
// from the saved bookmark. The agent process itself never restarts.
func (h *AuthCollectorHandle) Reload() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		log.Printf("authcollector: reload requested, resubscribing to Security log with new config")
		h.cancel()
	}
}

func StartAuthCollector(ctx context.Context, agentID string, wsc *wsclient.Client) *AuthCollectorHandle {
	handle := &AuthCollectorHandle{}

	// Make sure the config folder exists so the first config write succeeds.
	if err := os.MkdirAll(filepath.Dir(AuthConfigPath), 0o755); err != nil {
		log.Printf("authcollector: cannot create config dir: %v", err)
	}

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}

			runCtx, cancel := context.WithCancel(ctx)
			handle.mu.Lock()
			handle.cancel = cancel
			handle.mu.Unlock()

			reloaded := runAuthCollectorOnce(runCtx, agentID, wsc)
			cancel()

			if ctx.Err() != nil {
				return
			}
			if reloaded {
				continue // reload, not a failure: no backoff
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(authCollectorBackoff):
				log.Printf("authcollector: restarting after backoff")
			}
		}
	}()

	return handle
}

// Returns true if runCtx was cancelled by Reload() rather than a real error.
func runAuthCollectorOnce(runCtx context.Context, agentID string, wsc *wsclient.Client) (reloaded bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("authcollector: recovered from panic: %v", r)
		}
	}()

	err := auth.ReadSecurityLog(runCtx, auth.WindowsReaderOptions{
		AgentID:      agentID,
		Push:         wsc.Push,
		BookmarkFile: authBookmarkPath,
		ConfigFile:   AuthConfigPath,
	})
	if runCtx.Err() != nil {
		return true
	}
	if err != nil {
		log.Printf("authcollector: ReadSecurityLog stopped: %v", err)
	}
	return false
}

// A Windows service has no console, so log/stderr output is otherwise lost.
// Send both to a file we can read.
func init() {
	dir := programDataDir()
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "agent.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	os.Stderr = f
	log.SetOutput(f)
}
