# Project Status Report

Snapshot after completing the Go agent + one-line install milestone. This
reflects the actual state of the codebase — not the target state.
Cross-reference `api-contract.md` for exact endpoint shapes and
`developer.md` for how to pick up any of the "not yet done" items below.

---

## ✅ Done and Tested

### Backend — core connectivity (Phase 1.0)
- Postgres schema: `enrollment_tokens`, `agents`, `users` tables created
  and verified via `psql`
- `config.py` → `db/database.py` → `db/models.py` chain confirmed working
  end-to-end
- Timezone handling bug (naive vs. aware `datetime` on `timestamp without
  time zone` columns) found and fixed for **storage** — documented in
  `developer.md` so it isn't reintroduced in new endpoints
- **A second, related timezone bug was found and fixed in this phase**:
  `AgentResponse` was serializing `created_at`/`last_seen_at` back to JSON
  *without* a UTC marker, so browsers in non-UTC timezones (tested from
  UTC+7) misread "just happened" as "hours ago." Fixed with a
  `@field_serializer` on `AgentResponse` in `schemas/agent.py` that
  explicitly stamps naive DB datetimes as UTC before serializing. Any new
  response schema returning a `datetime` should follow this same pattern —
  don't rely on Pydantic's default serialization for naive datetimes.

### Backend — agent lifecycle
- `POST /admin/generate-token` — protected, 1-hour single-use enrollment
  token
- `POST /agent/register` — validates the enrollment token, generates a
  permanent `api_key`, creates the `Agent` row, marks the token used — now
  also accepts and persists **`ip_address`** and **`mac_address`**
  (see "Schema changes" below)
- `POST /agent/heartbeat` — validates `agent_id` + `api_key` + active
  status, updates `last_seen_at`
- `GET /agents` — protected, lists all agents, excludes `api_key`, now
  includes `ip_address`/`mac_address`, and correctly returns UTC-stamped
  timestamps (see timezone fix above)
- `PATCH /admin/agents/{agent_id}/revoke` — sets status to `"revoked"` —
  **now wired into the dashboard** (previously built-but-disconnected, see
  "Closed gaps" below)
- `DELETE /admin/agents/{agent_id}` — hard-deletes an agent row — **now
  wired into the dashboard**
- **`PATCH /admin/agents/{agent_id}/unrevoke`** — new endpoint, not in the
  original plan. Sets status back to `"active"`. Reuses `AgentActionResponse`.
  Confirmed the reversal is transparent to the agent: the same `api_key`
  works immediately on the agent's next heartbeat cycle, no re-registration
  needed.

### Schema changes — `ip_address` + `mac_address` on `agents`
Both columns added following the exact checklist in `developer.md` §3
(Postgres → `models.py` → `schemas/agent.py` → `routers/agent.py`).
**Worth noting for future changes**: `ip_address` initially shipped with a
gap — it was present in Postgres, `schemas/agent.py`, and
`routers/agent.py`, but missing from `db/models.py`, so SQLAlchemy silently
dropped it on every insert with no error. Confirmed only by querying
Postgres directly (`SELECT ... FROM agents`), not by trusting `/docs`
responses, which echoed the field back via Pydantic without it ever being
persisted. **Takeaway for any future column addition: verify with a direct
`psql` SELECT after the first real write, not just the API response.**

### Go agent (`agent/`) — built, compiled, tested end-to-end
Fully built out from the empty scaffold:
- `internal/core/netinfo.go` — walks network interfaces, returns IP + MAC
  from the *same* interface (skips loopback/disabled interfaces, first
  active IPv4)
- `internal/config/config.go` — CLI flag parsing (`--server`, `--token`),
  local config persistence at `C:\ProgramData\khemstrix-agent\config.json`
  (Windows) / `/etc/khemstrix-agent/config.json` (Linux)
- `internal/core/sender.go` — `Register()` and `Heartbeat()`, with the
  designed distinction between `err != nil` (can't reach server) and
  `resp.StatusCode != 200` (reached server, request rejected)
- `cmd/agent/main.go` — the state machine: config exists → heartbeat loop;
  config missing → require `--token`, register (loud/fatal on failure),
  save config, then heartbeat loop (quiet/retry on failure, never crashes)
- Module name: `khemstrix-agent`. Compiles clean on both Windows
  (`GOOS=windows`) and Linux (`GOOS=linux`, cross-compiled from Windows —
  no separate Linux build machine needed)

**Testing performed:**
- Local same-machine test: registered, heartbeated, confirmed in `psql`
- Cross-machine test: ran the compiled binary from a genuinely separate
  Kali VM against the Windows host server (`uvicorn --host 0.0.0.0`),
  confirmed `hostname`/`os`/`ip_address`/`mac_address` in `psql` matched
  the VM, not the host
- Full one-liner install flow tested: real download endpoint → real binary
  → runs → registers → heartbeats, all from a fresh VM with no manual file
  copying

**Known scaffold cleanup:** the original folder plan included several
placeholder files for later phases (`internal/core/event.go`,
`internal/core/module.go`, `internal/modules/{email,file,network,process,
response}/*.go`, `internal/platform/service_{linux,windows}.go`) — all were
**empty 0-byte files**, which breaks `go build` (`expected 'package', found
'EOF'`). All were deleted for now since they're not needed until their
respective phases begin; recreate with real `package` declarations when
picking those phases back up.

### Backend — download distribution
- `routers/downloads.py` (new file) — `GET /download/agent/windows`,
  `GET /download/agent/linux`, both public (no JWT — the caller hasn't
  registered yet), serve compiled binaries from `static/binaries/` via
  `FileResponse`, with a `404` if the binary isn't present yet
- **Bug found and fixed**: initial implementation built `BINARY_DIR` as a
  relative path (`"app/static/binaries"`), which resolved incorrectly
  depending on the current working directory `uvicorn` was launched from.
  Fixed by resolving the path relative to `__file__` instead
  (`os.path.dirname(os.path.dirname(os.path.abspath(__file__)))`) — this is
  now the standard pattern for any future file-serving code in this
  project; never trust cwd in server code.
- Real compiled binaries (`khemstrixAgent.exe`, `khemstrixAgent`) are in
  place in `static/binaries/`, replacing the earlier dummy test files.
  **These need to be manually rebuilt and re-copied any time the Go agent
  source changes** — no CI/build automation exists yet.

### Dashboard (React + Vite)
- Login page + `AuthContext`, `api.js` centralized request helper,
  `AuthError` → `forceLogout()` — unchanged from Phase 1.0, still solid
- **`GenerateTokenModal.jsx`** — now generates the real chained
  download-and-run one-liner (Windows PowerShell / Linux bash toggle),
  built from the dashboard's own `API_URL` (now exported from `api.js`)
  and the freshly generated token — replacing the old static placeholder
  command
  - **Bug found and fixed**: `API_URL` defaults to `http://localhost:8000`
    when `VITE_API_URL` isn't set in `.env`. That default is correct for
    the dashboard's *own* API calls but wrong for the install command,
    which must run on a different machine — `localhost` in that context
    means "call yourself." Fixed by setting `VITE_API_URL` explicitly to
    the server's real LAN-reachable IP in `.env`. **This same variable
    will need to become the real public domain at deployment time** — no
    code changes required then, just the `.env` value.
- **`Agents.jsx`** — significantly expanded:
  - Added `ip_address` / `mac_address` columns (fallback `—` for blank
    values, e.g. agents registered before the schema change)
  - Added a real Actions column: Revoke / Reactivate (context-dependent)
    and Delete (with an inline two-step confirm, not a native `confirm()`)
  - **Bug found and fixed**: `StatusBadge` was being passed a boolean
    (`online={isAgentOnline(agent)}`) on a prop (`online`) the component
    never actually read — it expects a `state` string prop. This meant
    every agent silently showed "Offline" regardless of real status. Fixed
    by switching to `getAgentState(agent)` (already correctly built for
    three states in `agentStatus.js`, just never wired up) and passing
    `state={getAgentState(agent)}`.
- **`AppShell.jsx`** — removed a `max-w-6xl` cap on the main content area
  that was preventing the layout from using full window width; `flex-1`
  alone handles full-width responsiveness correctly
- **`StatusBadge.jsx`** — no changes needed; was already correctly built
  for `online`/`offline`/`revoked` states, just never received the right
  prop until the `Agents.jsx` fix above

---

## 🟡 Closed Gaps (previously "built but not connected")

- **`PATCH /admin/agents/{agent_id}/revoke`** — now fully wired: `api.js`
  function, `Agents.jsx` button, confirmed working end-to-end including
  the effect on a live heartbeating agent (heartbeats start failing with
  `401` immediately, `last_seen_at` freezes at the last successful check-in)
- **`DELETE /admin/agents/{agent_id}`** — now fully wired, including the
  confirm-before-delete step recommended in the previous report
- **`PUT /auth/change-password`** — still not confirmed whether any page
  has a UI form calling it. Not touched this phase — carrying forward as
  an open item.

---

## ⬜ Not Started

- **Background/service execution.** The Go agent currently only runs in
  the foreground, in a terminal a human has to keep open — not a real
  background service yet. Planned next step:
  - **Linux**: a systemd unit file (`khemstrix-agent.service`) — no Go
    code changes needed, systemd just manages the existing binary's
    lifecycle (`systemctl start/stop/status/enable`)
  - **Windows**: genuinely more involved — Windows doesn't run arbitrary
    `.exe`s as services without the binary itself speaking the Windows
    Service Control Manager protocol. Requires Go code changes (likely
    `golang.org/x/sys/windows/svc`), not just an external config file.
  - Suggested order: Linux first (lower complexity, already have a tested
    Linux binary), Windows second.
- **Event pipeline** (`events` table, `routers/events.py`,
  `detection/dispatcher.py`, per-type scorers) — still fully commented out
  in `db/models.py`. Phase 1.1 per the roadmap, untouched this session.
- **Response actions** (`response/actions.py`) — Phase 3, no code written.
- **Alembic migrations** — still manual. The `ip_address`/`mac_address`
  additions this phase are a good concrete example of why this becomes
  worth introducing soon: two real bugs this session (`ip_address` missing
  from `models.py`, and the double-`/admin` prefix bug on the `unrevoke`
  route) came from manually keeping multiple files in sync by hand.
- **Un-revoke button parity with revoke's safety UX** — revoke and delete
  both give visual feedback via the Actions column; un-revoke does too now,
  but there's no confirmation step (arguably fine, since it's non-destructive)
  — worth a deliberate decision, not an oversight, but noting it.

---

## Known Gaps / Pre-Production Hardening Items

Carried forward from the previous report, still accurate:
- **`api_key` stored in plaintext** — unchanged, still a known simplification
- **No server-side JWT revocation** — unchanged
- **`create_admin.py` is a dev/testing tool** — unchanged
- **No HTTPS** — unchanged, all traffic still plain HTTP. Now more
  concretely relevant: `VITE_API_URL` will need to become an `https://`
  domain at deployment, and Go's `net/http` client handles TLS
  automatically with no code change — just the URL scheme.
- **No multi-tenancy** — unchanged
- **CORS locked to `http://localhost:5173`** — unchanged, will need
  updating once the dashboard itself is deployed to a real domain

**New items found this phase:**
- **The compiled Windows binary is flagged by 2/60 AV engines on
  VirusTotal** (both generic ML/heuristic detections — `Bkav` and
  Microsoft's `Wacatac.C!ml` — not signature matches). Expected for any
  unsigned, freshly-compiled, zero-reputation binary that does
  network-monitoring-agent-like things (persistent background process,
  outbound connections, system info collection) — not unique to this
  codebase, and not currently blocking testing. **Code-signing the binary
  is a real requirement before any production distribution**, both to
  reduce AV friction and as standard practice for distributed executables.
- **Revoke/delete don't stop the agent process itself.** Both only remove
  or flag the server-side database row. A physical agent process left
  running on a revoked or deleted machine will keep retrying its heartbeat
  loop forever (quiet-fail by design), showing up as a steady stream of
  `401`s in server logs. Nothing to fix code-wise — this is expected
  agent-side behavior — but worth knowing operationally: **deleting a
  dashboard row does not uninstall or stop anything on the actual
  endpoint.**
- **Duplicate/stale test agent rows.** Several rows accumulated in
  `agents` from iterative testing (mistyped server IPs consuming tokens,
  pre-schema-change registrations with blank IP/MAC, etc.). Cleaned up
  manually via the now-working dashboard delete button; worth a quick
  `psql` sanity check (`SELECT hostname, created_at FROM agents ORDER BY
  created_at;`) before any demo to confirm the list only shows real,
  current agents.

---

## Suggested Next Steps (in priority order)

1. **Background service execution** — systemd unit (Linux) first, then
   Windows Service support (Go code change) — this is what actually makes
   the agent usable outside of an open terminal window, and is the natural
   next milestone after the one-line install work.
2. Confirm/complete the `change-password` UI in `Settings.jsx`
3. Bring the `events` table online (Phase 1.1) — unblocks testing the
   agent's future event-sending path independently of scoring
4. Begin Phase 1.1 detection logic once events are flowing
5. Consider introducing Alembic given two manual-sync bugs surfaced this
   session