//go:build linux

// Command authreader is a throwaway Step-3 harness — same spirit as
// testpush/testreconnect. It runs the journald reader standalone and
// prints every parsed AuthEvent, so you can fail some logins and watch
// events appear before this is wired into run.go (Step 4). It never sends
// anything to the server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"khemstrix-agent/internal/modules/auth"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("authreader: watching journald for sshd/sudo failures (Ctrl+C to stop)")
	fmt.Printf("authreader: cursor file: %s\n", auth.DefaultCursorFile)

	err := auth.ReadJournal(ctx, auth.ReaderOptions{
		OnEvent: func(ev auth.AuthEvent) {
			fmt.Printf("EVENT service=%s reason=%s user=%q src_ip=%q src_port=%q time=%s cursor=%s\n",
				ev.Service, ev.Reason, ev.Username, ev.SourceIP, ev.SourcePort,
				ev.Time.Format("15:04:05.000"), ev.RawCursor)
		},
	})
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "authreader: %v\n", err)
		os.Exit(1)
	}
}
