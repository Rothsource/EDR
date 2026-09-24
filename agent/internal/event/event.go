// Package event builds the complete WSEvent JSON that wsclient.Push() sends.
//
// Every collector (auth, file, network, ...) calls Build instead of
// hand-assembling JSON, so the envelope fields the server stores as columns
// (schema_version, agent_version, host_os, host_os_version) are stamped in
// exactly one place, and event_id / type_uid can never be inconsistent.
//
// Usage from a collector:
//
//	id, payload, err := event.Build(event.Params{
//	    ClassUID: 3002, CategoryUID: 3, ActivityID: 1, SeverityID: 2,
//	    Username: "alice",
//	    Data:     map[string]any{"src_ip": "10.0.0.7", "failure_reason": "bad_password"},
//	})
//	if err != nil { ... }
//	err = wsc.Push(id, payload)
//
// Collectors that re-read a log (journald, Windows Event Log) should also set
// EventID (from DeterministicID) and Time (the log record's own timestamp),
// so a restart never creates duplicate or misdated events.
//
// Fields the server sets itself and this package must NOT send:
// agent_id, tenant_id, ingest_source, created_at.
package event

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SchemaVersion is the envelope shape version. Bump it when the envelope or
// a class's `data` shape changes incompatibly, so the backend can tell old
// and new rows apart.
const SchemaVersion = 1

// AgentVersion identifies this build. It can be overridden at release-build
// time without editing code:
//
//	go build -ldflags "-X khemstrix-agent/internal/event.AgentVersion=0.2.0"
var AgentVersion = "0.1.0"

// Server-side max_length limits (schemas/ws.py). An over-long value makes the
// server reject the event, and a rejected event would stay in the outbox and
// be retried forever, so clip defensively here.
const (
	maxAgentVersion  = 64
	maxHostOSVersion = 128
	maxHostOS        = 32
)

// Params is what a collector supplies. Everything else is filled in by Build.
type Params struct {
	ClassUID    int
	CategoryUID int
	ActivityID  int
	SeverityID  int

	Hostname string // optional; defaults to os.Hostname()
	Username string // optional
	Metadata map[string]any
	Data     map[string]any // class-specific fields; nil becomes {}

	// EventID is optional. Leave it empty for a random UUID. Collectors that
	// re-read a log (journald, Windows Event Log) set it from DeterministicID
	// so the same source record always gets the same ID, and the server's
	// idempotent insert absorbs any re-read after a restart. Must be a UUID.
	EventID string

	// Time is optional. Zero means "now". Collectors set it to the log
	// record's own timestamp so a delayed or replayed record is dated when
	// it happened, not when it was collected.
	Time time.Time
}

// envelope mirrors schemas.ws.WSEvent. Keep field names in sync with it.
type envelope struct {
	Type        string `json:"type"`
	EventID     string `json:"event_id"`
	Time        string `json:"time"`
	ClassUID    int    `json:"class_uid"`
	CategoryUID int    `json:"category_uid"`
	ActivityID  int    `json:"activity_id"`
	TypeUID     int64  `json:"type_uid"`
	SeverityID  int    `json:"severity_id"`

	Hostname string         `json:"hostname,omitempty"`
	Username string         `json:"username,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Data     map[string]any `json:"data"`

	SchemaVersion int    `json:"schema_version"`
	AgentVersion  string `json:"agent_version,omitempty"`
	HostOS        string `json:"host_os,omitempty"`
	HostOSVersion string `json:"host_os_version,omitempty"`
}

var (
	hostOnce  sync.Once
	hostName  string
	osVersion string
)

func hostInfo() (name, version string) {
	hostOnce.Do(func() {
		hostName, _ = os.Hostname()
		osVersion = clip(detectOSVersion(), maxHostOSVersion)
	})
	return hostName, osVersion
}

// detectOSVersion is implemented per-platform in version_linux.go,
// version_windows.go, and version_other.go (the fallback for anything
// else), so this package never imports a platform-specific package (like
// golang.org/x/sys/windows/registry) into a build for a different OS.

// clip truncates s to at most n characters (runes), matching pydantic's
// max_length, which counts characters rather than bytes.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// idNamespace is a fixed namespace so DeterministicID is stable across
// releases. Never change this string: doing so would change every ID.
var idNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("khemstrix-edr/event-id/v1"))

// DeterministicID returns the same UUID for the same parts, every time.
// Collectors pass values that uniquely identify the source record, for
// example (agentID, "journald", cursor) or (agentID, "windows-security", recordID).
// Parts are joined with a NUL byte, so ("ab","c") and ("a","bc") differ.
func DeterministicID(parts ...string) string {
	return uuid.NewSHA1(idNamespace, []byte(strings.Join(parts, "\x00"))).String()
}

// Build returns the event_id and the complete WSEvent JSON for it. Pass
// both straight to wsclient.Push: returning them together guarantees the
// outbox key and the "event_id" inside the payload always match.
//
// The event_id is p.EventID if set (validated, returned in canonical
// lower-case form), otherwise a fresh random UUID. The time is p.Time if
// set, otherwise now; either way it is sent as UTC.
func Build(p Params) (eventID string, payload []byte, err error) {
	if p.ClassUID <= 0 || p.CategoryUID <= 0 {
		return "", nil, fmt.Errorf("event: class_uid and category_uid are required (got %d, %d)", p.ClassUID, p.CategoryUID)
	}
	if p.ActivityID < 0 || p.SeverityID < 0 {
		return "", nil, fmt.Errorf("event: activity_id and severity_id must be >= 0 (got %d, %d)", p.ActivityID, p.SeverityID)
	}

	host, osVer := hostInfo()
	if p.Hostname != "" {
		host = p.Hostname
	}
	data := p.Data
	if data == nil {
		data = map[string]any{} // server requires `data` to be an object, not null
	}

	if p.EventID != "" {
		u, perr := uuid.Parse(p.EventID)
		if perr != nil {
			return "", nil, fmt.Errorf("event: EventID %q is not a valid UUID: %w", p.EventID, perr)
		}
		eventID = u.String() // canonical lower-case form
	} else {
		eventID = uuid.NewString()
	}

	ts := p.Time
	if ts.IsZero() {
		ts = time.Now()
	}

	env := envelope{
		Type:        "event",
		EventID:     eventID,
		Time:        ts.UTC().Format("2006-01-02T15:04:05.000000Z07:00"), // UTC, microseconds, trailing Z
		ClassUID:    p.ClassUID,
		CategoryUID: p.CategoryUID,
		ActivityID:  p.ActivityID,
		TypeUID:     int64(p.ClassUID)*100 + int64(p.ActivityID), // OCSF: class_uid * 100 + activity_id
		SeverityID:  p.SeverityID,

		Hostname: host,
		Username: p.Username,
		Metadata: p.Metadata,
		Data:     data,

		SchemaVersion: SchemaVersion,
		AgentVersion:  clip(AgentVersion, maxAgentVersion),
		HostOS:        clip(runtime.GOOS, maxHostOS), // "windows" / "linux"
		HostOSVersion: osVer,
	}

	payload, err = json.Marshal(env)
	if err != nil {
		return "", nil, fmt.Errorf("event: marshal: %w", err)
	}
	return eventID, payload, nil
}
