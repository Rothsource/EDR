// cmd/testpush is a throwaway harness for exercising Push() end-to-end
// before any real detection module exists. It loads the agent's real
// config (must already be registered), opens the real outbox, starts a
// real wsclient.Run(), fires one fabricated event through Push(), and
// logs what happens. Delete this once a real event source exists.
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
		log.Fatalf("no config found — register the agent first: %v", err)
	}
	log.Printf("DEBUG agent_id=%q api_key=%q server=%q", cfg.AgentID, cfg.APIKey, cfg.Server)

	st, err := store.Open()
	if err != nil {
		log.Fatalf("opening outbox: %v", err)
	}
	defer st.Close()

	wsURL := toWSURL(cfg.Server)
	client, err := wsclient.New(wsURL, cfg.AgentID, cfg.APIKey, st)
	if err != nil {
		log.Fatalf("creating wsclient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	// Give it a moment to connect + authenticate before pushing.
	time.Sleep(2 * time.Second)

	eventID := uuid.NewString()
	payload := fmt.Sprintf(`{
		"type":"event",
		"event_id":"%s",
		"time":"%s",
		"class_uid":1001,
		"category_uid":1,
		"activity_id":1,
		"type_uid":100101,
		"severity_id":1,
		"hostname":"testpush-harness",
		"username":"test-user",
		"metadata":{},
		"data":{"note":"testpush harness"}
	}`, eventID, time.Now().UTC().Format(time.RFC3339))

	log.Printf("pushing test event %s", eventID)
	if err := client.Push(eventID, []byte(payload)); err != nil {
		log.Fatalf("Push failed: %v", err)
	}

	log.Println("pushed — watching for ack/reconcile activity for 15s, check server logs and Postgres too")
	time.Sleep(15 * time.Second)
}

func toWSURL(httpURL string) string {
	u := strings.Replace(httpURL, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	return strings.TrimRight(u, "/") + "/agent/ws"
}
