package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// DefaultCursorFile is where ReadJournal persists the last-processed
// journald cursor between runs, so a restart resumes instead of
// re-scanning history. Adjust ReaderOptions.CursorFile if this path
// doesn't match your deployment layout (e.g. wherever config.json lives).
const DefaultCursorFile = "/var/lib/khemstrix-agent/auth_journal_cursor"

// ReaderOptions configures ReadJournal. The zero value uses sane defaults
// except OnEvent, which you must set — otherwise every parsed event is
// silently discarded.
type ReaderOptions struct {
	CursorFile string
	// OnEvent is called, in order, for every AuthEvent ParseRecord accepts.
	// Step 3 wires this to a print statement (see cmd/authreader). Step 4
	// wires it to ev.Build(agentID) + wsc.Push().
	OnEvent func(AuthEvent)
}

// ReadJournal runs `journalctl -f -o json`, resuming from a saved cursor
// if one exists, and calls opts.OnEvent for every line ParseRecord accepts.
// It blocks until ctx is cancelled or the journalctl subprocess exits.
//
// Per the locked "no backfill" decision: with no cursor file yet, this
// starts from "now" rather than replaying the whole journal, so installing
// the agent doesn't flood anything with history.
func ReadJournal(ctx context.Context, opts ReaderOptions) error {
	if opts.CursorFile == "" {
		opts.CursorFile = DefaultCursorFile
	}
	if opts.OnEvent == nil {
		return fmt.Errorf("auth: ReaderOptions.OnEvent is required")
	}

	args := []string{"-f", "-o", "json"}
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
	// journald lines can be long (a sudo COMMAND= with a long path, etc.);
	// grow past bufio's 64KB default rather than truncating/erroring.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}

		var fields map[string]string
		if jsonErr := json.Unmarshal(line, &fields); jsonErr != nil {
			// A malformed line must not crash the collector — skip it.
			continue
		}

		if ev, ok := ParseRecord(fields); ok {
			opts.OnEvent(ev)
		}

		// Save the cursor after every line — matched or not — so a
		// restart resumes exactly where we left off, rather than
		// re-scanning lines we already decided weren't events.
		if cursor := fields["__CURSOR"]; cursor != "" {
			if saveErr := saveCursor(opts.CursorFile, cursor); saveErr != nil {
				fmt.Fprintf(os.Stderr, "auth: saving cursor: %v\n", saveErr) // non-fatal
			}
		}
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("auth: reading journalctl output: %w", err)
	}
	return cmd.Wait()
}

// saveCursor writes atomically (write to a temp file, then rename on the
// same filesystem) so a crash mid-write can never leave a corrupt or
// half-written cursor file behind.
func saveCursor(path, cursor string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(cursor), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
