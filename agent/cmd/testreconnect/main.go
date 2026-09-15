// cmd/testreconnect: pushes an event, then waits much longer, giving you
// time to manually kill/restart the server mid-flight to prove the event
// survives a disconnect and gets resent via reconciliation. Delete once
// no longer needed.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"khemstrix-agent/internal/config"
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

	eventID := uuid.NewString()
	payload := fmt.Sprintf(`{"type":"event","event_id":"%s","time":"%s","class_uid":1001,"category_uid":1,"activity_id":1,"type_uid":100101,"severity_id":1,"hostname":"reconnect-test","username":"test-user","metadata":{},"data":{"note":"reconnect test"}}`,
		eventID, time.Now().UTC().Format(time.RFC3339))

	log.Printf("pushing event %s — NOW GO KILL THE SERVER (Ctrl+C uvicorn)", eventID)
	if err := client.Push(eventID, []byte(payload)); err != nil {
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