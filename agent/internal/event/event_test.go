package event

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBuildEnvelope(t *testing.T) {
	id, payload, err := Build(Params{
		ClassUID: 1001, CategoryUID: 1, ActivityID: 1, SeverityID: 1,
		Data: map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}

	if m["type"] != "event" {
		t.Errorf("type = %v", m["type"])
	}
	if m["event_id"] != id {
		t.Errorf("event_id in payload (%v) != returned id (%s)", m["event_id"], id)
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("event_id not a UUID: %v", err)
	}
	if m["type_uid"] != float64(100101) {
		t.Errorf("type_uid = %v, want 100101", m["type_uid"])
	}
	if m["schema_version"] != float64(SchemaVersion) {
		t.Errorf("schema_version = %v", m["schema_version"])
	}
	if m["agent_version"] != AgentVersion {
		t.Errorf("agent_version = %v", m["agent_version"])
	}
	if m["host_os"] != runtime.GOOS {
		t.Errorf("host_os = %v, want %s", m["host_os"], runtime.GOOS)
	}
	// Server-set fields must never be sent by the agent.
	for _, banned := range []string{"agent_id", "tenant_id", "ingest_source", "created_at"} {
		if _, ok := m[banned]; ok {
			t.Errorf("payload must not contain %q", banned)
		}
	}
	// time must parse as an RFC3339 UTC timestamp.
	ts, ok := m["time"].(string)
	if !ok {
		t.Fatalf("time missing or not a string: %v", m["time"])
	}
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t.Fatalf("time %q not RFC3339: %v", ts, err)
	}
	if _, off := parsed.Zone(); off != 0 {
		t.Errorf("time %q is not UTC", ts)
	}
}

func TestNilDataBecomesObject(t *testing.T) {
	_, payload, err := Build(Params{ClassUID: 1001, CategoryUID: 1, ActivityID: 1, SeverityID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	if _, ok := m["data"].(map[string]any); !ok {
		t.Errorf("data should be an object, got %T (%v)", m["data"], m["data"])
	}
}

func TestBuildRejectsMissingClass(t *testing.T) {
	if _, _, err := Build(Params{SeverityID: 1}); err == nil {
		t.Error("expected error for missing class_uid/category_uid")
	}
}

func TestClipIsRuneSafe(t *testing.T) {
	if got := clip("ááááá", 3); got != "ááá" {
		t.Errorf("clip = %q", got)
	}
	if got := clip("abc", 10); got != "abc" {
		t.Errorf("clip = %q", got)
	}
}
