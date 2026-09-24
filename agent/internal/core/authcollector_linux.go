//go:build linux

package core

import (
	"context"
	"log"
	"sync"
	"time"

	"khemstrix-agent/internal/modules/auth"
	"khemstrix-agent/internal/wsclient"
)

const authCollectorBackoff = 5 * time.Second

var AuthConfigPath = auth.DefaultAuthConfigFile

type AuthCollectorHandle struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// Reload kills the current journalctl subprocess by cancelling its run
// context; the loop below immediately relaunches and re-reads config
// fresh, resuming from the last saved cursor. No agent restart.
func (h *AuthCollectorHandle) Reload() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		log.Printf("authcollector: reload requested, restarting journalctl with new config")
		h.cancel()
	}
}

func StartAuthCollector(ctx context.Context, agentID string, wsc *wsclient.Client) *AuthCollectorHandle {
	handle := &AuthCollectorHandle{}

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
				continue // reload, not a failure — no backoff
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

// returns true if runCtx was cancelled by Reload() rather than a real error.
func runAuthCollectorOnce(runCtx context.Context, agentID string, wsc *wsclient.Client) (reloaded bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("authcollector: recovered from panic: %v", r)
		}
	}()

	err := auth.ReadJournal(runCtx, auth.ReaderOptions{
		AgentID: agentID,
		Push:    wsc.Push,
	})
	if runCtx.Err() != nil {
		return true
	}
	if err != nil {
		log.Printf("authcollector: ReadJournal stopped: %v", err)
	}
	return false
}
