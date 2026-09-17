# KhemStrix EDR — WebSocket Durable Delivery: Full Status Report
**Originally September 15, 2026 — updated September 17, 2026 with multi-agent and larger-backlog testing results**

---

## 1. What Is This Project, Briefly

KhemStrix EDR is an Endpoint Detection & Response (EDR) system for small-to-medium businesses in Cambodia. An "agent" (a small program) runs on each employee's computer, watches for suspicious activity (phishing emails, malware, brute-force login attempts), and reports what it sees back to a central server, where it's stored and analyzed.

This report covers one specific piece of that system: **how events get from the agent to the server reliably**, even when the network connection is unstable.

---

## 2. What Is a WebSocket? (Explained Simply)

Before the problem and solution make sense, it helps to understand the tool at the center of this work.

### The old way: REST (like sending letters)

Most web communication works like mailing a letter. Your computer (the "client") sends a request to a server — "here's an event that happened" — the server replies "got it" — and then the connection closes completely. If you have ten events, you send ten separate letters, each with its own trip there and back.

This is called **REST**, and it's how the agent used to work: it collected events for up to 20 seconds (or until it had 50 of them), then sent them all in one batch.

**The problem with this**: if something urgent happens — like ransomware starting to encrypt files — waiting up to 20 seconds before even telling the server about it is far too slow. Real damage can happen in that window.

### The new way: WebSocket (like a phone call)

A **WebSocket** is different. Instead of sending separate letters, the client and server **open one continuous connection and keep it open**, like a phone call. Once connected, either side can talk to the other *at any moment*, instantly, without setting up a new connection each time.

Think of it like this:
- **REST** = texting someone, one message at a time, and waiting for a reply before sending the next.
- **WebSocket** = calling someone and staying on the phone, so you can both talk back and forth instantly, whenever something comes up.

For KhemStrix, this means: the moment the agent detects something suspicious, it can tell the server **immediately** — no 20-second delay, no waiting to batch things up.

### The catch: phone calls can drop

Here's the tradeoff. A letter, once mailed, is out of your hands — the postal system deals with delivering it. But a phone call can just... drop. Wifi hiccups, a laptop goes to sleep, someone closes their laptop lid, the server itself restarts — any of these instantly kills a WebSocket connection, with **zero warning**.

And when that happens, whatever you were "saying" at that exact moment — the event you were trying to send — can simply vanish, unless you've specifically built something to prevent that.

**This is the actual engineering problem this whole report is about**: making sure that when the "phone call" drops, nothing important gets lost.

---

## 3. The Problem, Precisely

The agreed starting point: replace slow batching with an always-on WebSocket connection, so urgent events reach the server instantly.

But a WebSocket doesn't degrade gracefully — it just dies. So the real question this phase of work had to answer was:

> **What happens to an event if the connection drops at the exact moment we're trying to send it?**

Without an answer to that question, moving to WebSockets would trade "sometimes slow" for "sometimes silently loses data" — completely unacceptable for a security product, where a missed detection is exactly the kind of failure that matters most.

---

## 4. The Solution — How We Solved It

The design has several pieces working together. Here's each one, explained plainly:

### 4.1 A durable local "outbox" (the safety net)

Every event, the instant it's created, gets written to a small local database file on the endpoint's own hard drive (using SQLite — a lightweight, file-based database) — **before** the agent even tries to send it anywhere.

Think of it like writing a note in a notebook *before* you try to call someone to tell them the news. If the call drops, the note is still sitting in your notebook. You haven't lost anything — you just haven't delivered it yet.

The event only gets erased from this notebook once the server explicitly confirms "yes, I received it." Not when it's sent — only when it's *confirmed*.

### 4.2 What happens when the connection drops

Nothing special. That's the whole point. Because the event was already safely written to the local notebook the instant it was created, a dropped connection just means "the notebook has an unsent note in it." Nothing is lost — it's just waiting.

### 4.3 Reconnecting and catching up ("reconciliation")

When the connection comes back, the agent doesn't just blindly resend everything in its notebook — that could be wasteful and sometimes send duplicates. Instead:

1. The agent sends the server a list: "here are the IDs of everything in my notebook."
2. The server checks its own records and replies: "I already have these ones."
3. The agent then resends only the ones the server said it didn't have.

This is one efficient back-and-forth, not one round-trip per event.

### 4.4 The real safety net underneath all of this

Even if the reconciliation step above had a bug, or got skipped somehow, there's a deeper safety net: the server refuses to insert the same event twice, no matter how many times it receives it (this is called an "idempotent insert," keyed on a unique event ID). So even a duplicate, accidentally-resent event is completely harmless — it's just ignored the second time.

### 4.5 The heartbeat (how anyone knows the phone call is still connected)

Every 10 seconds or so, the server "pings" each connected agent — like saying "you still there?" — and the agent just replies "yep" and resets a timer. If the agent doesn't hear a ping for too long (about 20-30 seconds), it assumes the call has dropped, and switches into "just write everything to the notebook, don't try to call" mode until it reconnects.

### 4.6 Reconnecting without causing a stampede

If the *server* is the one that went down (not just one agent's wifi), then when it comes back online, potentially **every single agent** in the whole company tries to reconnect at the exact same moment. That flood of simultaneous connections could overwhelm the server right as it's trying to recover.

The fix: each agent waits a random, slightly different amount of time before retrying (called "jitter"), so reconnections spread out naturally instead of all landing in the same instant.

---

## 5. What We Have Actually Done and Verified

This section only lists things that have been **built and tested against a real, running system** — not just designed or written.

### 5.1 Server side — ✅ Fully built and tested

| Piece | What it does |
|---|---|
| Message formats (`schemas/ws.py`) | Defines the exact shape of every message sent back and forth |
| Connection tracker (`ws_manager.py`) | Keeps track of which agents are currently connected |
| The WebSocket endpoint itself (`routers/ws.py`) | Handles login, receiving events, sending confirmations, and reconciliation |
| Heartbeat loop (`ws_heartbeat.py`) | Pings every connected agent, disconnects unresponsive ones |

**Confirmed by real testing**, not just by reading the code:
- Logging in with correct credentials works; bad credentials are correctly rejected.
- The heartbeat keeps a connection alive, and correctly disconnects one that goes silent.
- Sending an event results in it being saved to the database and a confirmation being sent back — verified by directly querying the database and seeing the row.
- Reconciliation correctly reports back only the events the server actually has, ignoring a fake ID we tested it with.
- Sending the same event twice does not create a duplicate.

One real bug was found and fixed along the way: the database stores times without time-zone information, but the incoming timestamps included one, which the database driver rejected. Fixed by stripping the time zone before saving. Worth remembering for the agent's own time-handling too.

### 5.2 Agent side (the Go program running on each machine) — ✅ Fully built and tested

| Piece | What it does |
|---|---|
| The local "notebook" (`internal/store/store.go`) | Saves events to a local file before sending, deletes them only once confirmed |
| The WebSocket client (`internal/wsclient/wsclient.go`) | Connects, logs in, sends events, handles the heartbeat, reconnects automatically with the "avoid a stampede" delay, and reconciles on reconnect |

**Confirmed by real, hands-on testing** (using two small temporary test programs built specifically to exercise this code, since no real detection module exists yet to naturally trigger it):

- **The full happy-path round trip**: an event was created, written to the local notebook, sent live over the WebSocket, saved by the server, confirmed back to the agent, and correctly erased from the local notebook — all steps verified directly (by inspecting both the local file and the database).
- **The actual disconnect/reconnect scenario** — the most important test of all: the server was deliberately shut down, and an event was created *while it was down*. We confirmed the event sat safely in the local notebook, untouched, for as long as the server stayed down (even confirmed a second event queued up the same way during the same outage). Once the server came back online, the agent automatically reconnected, reconciled, resent both queued events, got them confirmed, and the local notebook ended up completely empty again — with the database showing exactly two rows, no duplicates.

This was a genuine, deliberate test of a real outage — not a lucky coincidence — and it worked exactly as designed on the first properly-controlled attempt.

### 5.3 New as of September 17: larger backlogs and a second, independent agent

Section 6 of the original version of this report listed "only tested with one single agent" as an open gap. That is now partially closed — here's exactly what was and wasn't tested.

**Larger backlog, single agent:** instead of one or two queued events, 15 events were pushed in rapid succession while the server was deliberately down. All 15 queued locally without loss, and — critically — **all 15 reconciled and cleared automatically** once the server came back, with no manual intervention beyond restarting the server itself. This is a meaningfully larger test than the original single-event and two-event cases.

**A second, independently registered agent:** a Linux binary was cross-compiled from the same Go source (`GOOS=linux go build`) and run on a separate machine (Kali), going through the *real* registration flow — its own enrollment token, its own freshly generated `agent_id`/`api_key`, recorded as a genuinely separate row in the `agents` table. This was not a copy of the Windows agent's credentials; it's a second, independent identity.

**What this second agent proved:**
- The same 15-event backlog test, run independently on the Linux agent, behaved identically — all 15 queued during an outage, all 15 cleared on reconnect.
- Both agents' backlogs were confirmed in Postgres to have reconciled within seconds of each other after the same server restart — effectively two agents recovering from the same outage concurrently, not sequentially one-at-a-time.
- Every one of the 30 total events (15 per agent) landed with the **correct, distinct `agent_id`** — no event from the Windows agent was ever attributed to the Linux agent or vice versa, confirmed directly in Postgres, not inferred from logs.

**What this does *not* yet prove**, to be precise about the boundary of what was tested: this confirms **two** agents reconnecting and recovering correctly around the same time, with correct isolation. It does not yet prove the jitter/anti-stampede logic (section 4.6) actually spreads out reconnect timing under load — that specifically needs many agents (tens or hundreds) reconnecting at the exact same instant, which two agents recovering within a few seconds of each other doesn't stress-test. See the corrected Section 7 below for how this changes the priority of that remaining test.

### 5.4 A note on process, for the record

Along the way, several early status summaries turned out to be inaccurate — describing work as "done" when it hadn't been checked against the real files, or testing the wrong machine's credentials without realizing it, or hitting a file-permissions issue that had nothing to do with the actual code. Each of these was caught and corrected before being accepted as true. The lesson worth keeping: a status is not "done" until it's actually been run and observed, not just written down. The September 17 testing continued this discipline — every claim in Section 5.3 above was confirmed directly in Postgres, not assumed from console output.

---

## 6. What Has NOT Been Done Yet

Being equally clear about the gaps matters as much as the wins. **This section has been corrected from the original version** based on the September 17 testing — two items changed.

1. **No real detection module exists yet.** Nothing in production actually calls the "send this event" function — everything verified so far used a temporary throwaway test program, not the real agent. *(Unchanged.)*
2. **The WebSocket client is not wired into the real agent yet.** It has never run as part of `run.go` / the actual agent binary — only inside the separate test programs. *(Unchanged — this remains the single biggest gap.)*
3. **~~Reconnect-storm behavior is untested. We've only ever tested with one single agent.~~ Corrected:** two independently registered agents (Windows + Linux) have now been tested, including simultaneous-ish backlog recovery, with confirmed correct isolation (Section 5.3). What remains genuinely untested is **jitter behavior at real scale** — many agents (tens or hundreds) reconnecting at the exact same instant, which two agents recovering a few seconds apart doesn't exercise.
4. **The local notebook has no size limit yet.** During a very long outage, it could theoretically grow without bound and fill up a disk. Needs a cap-and-alert policy — not urgent, but should not be forgotten. *(Unchanged — 15 events is nowhere near the scale this concern is about.)*
5. **Reconciliation isn't chunked.** If an agent were offline for a very long time with a huge backlog, the "here's my list of unsent IDs" message could get large. 15 events at once reconciled cleanly with no chunking; this hasn't been tested at an order of magnitude closer to the real concern (thousands to hundreds of thousands of events). *(Unchanged in substance, now with a data point showing it's fine at small scale.)*
6. **The event contract mismatch is unresolved.** An earlier design document describes a different event shape (`event_type`, `raw_data`, `extracted_features`) than what's actually implemented and tested (`class_uid`, `category_uid`, etc. plus a generic `data` field). These need to be reconciled before more detection modules get built against either one. *(Unchanged.)*
7. **Multi-tenancy is now confirmed present at the schema level, but not exercised as a real feature.** Every event row observed during testing (both Windows and Linux agents) carried a `tenant_id` column, populated with the same default tenant UUID. This means the *scaffolding* the original Step 4 asked for already exists — but it has only ever been tested with a single tenant. Whether a second, genuinely different `tenant_id` correctly isolates data between two organizations has not been tested. See the corrected Section 7 below.

---

## 7. What We Need to Do Next — In Order

**This section has been corrected from the original version.** Two of the original seven steps were based on assumptions that the September 17 testing has since confirmed or changed — see the notes under Steps 2 and 4 below.

### Step 1: Wire the WebSocket client into the real agent
Right now, everything works only inside disposable test programs. The next concrete step is adding the WebSocket client into `agent/internal/core/run.go`, so it starts up automatically alongside the agent's existing heartbeat loop, using the same saved credentials — making this a real, permanent part of the agent rather than a side experiment. **This remains the top priority and has not changed** — it's the one gap that blocks everything downstream of it, including real telemetry collection.

### Step 2 *(revised)*: Test jitter/reconnect behavior at real scale
The original version of this step said "start several agent instances at once... proving jitter logic works with more than one agent." **That framing is now out of date** — two independent agents have already been tested reconnecting around the same time, with correct isolation (Section 5.3). What's actually still needed is testing at a scale that would meaningfully stress the anti-stampede jitter logic: closer to 10–50+ simultaneous agent instances (can be done as multiple processes on one machine, each with its own registered identity, for now — doesn't require separate physical hardware), stopping the server, bringing it back, and confirming from the logs/timestamps that reconnect attempts spread out over time rather than clustering in the same fraction of a second.

### Step 3: Fix configuration precedence issues
Two small but real cleanup items flagged earlier: remove a hardcoded server address from the Linux startup file, and make sure the agent always trusts its saved configuration file over any command-line flags it might be started with by accident. *(Unchanged — not affected by this week's testing.)*

### Step 4 *(revised)*: Verify multi-tenant isolation, not build multi-tenant support from scratch
The original version of this step said "create the `organizations` table structure, and add a `tenant_id` to the existing tables." **This is now partially done** — every event observed during testing already carries a `tenant_id` column (Section 6, item 7), so the schema-level scaffolding exists. What's actually left is verification, not construction: register two agents under two genuinely different `tenant_id` values (rather than both defaulting to the same one, as in all testing so far) and confirm that a query scoped to one tenant never returns the other's events — proving isolation actually holds, not just that the column exists.

### Step 5: Reconcile the event contract
Decide, once and for all, on a single agreed shape for what an "event" looks like, and update whichever document is wrong (the older design doc or the implementation) so future work isn't built against two different ideas of the same thing. *(Unchanged.)*

### Step 6: Build the first real detection module — email phishing
This is the first genuine, real-world source of events: the agent will connect to an employee's email, look at incoming messages, extract useful signals in memory (never saving the actual email content to disk, per Cambodia's data protection rules), and feed the result into the now-working WebSocket path built and tested above. *(Unchanged — still appropriately last, since it depends on Step 1 being done first.)*

### Step 7: Full real-world verification
Once a real detection module exists, trigger an actual test phishing-style event, and confirm the entire path — from detection, to durable local storage, to WebSocket delivery, to database storage, to threat scoring — works correctly together, not just each piece in isolation. *(Unchanged.)*

---

## 8. How To Test This Yourself (After Pulling From GitHub)

This section is for anyone (like a teammate) who pulls the repo fresh and wants to actually see the durable-delivery system working, not just read about it. It walks through the same steps we used to verify everything in Section 5, now including the larger-backlog and second-agent tests from Section 5.3.

### 8.1 What you need running first

You need **two things running before any test makes sense**:

1. **The server**, with a PostgreSQL database it can reach.
2. **A registered agent identity** — either a real agent, or a throwaway one you register just for testing.

```powershell
# 1. Start the server (from server/app/)
cd server/app
uvicorn main:app --reload --host 0.0.0.0 --port 8000
```

Keep this terminal open and visible — you'll be watching its logs during every test below.

### 8.2 Register an agent (skip if you already have one)

You need a valid `agent_id` + `api_key` saved locally before any WebSocket test will work — the server rejects anyone it doesn't recognize.

```powershell
# Generate a one-time enrollment token (via the dashboard, logged in as an admin user,
# or directly through the /admin/generate-token endpoint)

# Then, from agent/:
go run ./cmd/agent -server http://<server-ip>:8000 -token <the-token-you-generated>
```

This creates `config.json` (on Windows: `C:\ProgramData\khemstrix-agent\config.json`; on Linux: `/etc/khemstrix-agent/config.json`) containing your new `agent_id` and `api_key`. Confirm it worked:

```powershell
Get-Content C:\ProgramData\khemstrix-agent\config.json
```

**Important — file permissions**: if a Windows service has run on this machine before, the files in that folder may be locked to admin-only access. If later steps fail with a "readonly database" error, run this once, from an elevated (Administrator) PowerShell:

```powershell
icacls C:\ProgramData\khemstrix-agent /grant Users:F /T
```

### 8.3 Build the two test tools

These are small, throwaway programs (not part of the real agent) built specifically to exercise the outbox and WebSocket client directly, since no real detection module exists yet.

```powershell
cd agent

# Pushes one fake event and watches for confirmation
go build -o testpush.exe ./cmd/testpush

# Same as above, but waits much longer — gives you time to manually kill the server mid-test
go build -o testreconnect.exe ./cmd/testreconnect

# Prints whatever is currently sitting in the local outbox file, unsent
go build -o dumpoutbox.exe ./cmd/dumpoutbox
```

*(If these `cmd/` folders don't exist in your checkout yet, ask whoever wrote this report for the three `main.go` files — they're intentionally not part of the permanent codebase.)*

### 8.4 Test 1 — The happy path (server stays up the whole time)

```powershell
.\testpush.exe
```

**What you should see:**
- A `DEBUG agent_id=... api_key=... server=...` line confirming which identity it's using
- `pushing test event <uuid>`
- No errors

**Then confirm it actually landed**, in Postgres:
```sql
SELECT event_id, hostname, data, created_at FROM events WHERE hostname = 'testpush-harness' ORDER BY created_at DESC;
```

**And confirm the local notebook is empty again** (meaning it got confirmed and cleaned up):
```powershell
.\dumpoutbox.exe
```
Should print `outbox is empty — no unacked events remain`.

### 8.5 Test 2 — The real test: surviving a dropped connection

This is the test that actually proves the whole point of this design.

**Step 1 — Stop the server completely.** Go to its terminal and press `Ctrl+C`. Confirm it's really down:
```powershell
Test-NetConnection -ComputerName <server-ip> -Port 8000
```
Should show `TcpTestSucceeded : False`.

**Step 2 — With the server confirmed down, run:**
```powershell
.\testreconnect.exe
```
It will try to push an event, fail to reach the server (expected), and then sit waiting for up to 120 seconds.

**Step 3 — While it's still waiting, in a second terminal, check the outbox:**
```powershell
.\dumpoutbox.exe
```
You should see the event still sitting there, unsent — this is the proof that nothing was lost.

**Step 4 — Now restart the server:**
```powershell
uvicorn main:app --reload --host 0.0.0.0 --port 8000
```

**Step 5 — Watch `testreconnect.exe`'s terminal.** Within a few seconds it should reconnect on its own, with no further action from you.

**Step 6 — Once its timer finishes, check the outbox one more time:**
```powershell
.\dumpoutbox.exe
```
Should now say it's empty again — meaning the event that survived the outage was automatically resent and confirmed.

**Step 7 — Confirm in Postgres there's exactly one copy, not two:**
```sql
SELECT event_id, hostname, created_at FROM events WHERE hostname = 'reconnect-test' ORDER BY created_at DESC;
```

If all seven steps check out, you've personally reproduced the exact test that validated this design — not just read that someone else did it.

### 8.6 Test 3 *(new)* — A larger backlog during an outage

Same idea as Test 2, but proving it holds up with more than one queued event.

```powershell
# 1. Kill the server (Ctrl+C in its terminal)

# 2. While it's down, push several events in a row:
for ($i=1; $i -le 15; $i++) { .\testpush.exe }

# 3. Confirm they all queued:
.\dumpoutbox.exe
# should report 15 (or however many you pushed) rows, none acked

# 4. Restart the server

# 5. Wait a few seconds, then check again:
.\dumpoutbox.exe
# should report empty — all of them reconciled

# 6. Confirm no duplicates were created in Postgres:
```
```sql
SELECT event_id, COUNT(*) FROM events GROUP BY event_id HAVING COUNT(*) > 1;
```
Zero rows back confirms duplicate protection held even under a larger backlog, not just a single event.

### 8.7 Test 4 *(new)* — A second, independent agent

This proves agent isolation, not just durable delivery for one agent in isolation.

1. Register a **second** agent identity — either cross-compile the Linux binary and run it on a separate machine/VM, or simply register a second Windows identity on the same machine using a fresh enrollment token (they'll each get their own `config.json` location if run from separate working directories).
2. Repeat Test 3 (the 15-event backlog test) independently for this second agent.
3. Confirm in Postgres that every event from both agents carries the correct, distinct `agent_id`:
```sql
SELECT event_id, agent_id, hostname, created_at
FROM events
ORDER BY created_at DESC
LIMIT 40;
```
Check that events from each agent are attributed correctly and nothing is mixed up.

### 8.8 Common gotchas (things that tripped us up, so you don't repeat them)

- **Run the test tools on the same machine where the agent is actually registered.** `config.json` is local to whatever machine you're on — running the test tool from a different computer (e.g. your host instead of a VM) will read a *different* config file with different (possibly stale or nonexistent) credentials, and fail with a confusing "invalid credentials" error that has nothing to do with the actual code.
- **A leftover Windows service can lock the outbox file.** If you get a "readonly database" error, check `Get-Service | Where-Object { $_.Name -like "*khemstrix*" }` — if one is running, stop it first, and fix folder permissions as shown in 8.2.
- **`--reload` on uvicorn can auto-restart the server on file changes**, which can accidentally interrupt your test at the wrong moment. If you're actively editing files nearby while testing, that restart isn't a real "server crashed" test — do a deliberate `Ctrl+C` instead, and don't touch files mid-test.
- **Double-check which agent's credentials you're actually testing with** if your project has multiple registered agents (e.g. from earlier testing) — a stale or deleted agent's credentials will always fail auth, even though the failure looks identical to a real bug.
- **New (from the September 17 testing): a service that "disappears" from `Get-Service` may have simply crashed, not been renamed or corrupted.** Check the System event log (`Get-EventLog -LogName System -Source "Service Control Manager"`) before assuming a lookup/permissions issue — if the process crashed, run the `.exe` directly first to see its actual error output, rather than fighting with service commands.
- **A `"use of closed network connection"` log line right after a successful test push is very likely harmless connection cleanup, not a real failure** — confirm by checking the outbox; if it's empty, the event went through fine despite the alarming-looking log line.

---

## 9. One-Sentence Summary

**The problem**: switching to instant delivery (WebSockets) risked losing security events whenever a connection dropped. **The solution**: never delete an event until the server confirms it, and automatically catch up on reconnect. **Status as of September 17**: this solution has been proven to survive a real server outage without losing or duplicating events — now confirmed with a larger backlog (15 events) and with two independently registered agents (Windows and Linux) recovering correctly and in isolation from each other — and the next job remains making it a permanent part of the real agent, instead of a test experiment, followed by verifying jitter behavior at real scale and confirming genuine multi-tenant isolation rather than assuming the schema alone guarantees it.