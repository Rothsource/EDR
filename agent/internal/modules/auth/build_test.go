package auth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleEvent() AuthEvent {
	return AuthEvent{
		Username:   "alice",
		SourceIP:   "203.0.113.5",
		SourcePort: "50539",
		Status:     "failure",
		Reason:     "bad_password",
		Service:    "sshd",
		RawCursor:  "s=823b...;i=103b4;b=354f...",
		Time:       time.UnixMicro(1789986431285574).UTC(),
	}
}

func TestAuthEvent_Build_SummaryLine_UnknownUser_EmptyUsername(t *testing.T) {
	e := AuthEvent{
		Username:  "", // PAM summary line has no user= field for unknown accounts
		SourceIP:  "192.168.5.1",
		Status:    "failure",
		Reason:    "auth_failures_summary",
		Service:   "sshd",
		RawCursor: "s=abc;i=3",
		Count:     1,
	}
	_, payload, err := e.Build("test-agent-id")
	if err != nil {
		t.Fatalf("unexpected error with empty username: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if got, _ := env["username"].(string); got != "" {
		t.Errorf("expected empty username in envelope, got %q", got)
	}
}

func TestAuthEvent_Build_SummaryLine_CarriesCount(t *testing.T) {
	e := AuthEvent{
		Username:  "vectorpeace",
		SourceIP:  "192.168.5.1",
		Status:    "failure",
		Reason:    "auth_failures_summary",
		Service:   "sshd",
		RawCursor: "s=abc;i=2",
		Count:     2,
	}
	_, payload, err := e.Build("test-agent-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(payload), `"count":2`) {
		t.Fatalf("expected count in payload, got: %s", payload)
	}
}

func TestAuthEvent_Build_NonSummaryLine_OmitsCount(t *testing.T) {
	e := AuthEvent{
		Username:  "vectorpeace",
		Status:    "failure",
		Reason:    "bad_password",
		Service:   "sshd",
		RawCursor: "s=abc;i=1",
	}
	_, payload, err := e.Build("test-agent-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(payload), `"count"`) {
		t.Fatalf("did not expect count in payload for non-summary event, got: %s", payload)
	}
}

func TestAuthEvent_Build_Envelope(t *testing.T) {
	id, payload, err := sampleEvent().Build("agent-123")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if id == "" {
		t.Fatalf("expected a non-empty event_id")
	}

	var env map[string]any
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}

	checks := map[string]any{
		"class_uid":    float64(3002),
		"category_uid": float64(3),
		"activity_id":  float64(1),
		"severity_id":  float64(2),
		"type_uid":     float64(300201), // class_uid*100 + activity_id, per event.Build()
		"username":     "alice",
		"event_id":     id,
	}
	for key, want := range checks {
		if got := env[key]; got != want {
			t.Errorf("envelope[%q] = %v, want %v", key, got, want)
		}
	}

	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be an object, got %T", env["data"])
	}
	dataChecks := map[string]string{
		"status":   "failure",
		"reason":   "bad_password",
		"service":  "sshd",
		"src_ip":   "203.0.113.5",
		"src_port": "50539",
	}
	for key, want := range dataChecks {
		if got, _ := data[key].(string); got != want {
			t.Errorf("data[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestAuthEvent_Build_OmitsEmptySourceFields(t *testing.T) {
	// A sudo failure has no SourceIP/SourcePort — confirm they're left out
	// of `data` entirely rather than sent as empty strings.
	ev := AuthEvent{
		Username:  "alice",
		Status:    "failure",
		Reason:    "sudo_bad_password",
		Service:   "sudo",
		RawCursor: "s=1",
		Time:      time.Now().UTC(),
	}

	_, payload, err := ev.Build("agent-123")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if _, present := data["src_ip"]; present {
		t.Errorf("expected src_ip to be omitted, got %v", data["src_ip"])
	}
	if _, present := data["src_port"]; present {
		t.Errorf("expected src_port to be omitted, got %v", data["src_port"])
	}
}

func TestAuthEvent_Build_DeterministicPerAgent(t *testing.T) {
	ev := sampleEvent()

	id1, _, err := ev.Build("agent-A")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	id2, _, err := ev.Build("agent-A")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if id1 != id2 {
		t.Errorf("same agent+event should produce the same event_id: got %q and %q", id1, id2)
	}

	id3, _, err := ev.Build("agent-B")
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if id3 == id1 {
		t.Errorf("different agents re-reading a coincidentally identical cursor should not collide, got same event_id %q", id1)
	}
}
