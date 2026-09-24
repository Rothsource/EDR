//go:build linux

package auth

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"khemstrix-agent/internal/event"
)

// NOTE: placeholderClassUID/CategoryUID/ActivityID/SeverityID and the
// authConfig type now live in common.go (no build tag) so the Windows
// build can use them too. Do not redeclare them here.

const DefaultCursorFile = "/var/lib/khemstrix-agent/auth_journal_cursor"

// DefaultAuthConfigFile is where the auth module's own config lives, per
// Section 7B's file layout.
const DefaultAuthConfigFile = "/var/lib/khemstrix-agent/config/auth.json"

// sourceIdentifiers maps a config-file source name to the journald
// SYSLOG_IDENTIFIER values it corresponds to. "ssh" is a legacy alias
// kept so old hand-edited configs still work; "sshd" is the canonical
// name the server sends.
var sourceIdentifiers = map[string][]string{
	"ssh":  {"sshd", "sshd-session"}, // legacy alias
	"sshd": {"sshd", "sshd-session"},
	"sudo": {"sudo"},
	"su":   {"su"},
}

// relevancePhrases implements Section 7A's "option 3" coarse filter: after
// identifier-level filtering narrows to the right services, this loose,
// non-classifying check trims routine noise (e.g. pam_unix session
// open/close lines) without deciding pass/fail/reason for anything. That
// judgment stays exclusively server-side. Deliberately loose.
var relevancePhrases = [][]byte{
	[]byte("Failed password"),
	[]byte("Accepted password"),
	[]byte("Accepted publickey"),
	[]byte("authentication failure"),
	[]byte("Invalid user"),
	[]byte("Connection closed by authenticating user"),
}

// looksAuthRelevant is the loose, non-classifying check from option 3.
// Sources without a defined relevance list (sudo, su) pass everything
// through unfiltered at this stage; only sshd/sshd-session's high routine
// volume motivated adding this check in the first place.
func looksAuthRelevant(identifier, message string) bool {
	if identifier != "sshd" && identifier != "sshd-session" {
		return true
	}
	msg := []byte(message)
	for _, phrase := range relevancePhrases {
		if bytes.Contains(msg, phrase) {
			return true
		}
	}
	return false
}

// defaultAuthConfig is written on first run, before any server config
// exists, so the agent isn't silent out of the box. It matches the
// server-side default for Linux.
var defaultAuthConfig = authConfig{Sources: []string{"sshd", "sudo", "su"}, Version: 0, Source: "local"}

// loadAuthConfig reads the module's own config file and resolves its
// "sources" list into the set of journald identifiers to watch. A missing
// file is created from defaultAuthConfig. A file with no recognized
// sources yields an empty set, which the caller treats as "watch nothing"
// (no silent fallback to "watch everything").
func loadAuthConfig(path string) (identifiers map[string]bool, err error) {
	identifiers = map[string]bool{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			b, err = json.MarshalIndent(defaultAuthConfig, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("auth: marshal default config: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("auth: creating config dir: %w", err)
			}
			if err := os.WriteFile(path, b, 0o644); err != nil {
				return nil, fmt.Errorf("auth: writing default config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("auth: reading config %s: %w", path, err)
		}
	}

	var cfg authConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("auth: parsing config %s: %w", path, err)
	}

	for _, src := range cfg.Sources {
		for _, id := range sourceIdentifiers[src] {
			identifiers[id] = true
		}
	}
	return identifiers, nil
}

// ReaderOptions configures ReadJournal.
type ReaderOptions struct {
	CursorFile string
	ConfigFile string // defaults to DefaultAuthConfigFile
	AgentID    string
	// Push sends a built event; must return nil only on durable-queue success.
	Push func(eventID string, payload []byte) error
}

// ReadJournal tails journalctl and ships one raw, unparsed record per kept
// line. Per the Section 7A decision, this is option 3 of the coarse
// filter: identifier-level filtering (which pile to read from) plus a
// loose, non-classifying relevance check that trims routine noise. Neither
// step decides pass/fail/reason for anything; that judgment stays
// exclusively server-side.
func ReadJournal(ctx context.Context, opts ReaderOptions) error {
	if opts.CursorFile == "" {
		opts.CursorFile = DefaultCursorFile
	}
	if opts.ConfigFile == "" {
		opts.ConfigFile = DefaultAuthConfigFile
	}
	if opts.Push == nil {
		return fmt.Errorf("auth: ReaderOptions.Push is required")
	}

	watched, err := loadAuthConfig(opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("auth: loading config: %w", err)
	}
	if len(watched) == 0 {
		return fmt.Errorf("auth: no sources configured in %s, nothing to watch", opts.ConfigFile)
	}

	args := []string{"-f", "-o", "json"}
	for id := range watched {
		args = append(args, "-t", id)
	}
	if cursor, err := os.ReadFile(opts.CursorFile); err == nil && len(cursor) > 0 {
		args = append(args, "--after-cursor="+string(cursor))
	} else {
		args = append(args, "--since=now")
	}

	cmd := exec.CommandContext(ctx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("auth: journalctl stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("auth: starting journalctl: %w", err)
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}

		var fields map[string]string
		if err := json.Unmarshal(line, &fields); err != nil {
			continue // malformed line must not crash the collector
		}

		cursor := fields["__CURSOR"]
		identifier := fields["SYSLOG_IDENTIFIER"]
		message := fields["MESSAGE"]

		// Stage 1: identifier-level filter. -t already restricts
		// journalctl's own output to watched identifiers, but this
		// double-checks it and makes the filter's intent explicit in code.
		//
		// Stage 2: loose relevance check (option 3). This is NOT content
		// classification. It only decides whether a line is worth shipping
		// at all, never success/failure/reason.
		if cursor == "" || !watched[identifier] || !looksAuthRelevant(identifier, message) {
			if cursor != "" {
				if err := saveCursor(opts.CursorFile, cursor); err != nil {
					fmt.Fprintf(os.Stderr, "auth: saving cursor: %v\n", err)
				}
			}
			continue
		}

		eventID := event.DeterministicID(opts.AgentID, "journald", cursor)
		id, payload, err := event.Build(event.Params{
			ClassUID:    placeholderClassUID,
			CategoryUID: placeholderCategoryUID,
			ActivityID:  placeholderActivityID,
			SeverityID:  placeholderSeverityID,
			EventID:     eventID,
			Time:        parseRealtime(fields["__REALTIME_TIMESTAMP"]),
			Data: map[string]any{
				"source": "journald",
				"raw": map[string]any{
					"identifier": identifier,
					"message":    message,
					"pid":        fields["_PID"],
				},
			},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "auth: build event: %v\n", err)
			continue
		}

		// The cursor only advances once Push has durably queued the event.
		// If Push fails we must NOT keep reading: a later successful line
		// would save a cursor past this one and lose it permanently. So on
		// failure we stop journalctl and return an error. The collector
		// loop restarts us after its backoff, resuming from the last saved
		// cursor, which is still before this line.
		if err := opts.Push(id, payload); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("auth: push failed, restarting from last saved cursor: %w", err)
		}
		if err := saveCursor(opts.CursorFile, cursor); err != nil {
			fmt.Fprintf(os.Stderr, "auth: saving cursor: %v\n", err)
		}
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("auth: reading journalctl output: %w", err)
	}
	return cmd.Wait()
}

func saveCursor(path, cursor string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(cursor); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
