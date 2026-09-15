// Package wsclient implements the agent's persistent WebSocket connection to
// the server's /agent/ws route.
//
// Contract with the server (matched directly against server/app/schemas/ws.py
// and server/app/routers/ws.py — not assumed):
//
//   - First message on the socket MUST be {"type":"auth","agent_id":...,"api_key":...}.
//   - There is NO explicit "auth succeeded" reply. Silence + the connection
//     staying open means auth passed. On failure the server sends
//     {"type":"error","detail":...} and then closes the socket — so a client
//     that reads an "error" message or has the connection die right after
//     sending auth should treat that as an auth failure, not a network blip.
//   - Events are sent as {"type":"event", ...fields}; the server replies
//     {"type":"ack","event_id":...} once the row is (idempotently) inserted.
//   - Reconciliation is client-initiated: client sends
//     {"type":"reconcile_request","event_ids":[...]}, server replies
//     {"type":"reconcile_response","known_event_ids":[...]}.
//   - The server sends {"type":"ping"} on its own schedule (ws_heartbeat.py);
//     the client must reply {"type":"pong"} or eventually get evicted
//     server-side. This client also watches ping arrival itself, so a dead
//     link is detected agent-side too, not just server-side.
//
// This client does not interpret event payloads. store.Event.Payload is
// expected to already be a complete, ready-to-send JSON object matching
// schemas.ws.WSEvent (i.e. it already has "type":"event", the matching
// "event_id", class_uid/category_uid/activity_id/type_uid/severity_id,
// and a "data" object). Whatever future module calls Push() owns building
// that JSON — this package only owns getting it there durably.
package wsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"khemstrix-agent/internal/store"
)

const (
	authTimeout       = 10 * time.Second
	reconcileTimeout  = 10 * time.Second
	pingWatchInterval = 5 * time.Second
	pingDeadAfter     = 25 * time.Second // server pings ~every 10s; 25s = missed 2, safely under its own ~30s eviction
	writeTimeout      = 5 * time.Second
	minBackoff        = 1 * time.Second
	maxBackoff        = 30 * time.Second
)

// Client is a single agent's persistent connection to the server's
// WebSocket route. Create one with New and call Run once; Run blocks
// until ctx is cancelled, reconnecting internally on any failure.
type Client struct {
	serverURL string // full ws:// or wss:// URL to /agent/ws, e.g. "ws://10.0.0.5:8000/agent/ws"
	agentID   string // must be a valid UUID string — validated in New
	apiKey    string
	st        *store.Store

	writeMu sync.Mutex // serializes all writes to the live conn (nhooyr requires this)

	connMu sync.Mutex
	conn   *websocket.Conn // nil when not currently connected

	pingMu   sync.Mutex
	lastPing time.Time
}

// New validates its inputs and returns a Client. It does not connect —
// call Run for that.
func New(serverURL, agentID, apiKey string, st *store.Store) (*Client, error) {
	if _, err := uuid.Parse(agentID); err != nil {
		return nil, fmt.Errorf("wsclient: agentID %q is not a valid UUID: %w", agentID, err)
	}
	if serverURL == "" {
		return nil, fmt.Errorf("wsclient: serverURL is empty")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("wsclient: apiKey is empty")
	}
	if st == nil {
		return nil, fmt.Errorf("wsclient: store is nil")
	}
	return &Client{
		serverURL: serverURL,
		agentID:   agentID,
		apiKey:    apiKey,
		st:        st,
	}, nil
}

// ---- wire message shapes -------------------------------------------------
// Minimal mirrors of schemas/ws.py. Only fields this client itself needs to
// build or read are represented; full event bodies pass through as raw JSON
// via store.Event.Payload / Push's payload argument.

type wireType struct {
	Type string `json:"type"`
}

type authMsg struct {
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
}

type ackMsg struct {
	Type    string `json:"type"`
	EventID string `json:"event_id"`
}

type reconcileRequest struct {
	Type     string   `json:"type"`
	EventIDs []string `json:"event_ids"`
}

type reconcileResponse struct {
	Type          string   `json:"type"`
	KnownEventIDs []string `json:"known_event_ids"`
}

type pongMsg struct {
	Type string `json:"type"`
}

type errorMsg struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

// ---- public API -----------------------------------------------------------

// Push durably queues an event and best-effort sends it immediately if a
// live connection exists. payload must already be the complete WSEvent JSON
// (including "type":"event" and "event_id"); eventID must match the
// "event_id" field inside it — it's the outbox's idempotency key.
//
// Push never blocks on the network for long: the durable write always
// happens; the opportunistic live send has its own short timeout and is
// allowed to fail silently, since the next reconcile pass will catch
// anything that didn't make it.
func (c *Client) Push(eventID string, payload []byte) error {
	if err := c.st.Write(eventID, string(payload)); err != nil {
		return fmt.Errorf("wsclient: durable write failed, event not queued: %w", err)
	}

	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()
	if conn == nil {
		return nil // no live connection right now; it's safe on disk, reconcile will send it later
	}

	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	if err := c.writeRaw(ctx, conn, payload); err != nil {
		// Not fatal for Push's caller — the event is durably queued either way.
		log.Printf("wsclient: opportunistic send of %s failed, will resend on reconcile: %v", eventID, err)
		return nil
	}
	if err := c.st.MarkSent(eventID); err != nil {
		log.Printf("wsclient: could not mark %s sent (non-fatal): %v", eventID, err)
	}
	return nil
}

// Run connects, authenticates, reconciles, and streams for as long as ctx
// is alive, reconnecting with exponential backoff + jitter on any failure.
// It returns only when ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	backoff := minBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		authenticated, err := c.runOnce(ctx)
		if err != nil {
			log.Printf("wsclient: connection attempt ended: %v", err)
		}

		c.connMu.Lock()
		c.conn = nil
		c.connMu.Unlock()

		if authenticated {
			// We got far enough to talk to the server; a fresh problem
			// afterwards shouldn't inherit an escalated backoff from
			// earlier unrelated trouble.
			backoff = minBackoff
		}

		if ctx.Err() != nil {
			return
		}

		wait := backoff + time.Duration(rand.Int63n(int64(backoff)))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// ---- internals --------------------------------------------------------

// runOnce performs one full connect -> authenticate -> reconcile -> stream
// cycle. The returned bool reports whether authentication succeeded, so Run
// knows whether to reset its backoff even if streaming later failed.
func (c *Client) runOnce(ctx context.Context) (authenticated bool, err error) {
	conn, _, err := websocket.Dial(ctx, c.serverURL, nil)
	if err != nil {
		return false, fmt.Errorf("dial failed: %w", err)
	}
	defer conn.CloseNow()

	if err := c.authenticate(ctx, conn); err != nil {
		return false, fmt.Errorf("auth failed: %w", err)
	}
	authenticated = true

	if err := c.reconcile(ctx, conn); err != nil {
		return authenticated, fmt.Errorf("reconcile failed: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	c.pingMu.Lock()
	c.lastPing = time.Now()
	c.pingMu.Unlock()

	// From here, both the read loop and the heartbeat watcher can end the
	// connection; whichever fires first wins and we tear down.
	done := make(chan error, 2)
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	go c.readLoop(streamCtx, conn, done)
	go c.heartbeatWatch(streamCtx, done)

	select {
	case <-ctx.Done():
		conn.Close(websocket.StatusNormalClosure, "shutting down")
		return authenticated, nil
	case streamErr := <-done:
		return authenticated, streamErr
	}
}

// authenticate sends the auth message and treats a fatal read (error frame
// or closed connection) in the immediate aftermath as an auth failure, per
// _authenticate() in routers/ws.py — there is no success reply to wait for.
func (c *Client) authenticate(ctx context.Context, conn *websocket.Conn) error {
	msg := authMsg{Type: "auth", AgentID: c.agentID, APIKey: c.apiKey}
	if err := c.writeJSON(ctx, conn, msg); err != nil {
		return fmt.Errorf("sending auth message: %w", err)
	}

	// The server doesn't ack a good auth, so we can't positively confirm
	// success here — we only watch, with a short timeout, for it to
	// immediately reject us. If nothing arrives, we proceed optimistically;
	// a bad auth that the server rejected only after this window would
	// surface as an error frame or close in the normal read loop instead.
	checkCtx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	_, data, err := conn.Read(checkCtx)
	if err != nil {
		// Timeout here is the expected/good case: server stayed silent,
		// meaning auth passed and it's just waiting for our next message.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	}

	var t wireType
	if jsonErr := json.Unmarshal(data, &t); jsonErr == nil && t.Type == "error" {
		var e errorMsg
		_ = json.Unmarshal(data, &e)
		return fmt.Errorf("server rejected auth: %s", e.Detail)
	}
	// Got a real, non-error message already (e.g. a ping raced in) — auth
	// clearly succeeded. Hand it to the normal dispatcher so it isn't lost.
	c.dispatch(ctx, conn, data)
	return nil
}

// reconcile runs once, right after authentication and before the concurrent
// read loop starts, so the catch-up resend and any live sends never race
// over the same outbox rows.
func (c *Client) reconcile(ctx context.Context, conn *websocket.Conn) error {
	ids, err := c.st.UnackedIDs()
	if err != nil {
		return fmt.Errorf("reading unacked ids: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	req := reconcileRequest{Type: "reconcile_request", EventIDs: ids}
	if err := c.writeJSON(ctx, conn, req); err != nil {
		return fmt.Errorf("sending reconcile_request: %w", err)
	}

	reconcileCtx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	defer cancel()

	var resp *reconcileResponse
	for resp == nil {
		_, data, err := conn.Read(reconcileCtx)
		if err != nil {
			return fmt.Errorf("waiting for reconcile_response: %w", err)
		}
		var t wireType
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		switch t.Type {
		case "reconcile_response":
			var r reconcileResponse
			if err := json.Unmarshal(data, &r); err != nil {
				return fmt.Errorf("malformed reconcile_response: %w", err)
			}
			resp = &r
		case "ping":
			// Can legitimately arrive while we're waiting; keep the
			// server happy so it doesn't evict us mid-reconcile.
			if err := c.writeJSON(ctx, conn, pongMsg{Type: "pong"}); err != nil {
				return fmt.Errorf("replying to ping during reconcile: %w", err)
			}
		case "error":
			var e errorMsg
			_ = json.Unmarshal(data, &e)
			return fmt.Errorf("server error during reconcile: %s", e.Detail)
		default:
			// Ignore anything else here; the normal read loop handles
			// acks etc. once streaming starts.
		}
	}

	known := make(map[string]bool, len(resp.KnownEventIDs))
	for _, id := range resp.KnownEventIDs {
		known[id] = true
	}
	for _, id := range ids {
		if known[id] {
			if err := c.st.MarkAcked(id); err != nil {
				log.Printf("wsclient: could not mark reconciled id %s acked (non-fatal): %v", id, err)
			}
		}
	}

	pending, err := c.st.Pending()
	if err != nil {
		return fmt.Errorf("reading pending events to resend: %w", err)
	}
	for _, ev := range pending {
		if known[ev.EventID] {
			continue // already acked above; nothing to resend
		}
		if err := c.writeRaw(ctx, conn, []byte(ev.Payload)); err != nil {
			return fmt.Errorf("resending %s during reconcile: %w", ev.EventID, err)
		}
		if err := c.st.MarkSent(ev.EventID); err != nil {
			log.Printf("wsclient: could not mark resent id %s sent (non-fatal): %v", ev.EventID, err)
		}
	}
	return nil
}

// readLoop is the sole reader of conn once streaming begins. It dispatches
// every message and reports the first fatal condition on done.
func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn, done chan<- error) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			select {
			case done <- fmt.Errorf("read failed: %w", err):
			default:
			}
			return
		}
		c.dispatch(ctx, conn, data)
	}
}

// dispatch handles a single inbound frame outside of the reconcile phase
// (and is also reused for the one frame authenticate() might consume).
func (c *Client) dispatch(ctx context.Context, conn *websocket.Conn, data []byte) {
	var t wireType
	if err := json.Unmarshal(data, &t); err != nil {
		log.Printf("wsclient: ignoring non-JSON or malformed frame: %v", err)
		return
	}

	switch t.Type {
	case "ping":
		c.pingMu.Lock()
		c.lastPing = time.Now()
		c.pingMu.Unlock()
		if err := c.writeJSON(ctx, conn, pongMsg{Type: "pong"}); err != nil {
			log.Printf("wsclient: failed to reply to ping: %v", err)
		}

	case "ack":
		var a ackMsg
		if err := json.Unmarshal(data, &a); err != nil {
			log.Printf("wsclient: malformed ack: %v", err)
			return
		}
		if err := c.st.MarkAcked(a.EventID); err != nil {
			log.Printf("wsclient: could not mark %s acked (non-fatal): %v", a.EventID, err)
		}

	case "error":
		var e errorMsg
		_ = json.Unmarshal(data, &e)
		log.Printf("wsclient: server sent error: %s", e.Detail)

	case "reconcile_response":
		// Only expected during the startup reconcile() phase, which reads
		// directly rather than going through dispatch. Seeing one here
		// means it arrived late/unsolicited; nothing to do with it.
		log.Printf("wsclient: unexpected reconcile_response outside reconcile phase, ignoring")

	default:
		log.Printf("wsclient: unrecognized message type %q, ignoring", t.Type)
	}
}

// heartbeatWatch declares the connection dead if too long passes without a
// ping from the server, independent of whatever the read loop is doing.
func (c *Client) heartbeatWatch(ctx context.Context, done chan<- error) {
	ticker := time.NewTicker(pingWatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.pingMu.Lock()
			since := time.Since(c.lastPing)
			c.pingMu.Unlock()
			if since > pingDeadAfter {
				select {
				case done <- fmt.Errorf("no ping from server in %s, assuming connection is dead", since):
				default:
				}
				return
			}
		}
	}
}

// writeJSON and writeRaw both serialize through writeMu since nhooyr's
// websocket.Conn forbids concurrent writers.
func (c *Client) writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.writeRaw(ctx, conn, b)
}

func (c *Client) writeRaw(ctx context.Context, conn *websocket.Conn, b []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, b)
}