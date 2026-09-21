# KhemStrix EDR — WebSocket Durable Delivery: Full Status Report
**Originally September 15, 2026 — updated September 17 with multi-agent and larger-backlog testing — updated September 18 to confirm real-agent integration, record the new event fields added to Postgres, and lay out the collector roadmap (auth → file → network) — updated September 21 to record the code that carries those new fields through the whole stack — updated again September 21 (later) to record that this code has now been VERIFIED against the live server and database with two agents (Windows + Linux), including an outage and reconnect**

---

## 1. What Is This Project, Briefly

KhemStrix EDR is an Endpoint Detection & Response (EDR) system for small-to-medium businesses in Cambodia. An "agent" (a small program) runs on each employee's computer, watches for suspicious activity (phishing emails, malware, brute-force login attempts), and reports what it sees back to a central server, where it's stored and analyzed.

This report covers one specific piece of that system: **how events get from the agent to the server reliably**, even when the network connection is unstable — and, as of this update, what real detection work is queued up now that the transport layer itself is finished and the new event fields are verified.

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

This section only lists things that have been **built and tested against a real, running system** — not just designed or written. Anything that has been written but not yet run against the real system is labelled as such. **As of the September 21 (later) update, nothing in this section is in the "written but unverified" state** — the September 21 code is now verified (Sections 5.6 and 5.7).

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

One real bug was found and fixed along the way: at the time, the database stored times without time-zone information, but the incoming timestamps included one, which the database driver rejected. Fixed by stripping the time zone before saving. **Superseded September 21:** the `events` table's `time` and `created_at` columns are now `timestamp with time zone`, so the handler no longer strips the time zone — it now passes timezone-aware UTC values instead (see Section 5.6). As far as `models.py` and the earlier docs show, the *other* tables still use timestamps without time zone, so the "strip it" rule still applies to those (see Section 6, item 13).

*(Files touched on September 21: `db/models.py`, `schemas/ws.py`, `routers/ws.py`. The behavior described in this section was verified before those edits, and **was re-confirmed after them on September 21 (later)** — see Section 5.7.)*

### 5.2 Agent side (the Go program running on each machine) — ✅ Fully built and tested

| Piece | What it does |
|---|---|
| The local "notebook" (`internal/store/store.go`) | Saves events to a local file before sending, deletes them only once confirmed |
| The WebSocket client (`internal/wsclient/wsclient.go`) | Connects, logs in, sends events, handles the heartbeat, reconnects automatically with the "avoid a stampede" delay, and reconciles on reconnect |

**Confirmed by real, hands-on testing** (using two small temporary test programs built specifically to exercise this code, since no real detection module exists yet to naturally trigger it):

- **The full happy-path round trip**: an event was created, written to the local notebook, sent live over the WebSocket, saved by the server, confirmed back to the agent, and correctly erased from the local notebook — all steps verified directly (by inspecting both the local file and the database).
- **The actual disconnect/reconnect scenario** — the most important test of all: the server was deliberately shut down, and an event was created *while it was down*. We confirmed the event sat safely in the local notebook, untouched, for as long as the server stayed down. Once the server came back online, the agent automatically reconnected, reconciled, resent the queued events, got them confirmed, and the local notebook ended up completely empty again — with the database showing no duplicates.

This was a genuine, deliberate test of a real outage — not a lucky coincidence — and it worked exactly as designed.

*(`wsclient.go` was deliberately **not** changed on September 21: it treats every event payload as opaque, ready-made JSON, so the new event fields are added by the code that builds the payload, not by the transport. The two test programs were updated — see Section 5.6.)*

### 5.3 September 17: larger backlogs and a second, independent agent

**Larger backlog, single agent:** 15 events were pushed in rapid succession while the server was deliberately down. All 15 queued locally without loss, and **all 15 reconciled and cleared automatically** once the server came back, with no manual intervention beyond restarting the server itself.

**A second, independently registered agent:** a Linux binary was cross-compiled from the same Go source (`GOOS=linux go build`) and run on a separate machine (Kali), going through the *real* registration flow — its own enrollment token, its own freshly generated `agent_id`/`api_key`, recorded as a genuinely separate row in the `agents` table.

**What this second agent proved:**
- The same 15-event backlog test, run independently on the Linux agent, behaved identically.
- Both agents' backlogs reconciled within seconds of each other after the same server restart — two agents recovering from the same outage concurrently, not one-at-a-time.
- Every one of the 30 total events (15 per agent) landed with the **correct, distinct `agent_id`** — no cross-contamination, confirmed directly in Postgres.

**What this did not yet prove**, to be precise: two agents recovering correctly is not the same as proving the jitter/anti-stampede logic (Section 4.6) actually spreads out reconnect timing under real load — that needs many agents (tens or hundreds), not two. Still open — see Section 7.

### 5.4 September 18 — Confirmed: the real agent, not just test tools, is now streaming

This closes what was, until this update, the single biggest open gap in the project.

**The WebSocket client is wired into `run.go`.** The block that opens the local outbox, builds the WebSocket URL, constructs the client, and launches it (`go wsc.Run(ctx)`) now runs as a real part of the agent's startup sequence — alongside the existing heartbeat loop, not instead of it. If the outbox can't be opened or the WebSocket URL can't be built, the agent logs the problem and keeps running its heartbeat loop anyway; streaming failing to start is not allowed to take down the whole agent.

**Confirmed with a packet capture, not just by reading the code:** a Wireshark capture of the actual production binary (`khemstrixAgent.exe`, not a test tool) showed one WebSocket connection staying open for the entire capture window, plus separate ~30-second-interval heartbeat HTTP calls — matching the `heartbeatInterval = 30 * time.Second` constant in `run.go` exactly. This is direct evidence that `wsc.Run(ctx)` is really running inside the shipped agent process.

**Also confirmed while reviewing `run.go`:** the configuration-precedence protection already exists and works as intended — if the agent is started with a `--server` flag that disagrees with its saved `config.json`, the saved config wins and the mismatch is only logged, never silently honored. A stray or stale flag can no longer redirect a running agent's event stream to the wrong server.

**What is still genuinely missing, now that the transport itself is done:** nothing generates a *real* event yet. The connection is live, authenticated, and idly reconciling zero events, because no collector has ever called `Push()` outside of the disposable test tools. This is now the actual next piece of work — see Section 7.

### 5.5 September 18 — Five new fields added to the `events` table

To support what the collectors in Section 7 will need to report, five columns were added to Postgres's `events` table (test data wiped first, since none of it was real):

| Column | Type | Purpose |
|---|---|---|
| `schema_version` | `smallint`, `NOT NULL`, default `1` | Lets every future consumer (backend, detection, dashboard) tell old event shapes apart from new ones without guessing |
| `agent_version` | `text`, nullable | Which build of the agent produced this event — useful once there are real deployed agents in the field, not just test VMs |
| `host_os` | `text`, nullable | `windows` / `linux` — needed because detection logic (e.g. brute-force SSH vs. brute-force RDP) will differ by platform |
| `host_os_version` | `text`, nullable | Finer-grained OS detail alongside `host_os` |
| `ingest_source` | `text`, `NOT NULL`, default `'websocket'` | How the event arrived — everything today comes over the WebSocket, but Phase 5 (email/Telegram) will introduce other paths |

**Update September 21 (later): ✅ verified.** These columns are now populated with real values from real agents on both Windows and Linux (Section 5.7). All four code layers that carry them — the `Event` model, the inbound WebSocket schema, the handler's insert logic, and the agent-side payload builder — are confirmed working against the live database.

### 5.6 September 21 — Code that carries the new fields end to end, and a timezone finding

**Status: ✅ code written, checked offline, and now verified against the live server and database (Section 5.7).**

#### Files changed

| File | What changed |
|---|---|
| `server/app/db/models.py` | The `Event` model now mirrors the real `\d events` output column for column: adds the five new columns, and `time` / `created_at` are now timezone-aware (`TIMESTAMP(timezone=True)`). Unused `Float` import removed; `SmallInteger` added. Other tables left as they were. |
| `server/app/schemas/ws.py` | `WSEvent` gains four **optional** agent-supplied fields: `schema_version` (default 1, allowed range 1–32767), `agent_version` (≤64 chars), `host_os` (≤32 chars), `host_os_version` (≤128 chars). `ingest_source` is deliberately **not** in the schema. |
| `server/app/routers/ws.py` | `_handle_event` now passes the four new fields into the insert, sets `ingest_source="websocket"` itself (never taken from the client), and passes a timezone-aware UTC time instead of stripping the time zone. |
| `agent/internal/event/event.go` *(new)* and `event_test.go` *(new)* | A small shared package with one function, `event.Build()`. It creates the `event_id`, the UTC `time`, computes `type_uid` (`class_uid × 100 + activity_id`), fills in `schema_version`, `agent_version`, `host_os` (from `runtime.GOOS`) and `host_os_version`, defaults `hostname`, and returns `(eventID, payload)` ready for `wsc.Push()`. Every future collector will call this instead of hand-writing JSON. |
| `agent/cmd/testpush/main.go`, `agent/cmd/testreconnect/main.go` | Both now build their event with `event.Build()` instead of a hand-written JSON string, so the tests exercise the new fields. Hostnames stay `testpush-harness` and `reconnect-test`, so the SQL in Section 8 still matches. |
| `agent/internal/wsclient/wsclient.go` | **Unchanged, on purpose** (see Section 5.2). |

#### The timezone finding

The September 18 `\d events` output shows that `events.time` and `events.created_at` are `timestamp with time zone`. The handler used to strip the time zone before inserting (Section 5.1), which was correct for the old column type but is risky for the new one: as far as we know, asyncpg treats a timezone-less datetime sent to a `timestamptz` column as **server local time**, so on a server not running in UTC (Cambodia is UTC+7) every event's `time` could be shifted by the offset. The handler now sends timezone-aware UTC instead, which is correct for both column types.

**Verified September 21 (later):** `time` in Postgres (for example `13:28:30+07`) matches the agent's UTC payload (`06:28:30Z`), and `created_at − time` is a few seconds for live events, not ~7 hours (Section 5.7).

#### Design decisions worth remembering

- **`ingest_source` is server-only.** A client can't claim to have arrived by a different transport; if an agent sends the field, it is ignored.
- **`host_os` is a plain string, not a fixed list.** An event the server rejects is never acknowledged, so it stays in the agent's outbox and is retried on every reconnect. A strict list would turn a future macOS agent into permanently stuck events.
- **`schema_version` has a range check** because the column is a `smallint`; an out-of-range value would otherwise cause a database error inside the handler instead of a clean validation error.
- **Old and new agents/servers interoperate.** The new fields are optional with defaults, and the schema ignores unknown fields, so the server and agent can be updated in either order.
- **`event.Build()` returns the `event_id` together with the payload**, so the outbox key and the `event_id` inside the JSON can never disagree (a requirement `Push()` previously only documented).
- **The agent never sends** `agent_id`, `tenant_id`, `ingest_source` or `created_at` — the server stamps those.

#### Checked offline (before the live run)

- The new Go package builds, passes `go vet` and its four unit tests, and cross-compiles for Windows.
- A payload generated by `event.Build()` was validated against the updated `WSEvent` schema and accepted; an out-of-range `schema_version` and a client-supplied `ingest_source` were confirmed rejected/ignored respectively.
- The updated `models.py` imports cleanly and produces a `CREATE TABLE` matching the `\d events` output.

### 5.7 September 21 (later) — Live verification against the real server and database ✅

This closes the "still to verify against the real system" list from Section 5.6.

#### Pre-flight checks (repo state)

- All eight expected files were present in the repo. One was misnamed: the unit-test file had been saved as `eventest.go` (missing underscore), so Go compiled it as ordinary code instead of running it as tests. Renamed to `event_test.go`; `go test ./internal/event/` then passed (`ok khemstrix-agent/internal/event`).
- Content checks: `models.py` has all five new columns and `timezone=True` on `time` and `created_at`; `schemas/ws.py` has the four agent-supplied fields and mentions `ingest_source` only in a comment; `routers/ws.py` sets `ingest_source="websocket"` itself and converts to UTC instead of stripping the time zone; both test tools call `event.Build`.
- `go vet` and `go build ./...` were clean. The server-side import check (`import db.models, schemas.ws, routers.ws`) passed once run inside the project's virtual environment (`server\venv`); outside it, it fails with `No module named 'asyncpg'`, which is an environment issue, not a code issue.

#### The live test

Two independently registered agents ran `testpush` at the same time against one server: **Windows** and **Linux (Kali)**, each looping 15 pushes. The server was stopped partway through the loops and restarted, so the run included a real outage.

| Check | Result |
|---|---|
| Rows in `events` | **30** — 15 per agent |
| Agents kept separate | ✅ Windows `agent_id` `7dfa82f9…`, Linux `agent_id` `58fd4af7…`, no cross-contamination |
| `schema_version` | `1` on all rows |
| `agent_version` | `0.1.0` on all rows |
| `host_os` | `windows` / `linux`, matching the agent that sent each row |
| `host_os_version` | Windows: empty (expected — not implemented yet). Linux: `Kali GNU/Linux Rolling`, read from `/etc/os-release` |
| `ingest_source` | `websocket` on all rows, set by the server |
| Timezone-aware UTC | ✅ `time` matches the agent's UTC payload; `created_at − time` is seconds for live events, not ~7 hours |
| Outage handling | ✅ 3 events queued in each agent's outbox while the server was down (confirmed with `dumpoutbox`); after the restart all 6 were delivered around 13:34:06–13:34:15, and both outboxes ended empty |
| Duplicates | ✅ `SELECT event_id, COUNT(*) FROM events GROUP BY event_id HAVING COUNT(*) > 1;` returned **0 rows** |

**Conclusion:** the five new `events` columns, the timezone-aware handling, and the reliability guarantees all hold together with two different operating systems at once.

#### Observations from the run

- **This was a partial backlog, not the full 15.** The server went down partway through the loops, so roughly 12 events per agent were delivered live and only 3 per agent queued. It proves the mechanism with the new schema, but it is not a repeat of the 15-event backlog test from Section 5.3. That earlier result still stands.
- **`created_at − time` can be slightly negative** (about −1 second on a few rows). `time` comes from the agent's clock and `created_at` from the server's, so this is clock skew between machines, not a bug. It matters later: detection logic that orders events across machines should not assume perfectly synchronized clocks.
- **The real Kali service was running during the test**, under the same `agent_id` as `testpush-linux`, so two connections with one identity existed at once. This may account for some of the `use of closed network connection` lines. The service was stopped (`systemctl stop khemstrix-agent`) for later runs; no "readonly database" error appeared in this run.
- **Resource use:** the shipped Kali agent service idles at about 19 MB RSS and 0.0% CPU (`ps aux`); the short-lived `testpush` harness stayed under about 4 MB RAM and under 1% CPU. These are idle numbers — re-measure once a real collector is running.
- **The test tool prints the API key.** `testpush` logs a `DEBUG agent_id=... api_key=...` line in plaintext (see Section 6, item 15).

---

## 6. What Has NOT Been Done Yet

Being equally clear about the gaps matters as much as the wins. This section has been corrected four times now — September 17, September 18, and twice on September 21.

1. ~~**The WebSocket client is not wired into the real agent yet.**~~ **Resolved as of September 18** — confirmed via Wireshark capture of the real production binary (Section 5.4). No longer an open item.
2. **No real collector exists yet.** Nothing calls `wsclient.Push()` outside of the disposable `testpush`/`testreconnect` test tools. This is now the single biggest actual gap, and the direct blocker on everything in Section 7 below.
3. ~~**The five new Postgres columns (Section 5.5) are not wired through the rest of the stack.**~~ **Resolved September 21 (later)** — code written and verified against the live database with two agents (Section 5.7).
4. **The event contract is still not finalized in practice.** Every test event so far has used the same placeholder `class_uid`/`category_uid`/`activity_id`/`type_uid`/`severity_id` values (1001/1/1/100101/1). No real event type has been assigned its own values yet — that has to happen before the first real collector is built, not after. *(`event.Build()` now computes `type_uid` automatically, so what remains is choosing the real class/category/activity/severity values for each event type.)*
5. **No TLS.** Traffic is still plaintext `ws://`/`http://`, confirmed via Wireshark — meaning `api_key` and all event data are currently readable on the wire. Fine while testing on a private LAN with synthetic data; must happen before any real endpoint data (real usernames, command lines, IPs) starts flowing.
6. **Jitter/reconnect-storm behavior is still untested at real scale.** Two agents recovering together (Sections 5.3 and 5.7) doesn't stress-test the anti-stampede logic — that needs something closer to 10–50+ simultaneous agents.
7. **The local outbox has no size limit yet.** A very long outage could theoretically let it grow unbounded and fill a disk. Not urgent, but shouldn't be forgotten.
8. **Reconciliation isn't chunked.** Confirmed fine at 15 events; untested anywhere near the scale (thousands+) where a single reconcile message could become unwieldy.
9. **Multi-tenancy exists at the schema level but isn't exercised.** Every event observed so far carries a `tenant_id`, but always the same default value (`00000000-0000-0000-0000-000000000001`) — genuine isolation between two different tenants hasn't been tested.
10. **A minor, low-severity shutdown-ordering note.** A goroutine in `run.go` closes the outbox the instant shutdown begins, running concurrently with the WebSocket client's own goroutines, one of which could still be mid-flight trying to mark an event acked. Not currently causing visible problems, and self-heals via reconciliation on the next reconnect even in the worst case — worth understanding, not urgent to fix.
11. **Windows `host_os_version` is not implemented.** On Linux it is read from `/etc/os-release` (verified: `Kali GNU/Linux Rolling`); on Windows the field is left empty (stored as `NULL`) until a Windows-specific lookup (registry or `RtlGetVersion`) is added. The column is nullable, so nothing breaks.
12. **`events.agent_id` is nullable in Postgres**, even though every event always has an agent. The model was written to match the database as it is. Tightening it is one statement — `ALTER TABLE events ALTER COLUMN agent_id SET NOT NULL;` — followed by adding `nullable=False` to the model.
13. **Only `events` has been compared against `models.py`.** The other tables (`organizations`, `agents`, `enrollment_tokens`, `users`, `alerts`) were left as they were; their real column types haven't been checked against the model since this work began.
14. **Permanently rejected events have no dead-letter handling.** If the server rejects an event as malformed, it sends an error and never acknowledges it, so the event stays in the agent's outbox and is resent after every reconnect. The new length and range limits prevent the known causes, but there is no general "give up after N failures" path.
15. **The test tools print the API key in plaintext** *(new, September 21 later)*. `testpush` and `testreconnect` log a `DEBUG agent_id=... api_key=... server=...` line. Remove or redact it before these tools are shared or run anywhere outside a private test setup. Because the keys for both test agents were displayed during testing and pasted into a shared conversation, consider re-registering those agents to issue fresh keys. This is separate from item 5: removing the log line does not fix the plaintext-on-the-wire problem, only TLS does.
16. **The full 15-event backlog with the new schema has not been re-run** *(new, September 21 later)*. Section 5.7 queued only about 3 events per agent because the server went down mid-loop. To repeat the backlog test properly, stop the server *first*, then run both loops. Optional, since the mechanism is already proven.
17. **Cross-machine clock skew is unaddressed** *(new, September 21 later)*. Agent `time` and server `created_at` can differ by about a second or more (Section 5.7). Detection rules that compare or order events across machines need to tolerate this.
18. **Two connections under one `agent_id` were not deliberately tested** *(new, September 21 later)*. It happened accidentally on Kali (service plus `testpush-linux` at once). The server tolerated it in this run, but how it should behave (reject the second, replace the first, or allow both) has not been decided.

---

## 7. What We Need to Do Next — In Order

**Updated September 21 (later).** Step 1 is now essentially done; what remains of it is housekeeping.

### Step 1: ✅ Verify the five new event fields end to end — DONE (Section 5.7)
Remaining close-out items:
1. Decide whether to tighten `events.agent_id` (Section 6, item 12), and add the Windows `host_os_version` lookup (item 11).
2. Update the two out-of-date documents listed at the end of Section 5.6 (below).
3. Remove or redact the plaintext `api_key` debug line in the test tools (item 15) and consider re-registering the two test agents.
4. Commit: `git add agent/internal/event` (the package is still untracked), and review `docs/setup_troubleshooting.md` — it shows as modified — before including it in the same commit.

#### Documents that are still out of date

- **The architecture guide:** its "Postgres columns here are `timestamp without time zone`" rule is no longer true for `events`; its table snapshot omits `organizations`, `alerts` and the five new columns; and its text still says real-agent integration isn't done (Section 5.4 confirmed it is). It also points at `schemas/event.py` as the inbound event schema, but that file is empty — the real one is `schemas/ws.py`. (`routers/events.py` is also empty, a likely leftover from the old REST design.)
- **`docs/phase2-event-schema.md`:** should list the five new envelope fields and say which are agent-set and which are server-set.

### Step 2: Finalize the event contract, for real events this time
Every test event so far has reused the same placeholder `class_uid`/`category_uid`/`activity_id`/`severity_id` numbers (`type_uid` is computed automatically by `event.Build()`). Before building the first real collector, decide the actual values for a failed-login event specifically — this both resolves the long-standing "event contract not finalized" gap and unblocks Step 3.

### Step 3: Build the first real collector — failed authentication attempts
The first genuine, non-synthetic telemetry source, and the direct successor to "wire the WebSocket client into the agent," which is done. Concretely:
- **Windows:** watch the Security event log for Event ID `4625` (failed logon).
- **Linux:** watch `/var/log/auth.log` (Debian/Ubuntu) or `journalctl` (systemd-based distros) for `sshd` authentication failures.
- Build each event with `event.Build()` — pass the values decided in Step 2, plus the collector's own `data` (attempted username, source IP if available, hostname, failure reason if the OS exposes one). The agent-level fields (`schema_version`, `agent_version`, `host_os`, `host_os_version`), the `event_id`, the UTC time and `type_uid` are filled in automatically.
- Call `wsc.Push(eventID, payload)` with what `Build` returned — the same `wsc` instance already sitting authenticated and idle in `run.go`, confirmed live in Section 5.4.
- Retest the outage scenario from Section 8, but with a real failed-login event this time instead of `testpush`'s placeholder payload.

### Step 4: Add TLS (`wss://`)
Should happen before or immediately alongside Step 3 — once real usernames and IP addresses are flowing over the wire (as they will be, starting with the auth collector), plaintext is no longer an acceptable tradeoff, even on a private LAN.

### Step 5: Repeat the same pattern for file monitoring, then network monitoring
Once the auth collector proves the pattern end-to-end, file integrity monitoring (creation/modification/deletion/rename in sensitive directories) and network socket logging (outbound connections, for spotting C2 callbacks) are built the same way: their own `class_uid` values, their own `data` shape, the same `event.Build()` + `wsc.Push()` calls. The outbox, reconciliation, and dedup machinery underneath doesn't change per collector — that was the entire point of building it as one shared pipeline instead of one-off code per feature.

### Step 6: Test jitter/reconnect behavior at real scale
Still open: run 10–50+ simultaneous agent instances (multiple processes on one machine, each with its own registered identity, is sufficient for now), stop the server, bring it back, and confirm from timestamps that reconnects spread out rather than clustering in the same instant.

### Step 7: Verify multi-tenant isolation
Register two agents under two genuinely different `tenant_id` values (rather than both defaulting to the same one, as in all testing so far) and confirm a query scoped to one tenant never returns the other's events.

### Step 8: Lower-priority durability hardening
Outbox size limits/retention policy for very long outages, chunked reconciliation for very large backlogs, a dead-letter path for events the server permanently rejects (Section 6, item 14), and a decision on duplicate connections per `agent_id` (item 18). None is urgent at current scale, but all should happen before a real deployment with agents that could be offline for days.

### Step 9 (optional): Understand, and eventually fix, the shutdown-ordering race
Low severity, self-healing, not urgent — but worth eventually making airtight now that this is genuinely production code rather than an experiment.

---

## 8. How To Test This Yourself (After Pulling From GitHub)

This section is for anyone (like a teammate) who pulls the repo fresh and wants to actually see the durable-delivery system working, not just read about it.

**Where to run commands:** every path below is relative to the repo root (the folder containing `server/` and `agent/`), unless a step says `cd`. Running a repo-root command from inside `server\app` produces confusing "path does not exist" errors.

### 8.1 What you need running first

You need **two things running before any test makes sense**:

1. **The server**, with a PostgreSQL database it can reach.
2. **A registered agent identity** — either a real agent, or a throwaway one you register just for testing.

```powershell
# 1. Start the server (from server/app/, with the project's virtual environment active,
#    e.g. ..\venv\Scripts\Activate.ps1)
cd server/app
uvicorn main:app --reload --host 0.0.0.0 --port 8000
```

Keep this terminal open and visible — you'll be watching its logs during every test below.

**Optional pre-flight** (from the repo root, with the virtual environment active): confirm the three server files agree with each other before starting.
```powershell
cd server\app
python -c "import db.models, schemas.ws, routers.ws; print('imports OK')"
```
If this fails with `No module named 'asyncpg'`, the virtual environment isn't active.

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

### 8.3 Build the test tools

These are small, throwaway programs (not part of the real agent) built specifically to exercise the outbox and WebSocket client directly, since no real collector exists yet. **`testpush` and `testreconnect` build their events with the shared `internal/event` package, so `agent/internal/event/` (`event.go`) must be present in your checkout or they will not compile.** Optionally run the package's unit tests first: `go test ./internal/event/` (this needs the file named exactly `event_test.go`).

**Windows binaries** (from `agent/`):
```powershell
go build -o testpush.exe ./cmd/testpush
go build -o testreconnect.exe ./cmd/testreconnect
go build -o dumpoutbox.exe ./cmd/dumpoutbox
```

**Linux binaries**, cross-compiled from Windows:
```powershell
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o testpush-linux ./cmd/testpush
go build -o testreconnect-linux ./cmd/testreconnect
go build -o dumpoutbox-linux ./cmd/dumpoutbox
Remove-Item Env:GOOS; Remove-Item Env:GOARCH
```
The last line matters: `GOOS` stays set for the rest of the PowerShell session, so without it your next `go build` would quietly produce Linux binaries again. Copy the three `-linux` files to the Linux machine (`scp`), then `chmod +x` them. Run them on the machine where the agent is registered, with `sudo` if reading `/etc/khemstrix-agent/config.json` or writing the outbox needs it.

*(If the `cmd/` folders don't exist in your checkout yet, ask whoever wrote this report for the three `main.go` files — they're intentionally not part of the permanent codebase.)*

### 8.4 Test 1 — The happy path (server stays up the whole time)

```powershell
.\testpush.exe
```

**What you should see:**
- A `DEBUG agent_id=... api_key=... server=...` line confirming which identity it's using *(this prints the key in plaintext — see Section 6, item 15 — so don't paste this output anywhere shared)*
- `pushing test event <uuid>`
- No errors

**Then confirm it actually landed**, in Postgres:
```sql
SELECT event_id, hostname, data, created_at FROM events WHERE hostname = 'testpush-harness' ORDER BY created_at DESC;
```

**And confirm the new envelope fields and the timestamp:**
```sql
SELECT hostname, host_os, host_os_version, agent_version, schema_version, ingest_source,
       created_at - time AS diff
FROM events WHERE hostname = 'testpush-harness' ORDER BY created_at DESC LIMIT 3;
```
Expected on Windows: `windows`, `NULL`, `0.1.0`, `1`, `websocket`, and a `diff` of a few seconds (not hours; a small negative value is just clock skew between machines). Expected on Linux: `linux`, a version string from `/etc/os-release` (for example `Kali GNU/Linux Rolling`), `0.1.0`, `1`, `websocket`.

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

### 8.6 Test 3 — A larger backlog during an outage

Same idea as Test 2, but proving it holds up with more than one queued event. **Order matters: stop the server first, confirm it is down, and only then start pushing.** (In the September 21 run the loops started with the server up, so only about 3 events per agent queued — see Section 5.7.)

```powershell
# 1. Kill the server (Ctrl+C in its terminal) and confirm it's down (Test-NetConnection, as in 8.5)

# 2. While it's down, push several events in a row:
for ($i=1; $i -le 15; $i++) { .\testpush.exe }
```
On Linux the equivalent loop is `for i in $(seq 1 15); do sudo ./testpush-linux; done`. Each `testpush` run waits about 15 seconds, so 15 runs take around four minutes; the server must stay down for the whole loop.

```powershell
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

### 8.7 Test 4 — A second, independent agent (and both at once)

This proves agent isolation, not just durable delivery for one agent in isolation.

1. Register a **second** agent identity — either cross-compile the Linux binary and run it on a separate machine/VM, or simply register a second Windows identity on the same machine using a fresh enrollment token.
2. Repeat Test 3 (the 15-event backlog test) independently for this second agent — or run both loops **at the same time**, one on each machine, as in the September 21 run (Section 5.7).
3. Confirm in Postgres that every event from both agents carries the correct, distinct `agent_id` and OS:
```sql
SELECT agent_id, host_os, host_os_version, COUNT(*) AS events
FROM events
WHERE hostname = 'testpush-harness'
GROUP BY agent_id, host_os, host_os_version;
```
Expect one row per agent, with the counts you pushed. On the Linux agent, `host_os = linux` and `host_os_version` comes from `/etc/os-release`. To count only the newest run, add `AND created_at > now() - interval '15 minutes'`.

### 8.8 Test 5 *(to be written once Step 3 in Section 7 is built)* — The first real collector

Once the failed-authentication collector exists, repeat Tests 1–2 against it directly: deliberately fail a login on the endpoint (wrong password, a few times) instead of running `testpush.exe`, and confirm the resulting event — with its real `class_uid`, `host_os`, and `data` fields — survives an outage and reconciles the same way the synthetic test events always have. This is the point where "the transport works" and "the product works" finally become the same test.

### 8.9 Common gotchas (things that tripped us up, so you don't repeat them)

- **Run the test tools on the same machine where the agent is actually registered.** `config.json` is local to whatever machine you're on — running the test tool from a different computer will read a *different* config file with different (possibly stale or nonexistent) credentials, and fail with a confusing "invalid credentials" error that has nothing to do with the actual code.
- **A leftover Windows service can lock the outbox file.** If you get a "readonly database" error, check `Get-Service | Where-Object { $_.Name -like "*khemstrix*" }` — if one is running, stop it first, and fix folder permissions as shown in 8.2.
- **On Linux, stop the real agent service before running the test tools.** The `khemstrix-agent` service uses the same `agent_id` and may share the same outbox file, so running `testpush-linux` alongside it means two connections under one identity and a possible lock conflict. Use `sudo systemctl stop khemstrix-agent`, run the test, then `sudo systemctl start khemstrix-agent`. (`systemctl list-units | grep -i khemstrix` printing nothing after the stop is normal — `list-units` only shows active units; use `systemctl status khemstrix-agent` or `systemctl list-unit-files` to confirm.)
- **`--reload` on uvicorn can auto-restart the server on file changes**, which can accidentally interrupt your test at the wrong moment. If you're actively editing files nearby while testing, that restart isn't a real "server crashed" test — do a deliberate `Ctrl+C` instead, and don't touch files mid-test.
- **Double-check which agent's credentials you're actually testing with** if your project has multiple registered agents from earlier testing — a stale or deleted agent's credentials will always fail auth, even though the failure looks identical to a real bug.
- **A service that "disappears" from `Get-Service` may have simply crashed, not been renamed or corrupted.** Check the System event log (`Get-EventLog -LogName System -Source "Service Control Manager"`) before assuming a lookup/permissions issue — if the process crashed, run the `.exe` directly first to see its actual error output.
- **A `"use of closed network connection"` log line right after a successful test push is very likely harmless connection cleanup, not a real failure** — confirm by checking the outbox; if it's empty, the event went through fine despite the alarming-looking log line.
- **If `created_at − time` in Postgres comes out as roughly your timezone offset (about 7 hours for Cambodia) instead of a few seconds**, a timezone-less datetime is reaching the `timestamptz` columns somewhere and being read as server local time. Check that the handler is sending timezone-aware UTC values (Section 5.6). A gap of a second or so, occasionally slightly negative, is normal clock skew.
- **If the server drops the connection right after the first event and the agent keeps retrying**, check the server terminal for an `AttributeError` mentioning `schema_version`, `agent_version`, `host_os` or `host_os_version`. It means `routers/ws.py` was updated but `schemas/ws.py` wasn't — deploy all three server files together. No events are lost while this is happening; they simply wait in the outbox.
- **`go test` says "no test files", or the unit tests never run** *(new)*: check the file is named exactly `event_test.go`. A file named `eventest.go` compiles as normal code and Go silently skips it as a test.
- **`No module named 'asyncpg'` when importing the server modules** *(new)*: the Python virtual environment isn't active. Activate it (`server\venv\Scripts\Activate.ps1`, adjusted to wherever your venv is) and retry.
- **`Cannot find path ...\server\app\server\app\...`** *(new)*: you ran a repo-root command from inside `server\app`. `cd` back to the repo root first.
- **After a `GOOS=linux` build, `go build` keeps producing Linux binaries** *(new)*: clear the variables with `Remove-Item Env:GOOS; Remove-Item Env:GOARCH`.

---

## 9. One-Sentence Summary

**The problem**: switching to instant delivery (WebSockets) risked losing security events whenever a connection dropped. **The solution**: never delete an event until the server confirms it, and automatically catch up on reconnect. **Status as of September 21 (later)**: this solution is confirmed running inside the real production agent (not just test tools); the five new event fields (`schema_version`, `agent_version`, `host_os`, `host_os_version`, `ingest_source`) and the switch of the `events` timestamps to timezone-aware UTC are now **verified against the live server and database** with two agents at once (Windows and Linux) — 30 events, correct per-agent attribution, correct OS fields, events queued during a server outage delivered afterward, and zero duplicates; the remaining housekeeping is small (redact the API-key debug line, update two out-of-date documents, commit the new `event` package), and the next jobs are deciding the event contract for real event types, adding TLS, and building the first genuine collector — failed authentication attempts — followed by file and network monitoring using the same proven pipeline.