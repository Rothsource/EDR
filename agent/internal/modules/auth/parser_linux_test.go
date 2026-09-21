package auth

import (
	"testing"
	"time"
)

// Fixtures below are your real journalctl output, scrubbed per your own
// convention: real LAN IP -> 203.0.113.5, real username -> alice.

func TestParseRecord_SSHBadPassword(t *testing.T) {
	// Sep 21 03:27:11 ... Failed password for alice from 203.0.113.5 port 50539 ssh2
	fields := map[string]string{
		"SYSLOG_IDENTIFIER":    "sshd-session",
		"MESSAGE":              "Failed password for alice from 203.0.113.5 port 50539 ssh2",
		"__CURSOR":             "s=823b...;i=103b4;b=354f...;t=65bfbb163b946;x=d653772f4c0ac851",
		"__REALTIME_TIMESTAMP": "1789986431285574", // real value from your Kali box
	}

	got, ok := ParseRecord(fields)
	if !ok {
		t.Fatalf("expected ok=true for a real Failed password line")
	}
	want := AuthEvent{
		Username:   "alice",
		SourceIP:   "203.0.113.5",
		SourcePort: "50539",
		Status:     "failure",
		Reason:     "bad_password",
		Service:    "sshd",
		RawCursor:  fields["__CURSOR"],
		Time:       time.UnixMicro(1789986431285574).UTC(),
	}
	if !got.Time.Equal(want.Time) {
		t.Errorf("got Time %v, want %v", got.Time, want.Time)
	}
	got.Time, want.Time = time.Time{}, time.Time{} // struct equality below ignores Time; checked above
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseRecord_SSHUnknownUser(t *testing.T) {
	// Sep 21 03:31:54 ... Failed password for invalid user nosuchuser from 203.0.113.5 port 58951 ssh2
	fields := map[string]string{
		"SYSLOG_IDENTIFIER":    "sshd-session",
		"MESSAGE":              "Failed password for invalid user nosuchuser from 203.0.113.5 port 58951 ssh2",
		"__CURSOR":             "s=823b...;i=103ec;b=354f...;t=65bfbc1af000;x=aaaa",
		"__REALTIME_TIMESTAMP": "1789986714630866", // real value from your Kali box
	}

	got, ok := ParseRecord(fields)
	if !ok {
		t.Fatalf("expected ok=true for a real invalid-user Failed password line")
	}
	if got.Reason != "unknown_user" {
		t.Errorf("got reason %q, want %q", got.Reason, "unknown_user")
	}
	if got.Username != "nosuchuser" {
		t.Errorf("got username %q, want %q", got.Username, "nosuchuser")
	}
	if got.SourceIP != "203.0.113.5" || got.SourcePort != "58951" {
		t.Errorf("got ip/port %q/%q, want 203.0.113.5/58951", got.SourceIP, got.SourcePort)
	}
	wantTime := time.UnixMicro(1789986714630866).UTC()
	if !got.Time.Equal(wantTime) {
		t.Errorf("got Time %v, want %v", got.Time, wantTime)
	}
}

func TestParseRecord_SudoBadPassword(t *testing.T) {
	// Sep 21 03:28:02 ... pam_unix(sudo:auth): authentication failure; logname=alice uid=1000 euid=0 tty=/dev/pts/2 ruser=alice rhost=  user=alice
	fields := map[string]string{
		"SYSLOG_IDENTIFIER":    "sudo",
		"MESSAGE":              "pam_unix(sudo:auth): authentication failure; logname=alice uid=1000 euid=0 tty=/dev/pts/2 ruser=alice rhost=  user=alice",
		"__CURSOR":             "s=823b...;i=103c9;b=354f...;t=65bfbb9a0000;x=bbbb",
		"__REALTIME_TIMESTAMP": "1789986482000000", // real value from your Kali box
	}

	got, ok := ParseRecord(fields)
	if !ok {
		t.Fatalf("expected ok=true for a real sudo authentication failure line")
	}
	want := AuthEvent{
		Username:   "alice",
		SourceIP:   "", // rhost is legitimately blank for local sudo
		SourcePort: "",
		Status:     "failure",
		Reason:     "sudo_bad_password",
		Service:    "sudo",
		RawCursor:  fields["__CURSOR"],
		Time:       time.UnixMicro(1789986482000000).UTC(),
	}
	if !got.Time.Equal(want.Time) {
		t.Errorf("got Time %v, want %v", got.Time, want.Time)
	}
	got.Time, want.Time = time.Time{}, time.Time{}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseRecord_GarbageAndBenignInput(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]string
	}{
		{
			name: "connection reset (client aborted, not a real failure)",
			fields: map[string]string{
				"SYSLOG_IDENTIFIER": "sshd-session",
				"MESSAGE":           "Connection reset by 203.0.113.5 port 62968 [preauth]",
				"__CURSOR":          "s=1",
			},
		},
		{
			name: "successful login",
			fields: map[string]string{
				"SYSLOG_IDENTIFIER": "sshd-session",
				"MESSAGE":           "Accepted password for alice from 203.0.113.5 port 51105 ssh2",
				"__CURSOR":          "s=2",
			},
		},
		{
			name: "pam check pass (unknown user probe, not the verdict line)",
			fields: map[string]string{
				"SYSLOG_IDENTIFIER": "sshd-session",
				"MESSAGE":           "pam_unix(sshd:auth): check pass; user unknown",
				"__CURSOR":          "s=3",
			},
		},
		{
			name: "sudo session opened (success, not a failure)",
			fields: map[string]string{
				"SYSLOG_IDENTIFIER": "sudo",
				"MESSAGE":           "pam_unix(sudo:session): session opened for user root(uid=0) by alice(uid=1000)",
				"__CURSOR":          "s=4",
			},
		},
		{
			name: "sudo N incorrect attempts summary (redundant w/ per-attempt failure lines)",
			fields: map[string]string{
				"SYSLOG_IDENTIFIER": "sudo",
				"MESSAGE":           "alice : 3 incorrect password attempts ; TTY=pts/2 ; PWD=/home/alice ; USER=root ; COMMAND=/usr/bin/systemctl status khemstrix-agent",
				"__CURSOR":          "s=5",
			},
		},
		{
			name:   "completely empty record",
			fields: map[string]string{},
		},
		{
			name: "unrelated systemd noise",
			fields: map[string]string{
				"SYSLOG_IDENTIFIER": "systemd",
				"MESSAGE":           "Starting ssh.service - OpenBSD Secure Shell server...",
				"__CURSOR":          "s=6",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseRecord(c.fields)
			if ok {
				t.Errorf("expected ok=false, got ok=true with event %+v", got)
			}
		})
	}
}

func TestAuthEvent_Validate(t *testing.T) {
	valid := AuthEvent{Username: "alice", Service: "sshd", Reason: "bad_password", RawCursor: "s=1"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid event to pass, got err: %v", err)
	}

	cases := []struct {
		name string
		ev   AuthEvent
	}{
		{"missing cursor", AuthEvent{Username: "alice", Service: "sshd", Reason: "bad_password"}},
		{"missing username", AuthEvent{Service: "sshd", Reason: "bad_password", RawCursor: "s=1"}},
		{"missing service", AuthEvent{Username: "alice", Reason: "bad_password", RawCursor: "s=1"}},
		{"missing reason", AuthEvent{Username: "alice", Service: "sshd", RawCursor: "s=1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.ev.Validate(); err == nil {
				t.Errorf("expected an error for %s", c.name)
			}
		})
	}
}
