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
//	    ClassUID: 3002, CategoryUID: 3, ActivityID: 1, SeverityID: 3, // example values
//	    Username: "alice",
//	    Data:     map[string]any{"src_ip": "10.0.0.7", "reason": "bad password"},
//	})
//	if err != nil { ... }
//	err = wsc.Push(id, payload)
//
// Fields the server sets itself and this package must NOT send:
// agent_id, tenant_id, ingest_source, created_at.
package event

import (
	"bufio"
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

// detectOSVersion returns a human-readable OS version, or "" if unknown
// (the server column is nullable). Linux reads /etc/os-release.
// TODO: Windows (registry ProductName/DisplayVersion or RtlGetVersion).
func detectOSVersion() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}

// clip truncates s to at most n characters (runes), matching pydantic's
// max_length, which counts characters rather than bytes.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Build returns a fresh event_id and the complete WSEvent JSON for it. Pass
// both straight to wsclient.Push: returning them together guarantees the
// outbox key and the "event_id" inside the payload always match.
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

	eventID = uuid.NewString()
	env := envelope{
		Type:        "event",
		EventID:     eventID,
		Time:        time.Now().UTC().Format("2006-01-02T15:04:05.000000Z07:00"), // UTC, microseconds, trailing Z
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
