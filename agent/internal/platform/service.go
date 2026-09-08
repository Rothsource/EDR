// internal/platform/service.go
//
// Wraps the agent's run loop (internal/core.Run) so it behaves as a proper
// background service on both Windows and Linux, via kardianos/service —
// one library, one code path, instead of a separate hand-rolled Windows
// Service Control Manager integration plus a systemd-only approach for Linux.
package platform

import (
	"context"
	"log"
	"time"

	"github.com/kardianos/service"
)

// RunFunc is whatever the actual agent work is. main.go supplies this —
// platform doesn't know or care what it does, only how to start/stop it.
type RunFunc func(ctx context.Context) error

type program struct {
	run    RunFunc
	cancel context.CancelFunc
	done   chan struct{}
}

func NewProgram(run RunFunc) *program {
	return &program{run: run}
}

// Start is called by the OS service manager (or directly, in foreground
// mode). Must return quickly — the real work happens in a goroutine.
func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})

	go func() {
		defer close(p.done)
		if err := p.run(ctx); err != nil {
			log.Printf("agent exited with error: %v", err)
		}
	}()

	return nil
}

// Stop cancels the shared context so Run() sees ctx.Done() between
// heartbeats and returns cleanly, then waits (with a timeout) for that
// to actually happen before reporting stopped.
func (p *program) Stop(s service.Service) error {
	log.Println("stop requested — shutting down")

	if p.cancel != nil {
		p.cancel()
	}

	select {
	case <-p.done:
		log.Println("agent stopped cleanly")
	case <-time.After(10 * time.Second):
		log.Println("shutdown timed out after 10s — exiting anyway")
	}

	return nil
}

func Config() *service.Config {
	return &service.Config{
		Name:        "khemstrix-agent",
		DisplayName: "Khemstrix EDR Agent",
		Description: "Endpoint monitoring agent — sends heartbeats and events to the EDR server.",
	}
}
