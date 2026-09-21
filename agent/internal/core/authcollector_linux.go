//go:build linux

package core

import (
	"context"
	"log"
	"time"

	"khemstrix-agent/internal/modules/auth"
	"khemstrix-agent/internal/wsclient"
)

const authCollectorBackoff = 5 * time.Second

func StartAuthCollector(ctx context.Context, agentID string, wsc *wsclient.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		runAuthCollectorOnce(ctx, agentID, wsc)

		select {
		case <-ctx.Done():
			return
		case <-time.After(authCollectorBackoff):
			log.Printf("authcollector: restarting after backoff")
		}
	}
}

func runAuthCollectorOnce(ctx context.Context, agentID string, wsc *wsclient.Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("authcollector: recovered from panic: %v", r)
		}
	}()

	err := auth.ReadJournal(ctx, auth.ReaderOptions{
		OnEvent: func(ev auth.AuthEvent) {
			eventID, payload, buildErr := ev.Build(agentID)
			if buildErr != nil {
				log.Printf("authcollector: failed to build event, dropping: %v", buildErr)
				return
			}
			if pushErr := wsc.Push(eventID, payload); pushErr != nil {
				log.Printf("authcollector: failed to push event %s: %v", eventID, pushErr)
			}
		},
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("authcollector: ReadJournal stopped: %v", err)
	}
}
