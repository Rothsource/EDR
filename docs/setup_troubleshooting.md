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

**When testing WebSocket reconnection/reliability specifically (§3.6
below), you will be repeatedly killing (Ctrl+C) and restarting this exact
command.** Keep it in its own dedicated terminal window that you don't use
for anything else, so "kill the server" is always just one focused Ctrl+C
away rather than hunting through scrollback in a shared terminal.

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

**Building the WebSocket test harness tools** (`testpush`, `testreconnect`,
`dumpoutbox` — see §3.6 and §3.7 below) follows the same pattern, one
binary per `cmd/` subfolder:
```powershell
go build -o testpush.exe ./cmd/testpush
go build -o testreconnect.exe ./cmd/testreconnect
go build -o dumpoutbox.exe ./cmd/dumpoutbox
```
These are development tools only — they exist to exercise the outbox and
WebSocket pipeline directly, without needing the full agent runtime or a
real collector generating events. Keep them out of anything you'd ship to
a customer.

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
The same rule applies to WebSocket event ingestion — a clean log line on
the agent side proves a push was *attempted*, not that it landed. Confirm
with `SELECT * FROM events ORDER BY created_at DESC LIMIT 5;` (or the
`dumpoutbox.exe` tool, from the agent's side — see §3.7) rather than
trusting console output alone.

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

### 2.6 `dial tcp <ip>:8000: connectex: No connection could be made because the target machine actively refused it` vs. `context deadline exceeded` — these mean different things, don't treat them the same

**Symptom:** a WebSocket/HTTP connection attempt from the agent fails, and
the exact wording of the error matters for diagnosis:

- **`context deadline exceeded` / `Client.Timeout exceeded while awaiting
  headers`** — the network path is fine; a SYN reached *something*, but
  nothing responded in time. Usually means: server process is running but
  overloaded/hung, or you're pointed at an IP that's reachable but nothing
  is listening on that specific port, or a firewall is silently dropping
  packets (rather than rejecting them) somewhere along the path.
- **`connectex: ... actively refused it`** — the opposite: the TCP
  handshake completed and the target machine explicitly said "nothing is
  listening here." This means the server process is **not running at
  all** on that host/port right now. Don't go looking for a firewall or
  network issue — go start (or restart) the server first.

**Fix:** treat "actively refused" as your first and cheapest diagnostic
step whenever a previously-working agent suddenly can't connect — before
assuming a networking or firewall change, just confirm the server process
is actually up:
```powershell
# on the server machine
Get-Process | Where-Object { $_.ProcessName -like "*uvicorn*" -or $_.ProcessName -like "*python*" }
```
Only move on to VM/network diagnosis (§4.2) once you've confirmed the
server process itself is genuinely running and bound correctly.

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

**If `Stop-Service`/`Restart-Service` can't find the service at all**
(`Cannot find any service with service name 'khemstrix-agent'`), see §3.8
below before assuming this fix doesn't apply to your situation — the
service registration itself may be missing, which is a different, prior
problem you need to resolve first.

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

### 3.5 Windows binary downloaded/built fine, but running it fails with
"The specified executable is not a valid application for this OS platform"

**Symptom:**
```
Program 'khemstrixAgent.exe' failed to run: The specified executable is
not a valid application for this OS platform.
```
The file exists, has the right name and `.exe` extension, downloaded or
copied without error — it just won't launch.

**Cause:** the file is not actually a Windows PE executable, despite its
name. Go builds for whatever `GOOS`/`GOARCH` are currently set in the
shell — it does **not** infer the target platform from the output
filename. If a Linux build (`GOOS=linux go build -o khemstrixAgent`) was
run and then, in the same or a related terminal/CI step, a build intended
for Windows used the same base name with `.exe` appended without
explicitly re-setting `GOOS=windows` first, the result is a Linux ELF
binary wearing a Windows filename. Go builds it successfully and produces
a file — there's no error at build time, since Go has no way to know the
filename implies a platform mismatch.

**Confirm this is the cause:**
On a Linux/WSL machine (or via `file` if available):
```bash
file khemstrixAgent.exe
```
`ELF 64-bit LSB executable` confirms it's actually a Linux binary. A
correct Windows binary reports `PE32+ executable (console) x86-64, for MS
Windows`.

On Windows itself (no `file` command available), check the first two
bytes — every valid Windows PE binary starts with `MZ`:
```powershell
Get-Content .\khemstrixAgent.exe -Encoding Byte -TotalCount 2 | ForEach-Object { [char]$_ }
```
Should print `MZ`. Anything else means it's not a valid Windows
executable.

**Fix — rebuild explicitly, setting `GOOS`/`GOARCH` in the same command
as the build** rather than exporting them separately and relying on
remembering to reset:
```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o khemstrixAgent.exe .\cmd\agent
```
Re-verify with the `MZ` check above before distributing it. Then replace
the bad file in `server/app/static/binaries/khemstrixAgent.exe`.

**Prevention:** always build both platform binaries as fully separate,
explicit commands rather than relying on a previously-set environment
variable from earlier in the session:
```powershell
GOOS=windows GOARCH=amd64 go build -o khemstrixAgent.exe ./cmd/agent
GOOS=linux GOARCH=amd64 go build -o khemstrixAgent ./cmd/agent
```

### 3.6 Testing WebSocket reliability: the actual step-by-step (durable
delivery, reconnection, reconciliation, duplicate protection)

This is the practical companion to the reliability *design* covered in
`architecture.md` §4 — the concrete steps that actually exercise it,
confirmed to work during real testing.

**Single-event durable delivery + reconnect, minimal version:**
1. Start the server normally (§1.1).
2. Run `.\testpush.exe` — it registers/authenticates using the agent's
   saved config, pushes one synthetic event, and watches for ~15 seconds
   for an ack.
3. Confirm it landed: `.\dumpoutbox.exe` should report `outbox is empty —
   no unacked events remain`.

**Testing an actual outage (the important one):**
1. Run `.\testpush.exe` (or `.\testreconnect.exe`, which is built
   specifically for this and prints an explicit prompt telling you when to
   kill the server).
2. **Kill the server** (Ctrl+C in its terminal) — do this *before* the
   test tool's watch window closes.
3. Confirm the event is stuck locally: `.\dumpoutbox.exe` should now show
   `outbox has 1 row(s)` (or however many you pushed), each still at
   `pending` or `sent_unacked` status.
4. **Restart the server.**
5. Wait a few seconds, then re-check: `.\dumpoutbox.exe` should now report
   empty again — the agent reconnected on its own and the queued event(s)
   reconciled without any manual restart of the agent itself.

**Testing a larger backlog** (closer to a real extended outage than a
single event):
1. Kill the server.
2. Fire off several pushes in a row while it's down:
   ```powershell
   for ($i=1; $i -le 15; $i++) { .\testpush.exe }
   ```
3. `.\dumpoutbox.exe` should show all of them queued.
4. Restart the server, wait, re-check — should clear to empty.

**Confirming duplicate protection actually held**, after any of the above:
```sql
SELECT event_id, COUNT(*) FROM events GROUP BY event_id HAVING COUNT(*) > 1;
```
Zero rows returned means no duplicates were created, even if an event was
retried multiple times across a reconnect.

**Testing multi-agent isolation:** run the same test tools from a second,
independently registered agent (e.g. the Linux cross-compiled binary from
§1.3, registered with its own enrollment token so it gets its own
`agent_id`/`api_key`), and confirm in Postgres that each agent's events
carry the correct, distinct `agent_id` with nothing crossed over:
```sql
SELECT event_id, agent_id, hostname, created_at
FROM events
ORDER BY created_at DESC
LIMIT 20;
```

### 3.7 `dumpoutbox.exe`/reading `outbox.db` directly — don't trust a raw
`Get-Content` on the `.db`/`.db-wal` files

**Symptom:** you try to peek at the outbox without the `dumpoutbox` tool,
e.g.:
```powershell
Get-Content C:\ProgramData\khemstrix-agent\outbox.db-wal | Select-String "pending|sent_unacked"
```
This *can* show readable JSON fragments (SQLite's WAL format is partially
plain-text for TEXT columns), but it's unreliable while the agent process
has the database open — SQLite in WAL mode keeps recent writes in the
`-wal` file and only periodically checkpoints them into the main `.db`
file, and a live connection can hold pages in a state that doesn't match
what a naive text scan shows you. You may see stale rows that have
actually already been deleted, or miss rows that exist but haven't been
flushed to a place a text scan can see.

**Fix:** use `dumpoutbox.exe` (or an equivalent that opens the DB properly
through SQLite's own driver) instead of reading the raw file. If you don't
have `sqlite3` installed and don't have a purpose-built dump tool either,
a quick one-off in Python works too, since Python's standard library
includes `sqlite3`:
```powershell
python -c "import sqlite3; c=sqlite3.connect(r'C:\ProgramData\khemstrix-agent\outbox.db'); print(c.execute('SELECT event_id, status FROM outbox').fetchall())"
```
Installing the SQLite CLI properly is worthwhile if you'll be checking
this often:
```powershell
winget install SQLite.SQLite
```

### 3.8 Windows service for the agent (`khemstrix-agent`) intermittently
can't be found by `Get-Service`/`Restart-Service`/`sc.exe`, even right
after confirming it exists

**Symptom:** `Get-Service *khemstrix*` returns the service, showing
`Running` — but moments later, `Restart-Service khemstrix-agent` (or
`sc.exe query khemstrix-agent`) reports `Cannot find any service with
service name 'khemstrix-agent'`, and a subsequent `Get-Service *khemstrix*`
now returns nothing at all. Nothing about the commands you ran should have
removed it.

**What this actually means:** if `Get-Process` also stops showing the
agent process around the same time, the **process crashed or exited on
its own** — check the System event log for the definitive answer before
troubleshooting PowerShell syntax or permissions:
```powershell
Get-EventLog -LogName System -Source "Service Control Manager" -Newest 20 |
  Where-Object { $_.Message -like "*khemstrix*" }
```
A line like `The Khemstrix EDR Agent service terminated unexpectedly`
confirms the service really did stop running — this isn't a lookup/naming
problem, the service is genuinely gone from the SCM until something
restarts it.

**Immediate workaround — run the executable directly instead of via the
service**, while you investigate why the service died:
```powershell
& "C:\Program Files\khemstrix-agent\khemstrixAgent.exe"
```
This runs it in the foreground of your current terminal, which also has
the benefit of showing you its logs directly rather than needing the
Event Log — often enough on its own to reveal why it had been
crashing as a service (e.g. a bad config value, a missing permission the
service account didn't have but your interactive session does).

**Root-causing the actual crash** is the real fix and is specific to
whatever the agent's own error output says — this section only covers
recognizing *that* it crashed rather than chasing a phantom
"service not found" naming issue. Once you've identified and fixed the
underlying cause, re-register the service (however your install script
does this — typically an admin-elevated
`khemstrixAgent.exe install`/`sc.exe create` step) rather than continuing
to run it manually going forward.

### 3.9 `wsclient: connection attempt ended: read failed: failed to get
reader: use of closed network connection` right after a normal, successful
test push

**Symptom:** `testpush.exe` logs this immediately after `pushed —
watching for ack/reconcile activity for 15s`, and it looks alarming, but
`dumpoutbox.exe` confirms the event was actually delivered and acked (the
outbox is empty afterward).

**Cause:** this is very likely just the test tool's own WebSocket
connection closing cleanly after its single test event is confirmed,
logged at a level that makes it look like a failure rather than expected
teardown. Confirmed non-fatal by checking the outbox immediately after —
if it's empty, the event went through fine despite this log line.

**When to actually worry:** if this message appears but `dumpoutbox.exe`
still shows the event stuck at `pending`/`sent_unacked` afterward, *that*
combination is a real problem worth digging into (the connection is dying
before the ack round-trip completes, not just during cleanup after it).
Always check the outbox state before deciding whether a log line like this
is signal or noise.

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
4. Distinguish "actively refused" from "timed out" (§2.6) — they point at
   different problems (server not running, vs. genuine network path
   issue) and chasing the wrong one wastes time.

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

## 6. Windows Endpoint Problems (agent host machine, not the dev machine)

### 6.1 Can't save edits to `config.json` under `ProgramData` — Notepad
opens it fine but Save silently does nothing, or errors with "Access is
denied"

**Symptom:** `C:\ProgramData\khemstrix-agent\config.json` opens for
viewing normally (double-click, or File → Open in a regular Notepad
window), but editing and saving fails — either silently, or with an
explicit permissions error.

**Two independent causes, check both:**

**Cause A — `ProgramData` is an admin-protected folder.** Standard user
write access to files under `C:\ProgramData\` is restricted by default.
A non-elevated Notepad (which is what you get by double-clicking the file
in Explorer, or launching Notepad normally) can read the file but Windows
blocks the write-back.

**Fix — run Notepad elevated:**
```powershell
Start-Process notepad "$env:ProgramData\khemstrix-agent\config.json" -Verb RunAs
```
Accept the UAC prompt. This opens the file directly, pre-elevated, so
Ctrl+S works normally from there.

**Alternative (GUI-only, no PowerShell needed):** Start Menu → search
`Notepad` → right-click the result → **Run as administrator** → accept
UAC → then use **File → Open** inside that elevated window to browse to
the file. Double-clicking the file itself from Explorer will still open a
non-elevated instance and fail to save, regardless of this fix — the
elevation has to happen at the point Notepad itself launches.

**Permanent fix, so double-click works normally afterward:** grant your
user account **Modify**/**Write** permissions on the folder directly —
right-click `C:\ProgramData\khemstrix-agent` → Properties → Security tab →
Edit... → select your account (Add... if not listed) → check **Modify**
and **Write** under Allow → OK. After this, no elevation is needed for
future edits.

**Cause B — the file is locked by the running agent process.** If
`khemstrixAgent.exe` is currently running (foreground terminal, background
process, or — once built — as a service), it may hold the config file open
in a way that blocks external writes, depending on how the agent reads it.

**Fix — stop the process before editing, restart after:**
```powershell
Stop-Process -Name khemstrixAgent -Force -ErrorAction SilentlyContinue
Get-Process khemstrixAgent -ErrorAction SilentlyContinue   # should return nothing — confirms it's stopped
# ... edit and save config.json here (with elevated Notepad, per Cause A) ...
& "$env:ProgramData\khemstrix-agent\khemstrixAgent.exe"    # no flags needed — reloads from the edited config
```

**Diagnostic tip:** if save fails with no visible error at all (rather
than an explicit "Access is denied" dialog), suspect Cause A first — a
silent failure is Notepad's typical behavior when Windows blocks the
write due to permissions, rather than a file lock, which usually surfaces
a more explicit "being used by another process" message.

**If you also need to restart the agent afterward and it's registered as
a Windows service rather than run manually,** see §3.8 above first if
`Restart-Service`/`Stop-Service` can't find it — don't assume the service
name is wrong before checking whether the process actually crashed.

---

## 7. General Debugging Habits Worth Keeping

- **When in doubt about whether something was actually persisted, query
  Postgres directly.** `/docs` and API responses can look correct while
  hiding a real gap in the ORM layer (see §2.2). The same applies to
  WebSocket event delivery — `dumpoutbox.exe` reporting empty and a
  `SELECT COUNT(*) FROM events` matching what you expect are the two real
  sources of truth, not console log lines.
- **Compare backend-reported timestamps against `NOW()` in the same
  query** when something's timing looks wrong — this immediately tells you
  whether the bug is in the backend (timestamp is genuinely stale) or in
  how the frontend interprets it (timestamp is actually fine, display is
  wrong).
- **When a Go build fails on one empty file, check for others before
  fixing them one at a time** — `Get-ChildItem -Recurse -Filter *.go |
  Where-Object { $_.Length -eq 0 }` finds every empty stub in one pass.
- **Never trust a binary's filename to confirm its platform.** Go builds
  for whatever `GOOS`/`GOARCH` are currently set, regardless of the output
  filename you choose — always verify with the `MZ`-header check (Windows)
  or `file` (Linux) before distributing, especially after cross-compiling
  both platforms in the same terminal session (see §3.5).
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
- **On Windows endpoints, a config edit that "doesn't save" is almost
  always permissions, not corruption.** Check elevation first (§6.1)
  before assuming the file itself or the agent's config-parsing logic is
  broken.
- **"Actively refused" and "timed out" are different diagnoses — don't
  treat them as interchangeable network errors.** Refused means nothing is
  listening (go check the server process); timed out means something
  didn't respond in time (go check the network path or an overloaded
  process). See §2.6.
- **A service that "can't be found" moments after `Get-Service` showed it
  running almost certainly crashed, not renamed itself.** Check the System
  event log before assuming a PowerShell syntax or permissions issue —
  see §3.8.
- **When testing WebSocket reliability, always confirm the actual outcome
  in the outbox and/or Postgres, never just the console log line.** A log
  message that looks like an error (§3.9) can be harmless cleanup, and a
  log message that looks successful can still leave an event stuck — the
  outbox/database state is the only real signal.