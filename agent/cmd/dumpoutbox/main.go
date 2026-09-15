// cmd/dumpoutbox is a throwaway debug tool: prints every row currently in
// the agent's local outbox.db. Delete once no longer needed.
package main

import (
	"fmt"
	"log"

	"khemstrix-agent/internal/store"
)

func main() {
	st, err := store.Open()
	if err != nil {
		log.Fatalf("opening outbox: %v", err)
	}
	defer st.Close()

	events, err := st.Pending()
	if err != nil {
		log.Fatalf("reading pending events: %v", err)
	}

	if len(events) == 0 {
		fmt.Println("outbox is empty — no unacked events remain")
		return
	}

	fmt.Printf("outbox has %d row(s):\n", len(events))
	for _, e := range events {
		fmt.Printf("  event_id=%s payload=%s\n", e.EventID, e.Payload)
	}
}