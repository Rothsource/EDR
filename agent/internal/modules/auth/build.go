package auth

import "khemstrix-agent/internal/event"

const (
	classUID    = 3002
	categoryUID = 3
	activityID  = 1
	severityID  = 2
)

func (e AuthEvent) Build(agentID string) (eventID string, payload []byte, err error) {
	data := map[string]any{
		"status":  e.Status,
		"reason":  e.Reason,
		"service": e.Service,
	}
	if e.SourceIP != "" {
		data["src_ip"] = e.SourceIP
	}
	if e.SourcePort != "" {
		data["src_port"] = e.SourcePort
	}
	if e.Reason == "auth_failures_summary" {
		data["count"] = e.Count
	}

	return event.Build(event.Params{
		ClassUID:    classUID,
		CategoryUID: categoryUID,
		ActivityID:  activityID,
		SeverityID:  severityID,
		Username:    e.Username,
		Data:        data,
		EventID:     event.DeterministicID(agentID, "journald", e.RawCursor),
		Time:        e.Time,
	})
}
