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
	// Escapes keep this test independent of the file's encoding: five
	// two-byte characters, clipped to three, must stay whole characters.
	in := "\u00e1\u00e1\u00e1\u00e1\u00e1"
	want := "\u00e1\u00e1\u00e1"
	if got := clip(in, 3); got != want {
		t.Errorf("clip = %q, want %q", got, want)
	}
	if got := clip("abc", 10); got != "abc" {
		t.Errorf("clip = %q", got)
	}
}

func TestBuildCustomEventID(t *testing.T) {
	// Upper-case input must come back as the canonical lower-case UUID,
	// and the payload must carry the same value.
	id, payload, err := Build(Params{
		ClassUID: 3002, CategoryUID: 3, ActivityID: 1, SeverityID: 2,
		EventID: "3F2504E0-4F89-11D3-9A0C-0305E82C3301",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "3f2504e0-4f89-11d3-9a0c-0305e82c3301" {
		t.Errorf("id = %s", id)
	}
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	if m["event_id"] != id {
		t.Errorf("payload event_id %v != returned id %s", m["event_id"], id)
	}
}

func TestBuildRejectsBadEventID(t *testing.T) {
	_, _, err := Build(Params{
		ClassUID: 3002, CategoryUID: 3, ActivityID: 1, SeverityID: 2,
		EventID: "not-a-uuid",
	})
	if err == nil {
		t.Error("expected error for invalid EventID")
	}
}

func TestBuildTimeOverride(t *testing.T) {
	// 13:00 in UTC+7 is 06:00 UTC. The payload must be converted to UTC.
	local := time.Date(2026, 9, 21, 13, 0, 0, 123456000, time.FixedZone("ICT", 7*3600))
	_, payload, err := Build(Params{
		ClassUID: 3002, CategoryUID: 3, ActivityID: 1, SeverityID: 2,
		Time: local,
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	if m["time"] != "2026-09-21T06:00:00.123456Z" {
		t.Errorf("time = %v", m["time"])
	}
}

func TestBuildDefaultTimeIsNow(t *testing.T) {
	before := time.Now().Add(-2 * time.Second)
	_, payload, err := Build(Params{ClassUID: 1001, CategoryUID: 1, ActivityID: 1, SeverityID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	parsed, err := time.Parse(time.RFC3339Nano, m["time"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Before(before) || parsed.After(time.Now().Add(2*time.Second)) {
		t.Errorf("default time %v is not close to now", parsed)
	}
}

func TestDeterministicID(t *testing.T) {
	a := DeterministicID("agent-1", "journald", "cursor-42")
	b := DeterministicID("agent-1", "journald", "cursor-42")
	if a != b {
		t.Errorf("same input gave different IDs: %s vs %s", a, b)
	}
	if _, err := uuid.Parse(a); err != nil {
		t.Errorf("not a valid UUID: %v", err)
	}
	if a == DeterministicID("agent-1", "journald", "cursor-43") {
		t.Error("different input gave the same ID")
	}
	if a == DeterministicID("agent-2", "journald", "cursor-42") {
		t.Error("different agent gave the same ID")
	}
	// Part boundaries matter: ("ab","c") must not equal ("a","bc").
	if DeterministicID("ab", "c") == DeterministicID("a", "bc") {
		t.Error("part boundaries are not respected")
	}
}
