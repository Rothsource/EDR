// cmd/testreconnect: pushes an event, then waits much longer, giving you
// time to manually kill/restart the server mid-flight to prove the event
// survives a disconnect and gets resent via reconciliation. Delete once
// no longer needed.
package main

import (
	"context"
	"log"
	"strings"
	"time"

	"khemstrix-agent/internal/config"
	"khemstrix-agent/internal/event"
	"khemstrix-agent/internal/store"
	"khemstrix-agent/internal/wsclient"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("no config found: %v", err)
	}

	st, err := store.Open()
	if err != nil {
		log.Fatalf("opening outbox: %v", err)
	}
	defer st.Close()

	client, err := wsclient.New(toWSURL(cfg.Server), cfg.AgentID, cfg.APIKey, st)
	if err != nil {
		log.Fatalf("creating wsclient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	time.Sleep(2 * time.Second)

	// Hostname is overridden so the SQL in the test guide
	// (WHERE hostname = 'reconnect-test') still matches.
	eventID, payload, err := event.Build(event.Params{
		ClassUID:    1001,
		CategoryUID: 1,
		ActivityID:  1,
		SeverityID:  1,
		Hostname:    "reconnect-test",
		Username:    "test-user",
		Data:        map[string]any{"note": "reconnect test"},
	})
	if err != nil {
		log.Fatalf("building event: %v", err)
	}

	log.Printf("pushing event %s — NOW GO KILL THE SERVER (Ctrl+C uvicorn)", eventID)
	if err := client.Push(eventID, payload); err != nil {
		log.Fatalf("Push failed: %v", err)
	}

	log.Println("running for 120s — kill the server now, wait, then restart it, and watch this log")
	time.Sleep(120 * time.Second)
	log.Println("done. check outbox and Postgres now.")
}

func toWSURL(httpURL string) string {
	u := strings.Replace(httpURL, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	return strings.TrimRight(u, "/") + "/agent/ws"
}
