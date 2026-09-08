# Setup & Troubleshooting Guide

A practical, "how do I actually get this running / how do I fix the thing
that just broke" companion to the other docs. `architecture.md` explains
*why* things are built the way they are; `developer.md` explains *how to
extend* the system; this doc is for *getting it running* and *unsticking
yourself* when something familiar goes wrong. Every problem below actually
happened during development — nothing here is hypothetical.

---

## 1. First-Time Setup

### 1.1 Backend

```powershell
cd server/app
python -m venv ../venv          # if not already created
../venv/Scripts/activate
pip install -r requirements.txt # if a requirements.txt exists; otherwise
                                 # install fastapi, sqlalchemy[asyncio],
                                 # asyncpg, python-jose, passlib[bcrypt],
                                 # uvicorn, python-dotenv individually
uvicorn main:app --reload
```

Runs on `http://localhost:8000` by default, `/docs` for interactive
testing.

**To reach the server from a different machine (a VM, a phone on the same
network, etc.), you must bind it explicitly:**
```powershell
uvicorn main:app --reload --host 0.0.0.0 --port 8000
```
Without `--host 0.0.0.0`, the server only accepts connections from
`localhost` — a separate VM trying to connect will simply time out or
refuse, with no useful error message pointing at this cause.

### 1.2 Dashboard

```powershell
cd dashboard
npm install
npm run dev
```

Runs on `http://localhost:5173` by default (Vite).

**Set `dashboard/.env` before your first real test:**
```
VITE_API_URL=http://localhost:8000
```
Change this to your server's real LAN IP or domain once you need the
dashboard — or, critically, the agent install command it generates — to
be reachable from anywhere other than the exact machine running the
dashboard. See §3.4 and §4.1 below; this is one of the most common points
of confusion.

**`.env` changes require a full restart** of `npm run dev` (`Ctrl+C` then
re-run) — Vite's hot-reload does not pick up new environment variables on
its own.

### 1.3 Go agent

```powershell
cd agent
go mod init khemstrix-agent     # only once, if go.mod doesn't exist yet
go build -o khemstrixAgent.exe ./cmd/agent
```

Cross-compiling the Linux binary from Windows (no separate Linux machine
needed):
```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -o khemstrixAgent ./cmd/agent
```

**Always reset back to Windows afterward** if you're going to keep
building in the same terminal session:
```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"
```
Leftover cross-compile environment variables silently persist for the
rest of that terminal session — a later `go build` intended for Windows
will produce a Linux binary instead, with no warning.

Running it:
```powershell
.\khemstrixAgent.exe --server=http://<server-ip>:8000 --token=<enrollment-token>
```

### 1.4 Getting a fresh enrollment token

Tokens are single-use and expire after 1 hour. Generate one via `/docs`
(`POST /admin/generate-token`, needs a valid admin JWT — log in via
`/auth/login` first) or through the dashboard's "Generate Enrollment
Token" button.

---

## 2. Backend Problems

### 2.1 `IndentationError` / `SyntaxError` on startup, pointing at a line you just edited

```
File "...\routers\agent.py", line 66
    return AgentRegisterResponse(...)
IndentationError: unexpected indent
```

**Cause:** whitespace drift — usually a stray tab mixed with spaces, or a
line left at the wrong indentation level after editing nearby code.

**Fix:** open the file, check the indentation of the reported line against
the lines immediately around it. Make sure your editor is consistently
using spaces (most Python style guides: 4 spaces, no tabs) throughout the
function.

### 2.2 New field accepted by `/docs` but always `NULL`/blank in the database

**Symptom:** you added a new column (e.g. `ip_address`), updated the
schema, tested via `/docs`, and it *looked* like it worked — the response
echoed the value back. But querying Postgres directly shows the column is
empty.

**Cause:** the field is present in `schemas/agent.py` and referenced in
`routers/agent.py`, but was never actually added to `db/models.py`.
SQLAlchemy silently ignores any keyword argument passed to a model
constructor that the model class doesn't define as a `Column` — no error,
no warning.

**Fix:** add the missing `Column(...)` line to the model class in
`db/models.py`. This is exactly what happened with `ip_address` during
this project's development — see `developer.md` §3 for the full checklist
that prevents this (Postgres → `models.py` → `schemas` → `router`, in that
order, all four steps, every time).

**Prevention:** never trust `/docs` alone as proof a new field is being
stored. After the first real write, confirm directly:
```sql
SELECT hostname, ip_address, mac_address FROM agents ORDER BY created_at DESC LIMIT 1;
```

### 2.3 A newly-added `PATCH`/`POST`/`DELETE` route returns `404`, even though the code looks right and the server restarted fine

**Symptom:** you added a new route, e.g. `PATCH
/admin/agents/{agent_id}/unrevoke`, but calling it returns `404 Not
Found`.

**Cause:** a double-prefixed path. If the router is mounted with a prefix
in `main.py`:
```python
app.include_router(admin.router, prefix="/admin", tags=["admin"])
```
then every route *inside* `routers/admin.py` should be written **without**
`/admin` in its own path string:
```python
@router.patch("/agents/{agent_id}/revoke", ...)   # becomes /admin/agents/{agent_id}/revoke
```
If a new route is accidentally written *with* the prefix already included:
```python
@router.patch("/admin/agents/{agent_id}/unrevoke", ...)   # becomes /admin/admin/agents/.../unrevoke
```
the real path ends up doubled and doesn't match what any client is
actually calling.

**Fix:** check every existing route in the same file for the pattern
they follow, and match it exactly — don't include the router's mount
prefix in individual route decorators.

### 2.4 Download endpoint returns `404 { "detail": "agent binary not available" }` even though the file is definitely in `static/binaries/`

**Cause:** the binary directory path was built relative to the current
working directory (e.g. `BINARY_DIR = "app/static/binaries"`), which only
resolves correctly if `uvicorn` happens to be launched from one specific
folder. Launch it from anywhere else and the path silently points
somewhere that doesn't exist.

**Fix:** build the path relative to the router file's own location
instead:
```python
BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BINARY_DIR = os.path.join(BASE_DIR, "static", "binaries")
```
This resolves correctly no matter which directory you run `uvicorn` from.

### 2.5 Timestamps look wrong on the frontend — "7h ago" for something that just happened

**Cause:** Postgres stores `timestamp without time zone` (naive) values.
If the API serializes that naive datetime straight to JSON without
attaching a UTC marker, a browser running in a non-UTC timezone will
misinterpret the bare string as *local* time, not UTC — producing an
offset error equal to however far your timezone is from UTC.

**Fix:** add an explicit serializer to the response schema that stamps
naive datetimes as UTC before they're sent:
```python
from pydantic import field_serializer
from datetime import timezone

@field_serializer("created_at", "last_seen_at")
def serialize_as_utc(self, dt):
    if dt is None:
        return None
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.isoformat()
```

**Diagnostic tip:** compare the raw value against Postgres's own `NOW()`
in the same query to confirm whether the *backend* thinks the timestamp is
recent (in which case the bug is in serialization/frontend parsing) or
whether it's genuinely stale (in which case the problem is upstream —
heartbeats aren't actually landing).
```sql
SELECT hostname, last_seen_at, NOW() FROM agents WHERE hostname = 'x';
```

---

## 3. Go Agent Problems

### 3.1 `go build` fails with `expected 'package', found 'EOF'`

```
internal\core\event.go:1:1: expected 'package', found 'EOF'
```

**Cause:** an empty (0-byte) `.go` file. Go requires every `.go` file to
start with a `package` declaration, even placeholder files with no other
content. This project's original folder scaffolding included several
empty stub files for later development phases (`event.go`, `module.go`,
files under `internal/modules/*`, `internal/platform/service_*.go`).

**Fix — find and remove all of them in one pass** rather than fixing them
one at a time as the compiler discovers each one:
```powershell
Get-ChildItem -Recurse -Filter *.go | Where-Object { $_.Length -eq 0 }
Get-ChildItem -Recurse -Filter *.go | Where-Object { $_.Length -eq 0 } | Remove-Item
```
Recreate any of these files with a real `package` declaration (at minimum)
whenever you actually reach that phase of development.

### 3.2 Freshly built `.exe` reports "not recognized as the name of a
cmdlet..." when run

**Cause:** almost always because the *previous* `go build` failed (see
3.1) — there's no `.exe` to run, so PowerShell correctly reports the
filename as unrecognized. This isn't really a separate bug; fix the build
failure first, then this resolves itself.

### 3.3 Agent keeps logging `heartbeat failed: server rejected heartbeat:
invalid credentials` in an endless loop, and the server logs a matching
stream of `401 Unauthorized`

This has two different possible causes — check which one applies:

**Cause A — the agent was revoked or deleted server-side, but the process
is still running.** This is expected behavior, not a bug: neither revoke
nor delete stops the actual running process on the endpoint, they only
change/remove the database row. The agent has no way to know this happened
— it just keeps trying, quietly, forever, by design (heartbeat failures
are meant to log-and-retry, never crash).
- If revoked: use the dashboard's "Reactivate" button, or manually:
  ```sql
  UPDATE agents SET status = 'active' WHERE hostname = '<name>';
  ```
  The agent's *next* heartbeat attempt (within ~30s) will succeed
  automatically — no restart needed.
- If deleted: the `agent_id` no longer exists at all — there's nothing to
  reactivate. You must clear the local config and re-register (Cause B
  below).

**Cause B — stale local config pointing at credentials that no longer
exist** (most common after you manually delete an agent's row and then
try to "reconnect" the same physical machine).

**Fix:** stop the running agent process, delete its saved local config
file, then re-run with a **fresh** `--token` (the old one is already
consumed):

Windows:
```powershell
Remove-Item "$env:ProgramData\khemstrix-agent\config.json" -Force
.\khemstrixAgent.exe --server=http://<server-ip>:8000 --token=<new-token>
```

Linux:
```bash
sudo rm /etc/khemstrix-agent/config.json
./khemstrixAgent --server=http://<server-ip>:8000 --token=<new-token>
```

Since no config file exists afterward, the agent's own state-machine logic
in `main.go` treats that as "first-time install" and goes through
`Register()` again from scratch, producing a brand-new `agent_id` and
`api_key`.

### 3.4 Agent suddenly can't reach the server anymore, even though nothing
about the agent itself changed — `heartbeat failed: cannot reach server:
... context deadline exceeded`

**Symptom:** an agent that was working fine (had successful heartbeats
before) suddenly starts failing every single heartbeat with a network-level
error like:
```
heartbeat failed (will retry next cycle): cannot reach server: Post
"http://172.16.104.160:8000/agent/heartbeat": context deadline exceeded
(Client.Timeout exceeded while awaiting headers)
```
`systemctl status` still shows the service `active (running)` — the agent
process itself is healthy, it just can't reach anything at that address.
Restarting the service (`systemctl restart khemstrix-agent`) does **not**
fix it on its own.

**Cause:** the server's IP address changed (e.g. the server machine moved
to a different network, or its DHCP lease renewed with a new address) —
but the agent has the **old** IP permanently baked into `config.json`'s
`"server"` field, from the moment it first registered. Nothing about the
agent watches for or auto-detects a server IP change; it will keep dialing
the same stale address forever, by design, since `config.Load()` finds an
existing config and never re-reads `--server` from a fresh command-line
invocation once it's already registered.

**Why re-running the one-liner does NOT fix this:** `run.go` only uses the
`--server`/`--token` flags when **no local config exists yet**. Since a
config file already exists (from the original registration), any new
`--server` value passed on the command line is silently ignored — the
agent will keep loading and using the old IP from disk regardless of what
flag you pass. This applies even if you re-download and re-run the
one-liner fresh; the presence of the old `config.json` alone is what
causes it to skip registration entirely.

**Fix — edit the saved config directly, no reinstall/re-registration
needed:**

Linux:
```bash
sudo systemctl stop khemstrix-agent
sudo nano /etc/khemstrix-agent/config.json   # update the "server" field
sudo systemctl restart khemstrix-agent
```

Windows:
```powershell
Stop-Service khemstrix-agent
notepad "$env:ProgramData\khemstrix-agent\config.json"   # update "server"
Restart-Service khemstrix-agent
```

Only the `"server"` value needs to change — leave `agent_id` and `api_key`
untouched, since those still identify a valid, already-registered agent.
This preserves the agent's history in Postgres (same `agent_id`
throughout) rather than creating a duplicate "new" agent, which a full
uninstall-and-re-enroll would do instead.

**Confirm the fix worked** — wait ~30–90s after restarting, then check:
```bash
cat /etc/khemstrix-agent/state.json
```
`last_success_at` should show a fresh, advancing timestamp with no
`last_error` key present. The dashboard's "Offline" badge will clear on
its own shortly after — it just reflects `last_seen_at` in Postgres, which
updates automatically on the next successful heartbeat, no dashboard
action needed.

**Known gap, not yet built:** there is currently no command or mechanism
to update the server address centrally — it must be hand-edited on every
affected machine, individually, exactly as above. If the server's IP
changes often (e.g. frequent lab/VM testing across different networks),
consider giving the server machine a static/reserved IP to avoid hitting
this repeatedly. A future `khemstrix-agent set-server <url>` subcommand,
or a small always-reachable "redirect" endpoint the agent checks before
each heartbeat, would remove the need for manual file edits — noted as a
possible improvement, not yet implemented.

---

## 4. Cross-Machine / VM Testing Problems

### 4.1 Install command downloads/runs, but tries to reach `localhost` —
fails silently or connects to the wrong thing

**Symptom:** the generated one-liner in `GenerateTokenModal.jsx` shows
`--server=http://localhost:8000`, even though you're about to run it on a
completely different machine.

**Cause:** `VITE_API_URL` isn't set in `dashboard/.env`, so `API_URL`
falls back to `http://localhost:8000`. That default is correct for the
dashboard's *own* browser-side API calls (when viewed on the same machine
as the server) but wrong for an install command meant to run somewhere
else — on the target machine, `localhost` means "call yourself," not "call
the real server."

**Fix:** set `VITE_API_URL` explicitly to the server's real,
network-reachable address:
```
VITE_API_URL=http://<server-lan-ip>:8000
```
Then fully restart `npm run dev` (env changes need a restart, not just a
save). This same variable becomes the real public domain
(`https://api.yourdomain.com`) at actual deployment — no code changes
needed at that point, just this one value.

### 4.2 VM can't reach the host server at all — connection refused / times
out

**Checklist, in order:**
1. Is `uvicorn` running with `--host 0.0.0.0`? (See §1.1 — this is the
   most common cause.)
2. From inside the VM, can you `ping <host-ip>` at all? If not, this is a
   VM networking configuration issue (VM network mode — bridged vs. NAT —
   or a firewall on the host), not an application bug.
3. Confirm you're using the *correct* IP — the address of the network
   adapter your VM software creates (e.g. a "VMnet" adapter), not your
   host's Wi-Fi/Ethernet IP. Check with `ipconfig` on the host and look for
   the adapter your hypervisor manages.

### 4.3 New Windows/Linux binary flagged by antivirus (e.g. VirusTotal
shows a handful of detections)

**This is expected and not a sign of a real problem**, as long as the
detections are generic/heuristic (e.g. names like `Wacatac.C!ml` or
similarly ML-flagged, rather than a specific named malware family matched
by multiple major engines). Unsigned, freshly-compiled, zero-reputation
binaries that behave like background monitoring agents (persistent
process, outbound network calls, system info collection) commonly trigger
a small number of heuristic false positives — this is a known, industry-
wide pattern, not unique to this codebase. It doesn't block testing.
Code-signing is the real, standard fix — but it's a pre-production
concern, not something to chase down during development.

---

## 5. Dashboard Problems

### 5.1 Every agent shows "Offline" regardless of actual status

**Cause:** `StatusBadge` expects a `state` prop (one of `"online"` /
`"offline"` / `"revoked"`), computed by `getAgentState(agent)` in
`agentStatus.js`. If the calling component instead passes a boolean on a
differently-named prop (e.g. `online={isAgentOnline(agent)}`), the badge
component receives no usable `state` at all and silently falls back to
its offline styling — with no error, since this is just an unused prop in
JavaScript/React, not a crash.

**Fix:** make sure the calling component does:
```jsx
<StatusBadge state={getAgentState(agent)} />
```
not a differently-shaped prop.

**Also worth checking first, before assuming this is a code bug:** an
agent genuinely showing "Offline" is often just an accurate report — see
§3.4 above for a real-world case where "Offline" was completely correct
because the agent's saved server address had gone stale after the
server's IP changed. Confirm the agent is actually heartbeating
successfully (check `state.json` on the endpoint itself) before assuming
the dashboard's status logic is wrong.

### 5.2 Page content doesn't fill the full browser width

**Cause:** a `max-w-*` Tailwind class (e.g. `max-w-6xl`) on the main
content wrapper, left over from an earlier layout iteration, capping the
content area at a fixed pixel width regardless of actual window size.

**Fix:** remove the `max-w-*` class from the wrapping element (e.g.
`AppShell.jsx`'s `<main>`). A `flex-1` class alone is sufficient to make
the content area fill all space next to a fixed-width sidebar, at any
window size.

---

## 6. General Debugging Habits Worth Keeping

- **When in doubt about whether something was actually persisted, query
  Postgres directly.** `/docs` and API responses can look correct while
  hiding a real gap in the ORM layer (see §2.2).
- **Compare backend-reported timestamps against `NOW()` in the same
  query** when something's timing looks wrong — this immediately tells you
  whether the bug is in the backend (timestamp is genuinely stale) or in
  how the frontend interprets it (timestamp is actually fine, display is
  wrong).
- **When a Go build fails on one empty file, check for others before
  fixing them one at a time** — `Get-ChildItem -Recurse -Filter *.go |
  Where-Object { $_.Length -eq 0 }` finds every empty stub in one pass.
- **Revoking or deleting an agent server-side never stops the actual
  process on the endpoint.** If you need a physical machine to stop
  heartbeating, you have to act on that machine directly (kill the
  process, clear its config, or uninstall) — the dashboard only ever
  controls the database record.
- **On the agent side specifically, treat `journalctl -u khemstrix-agent`
  (Linux) / the Windows Event Log (Windows) as the first source of truth,
  and `state.json` as the second.** The systemd/SCM status only tells you
  the *process* is alive, not that it's successfully reaching the server —
  those are genuinely different things that can disagree (see §3.4). Always
  check both `systemctl status` (process alive?) and `state.json`
  (actually connecting?) before concluding something's fixed or broken.
- **When an agent that used to work suddenly can't reach the server,
  suspect the server's address before suspecting the agent's code.** A
  changed server IP produces symptoms (`context deadline exceeded`) that
  look identical to a firewall or VM networking problem — check whether
  the server's IP actually changed first (§3.4) before re-diagnosing VM
  networking from scratch (§4.2).