// Package store implements the agent's durable local outbox: every event is
// written to disk the moment it's generated, before any network send is
// attempted, and is only ever removed once the server has confirmed receipt
// (MarkAcked). This is what makes a dropped WebSocket connection safe — the
// event was already safe the instant Write() returned.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"

	"khemstrix-agent/internal/config"
)

// Status values for the outbox row lifecycle: pending -> sent_unacked ->
// (row deleted on ack). There is no "acked" status stored on disk — acking
// is a deletion, not a state, since a row's only reason to exist is "the
// server doesn't have this yet."
const (
	StatusPending     = "pending"
	StatusSentUnacked = "sent_unacked"
)

// Event is a single outbox row, as returned by Pending() for resending
// after a reconnect.
type Event struct {
	EventID string
	Payload string // raw JSON, as generated — store never interprets it
}

type Store struct {
	db *sql.DB
}

// Open creates outbox.db next to config.json/state.json (via config.Dir()),
// enables WAL mode so a burst of writes doesn't block concurrent reads, and
// ensures the schema exists.
func Open() (*Store, error) {
	path := filepath.Join(config.Dir(), "outbox.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("cannot open outbox db: %w", err)
	}

	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot enable WAL mode: %w", err)
	}

	const schema = `
	CREATE TABLE IF NOT EXISTS outbox (
		event_id TEXT PRIMARY KEY,
		payload  TEXT NOT NULL,
		status   TEXT NOT NULL DEFAULT 'pending'
	);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot create outbox schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Write inserts a new event as pending. Idempotent via ON CONFLICT DO
// NOTHING — matches the server's own idempotent insert, so a duplicate
// write here is a harmless no-op rather than an error.
func (s *Store) Write(eventID, payload string) error {
	_, err := s.db.Exec(
		`INSERT INTO outbox (event_id, payload, status) VALUES (?, ?, ?)
		 ON CONFLICT(event_id) DO NOTHING`,
		eventID, payload, StatusPending,
	)
	if err != nil {
		return fmt.Errorf("cannot write event %s to outbox: %w", eventID, err)
	}
	return nil
}

// MarkSent transitions an event from pending to sent_unacked. It is NOT a
// deletion path — the event stays on disk until the server actually acks
// it, so a drop right after send still leaves the event recoverable.
func (s *Store) MarkSent(eventID string) error {
	_, err := s.db.Exec(
		`UPDATE outbox SET status = ? WHERE event_id = ?`,
		StatusSentUnacked, eventID,
	)
	if err != nil {
		return fmt.Errorf("cannot mark event %s sent: %w", eventID, err)
	}
	return nil
}

// MarkAcked is the only deletion path — an event only ever leaves the
// outbox once the server has confirmed it has it.
func (s *Store) MarkAcked(eventID string) error {
	_, err := s.db.Exec(`DELETE FROM outbox WHERE event_id = ?`, eventID)
	if err != nil {
		return fmt.Errorf("cannot mark event %s acked: %w", eventID, err)
	}
	return nil
}

// UnackedIDs returns every event ID currently in the outbox, regardless of
// status, for use in a reconcile request after reconnecting.
func (s *Store) UnackedIDs() ([]string, error) {
	rows, err := s.db.Query(`SELECT event_id FROM outbox`)
	if err != nil {
		return nil, fmt.Errorf("cannot query unacked ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot scan unacked id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating unacked ids: %w", err)
	}
	return ids, nil
}

// Pending returns full event rows, oldest-first, for actually resending
// after a reconnect (as opposed to UnackedIDs, which is just for the
// reconcile round trip).
func (s *Store) Pending() ([]Event, error) {
	rows, err := s.db.Query(`SELECT event_id, payload FROM outbox ORDER BY rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("cannot query pending events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.EventID, &e.Payload); err != nil {
			return nil, fmt.Errorf("cannot scan pending event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pending events: %w", err)
	}
	return events, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
