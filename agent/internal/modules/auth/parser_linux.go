package auth

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

type AuthEvent struct {
	Username   string
	SourceIP   string
	SourcePort string
	Status     string
	Reason     string
	Service    string
	RawCursor  string
	Time       time.Time
	Count      int
}

func parseRealtime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	micros, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMicro(micros).UTC()
}

const maxFieldLen = 64

func clamp(s string) string {
	if len(s) > maxFieldLen {
		return s[:maxFieldLen]
	}
	return s
}

var (
	sshFailedPassword = regexp.MustCompile(`^Failed password for (invalid user )?(\S+) from (\S+) port (\d+) ssh2$`)

	sudoAuthFailure = regexp.MustCompile(`^pam_unix\(sudo:auth\): authentication failure; logname=(\S*) uid=(\d+) euid=(\d+) tty=(\S+) ruser=(\S*) rhost=(\S*)\s+user=(\S+)$`)

	sshPamSummary = regexp.MustCompile(`^PAM (\d+) more authentication failures?; logname=(\S*) uid=(\d+) euid=(\d+) tty=(\S+) ruser=(\S*) rhost=(\S*)(?:\s+user=(\S+))?$`)
)

func ParseRecord(fields map[string]string) (AuthEvent, bool) {
	msg := fields["MESSAGE"]
	cursor := fields["__CURSOR"]
	ts := parseRealtime(fields["__REALTIME_TIMESTAMP"])

	switch fields["SYSLOG_IDENTIFIER"] {
	case "sshd-session", "sshd":
		if m := sshFailedPassword.FindStringSubmatch(msg); m != nil {
			reason := "bad_password"
			if m[1] != "" {
				reason = "unknown_user"
			}
			return AuthEvent{
				Username:   clamp(m[2]),
				SourceIP:   m[3],
				SourcePort: m[4],
				Status:     "failure",
				Reason:     reason,
				Service:    "sshd",
				RawCursor:  cursor,
				Time:       ts,
			}, true
		}

		if m := sshPamSummary.FindStringSubmatch(msg); m != nil {
			count, err := strconv.Atoi(m[1])
			if err != nil {
				return AuthEvent{}, false
			}
			username := ""
			if len(m) > 8 {
				username = clamp(m[8])
			}
			return AuthEvent{
				Username:  username,
				SourceIP:  m[7],
				Status:    "failure",
				Reason:    "auth_failures_summary",
				Service:   "sshd",
				RawCursor: cursor,
				Time:      ts,
				Count:     count,
			}, true
		}

	case "sudo":
		if m := sudoAuthFailure.FindStringSubmatch(msg); m != nil {
			return AuthEvent{
				Username:   clamp(m[7]),
				SourceIP:   clamp(m[6]),
				SourcePort: "",
				Status:     "failure",
				Reason:     "sudo_bad_password",
				Service:    "sudo",
				RawCursor:  cursor,
				Time:       ts,
			}, true
		}
	}

	return AuthEvent{}, false
}

func (e AuthEvent) Validate() error {
	if e.RawCursor == "" {
		return fmt.Errorf("linuxauth: missing journal cursor")
	}
	if e.Username == "" && e.Reason != "auth_failures_summary" {
		return fmt.Errorf("linuxauth: missing username")
	}
	if e.Service == "" || e.Reason == "" {
		return fmt.Errorf("linuxauth: missing service/reason")
	}
	return nil
}
