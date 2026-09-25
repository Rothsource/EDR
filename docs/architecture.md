# EDR Project — Architecture & Scaling Guide

This is your reference doc for understanding what each file does, and the exact
checklist to follow every time you add a new table or feature.

**Updated September 24, 2026.** The previous version of this doc predated the
server-side decoder framework, remote agent configuration, and the Windows
auth collector — all three are now built and running. Section 4 is rewritten,
and Sections 4A and 4B are new.

---

## 1. The Big Picture — How a Request Flows Through Your App

```
Client (agent / admin)
        │
        ▼
   routers/*.py        ← receives HTTP request, validates input, calls logic
        │
        ▼
   schemas/*.py         ← defines what valid input/output looks like (Pydantic)
        │
        ▼
   db/models.py          ← Python classes mapped to real Postgres tables
        │
        ▼
   db/database.py         ← manages the connection pool + session lifecycle
        │
        ▼
   config.py               ← loads .env, builds the DB connection string
        │
        ▼
   Postgres (erp database)  ← actual data lives here
```

Every layer has ONE job. Never let a layer do another layer's job (e.g. never
put raw SQL string-building inside a router, never put request-validation logic
inside `models.py`). This separation is what keeps the codebase debuggable as
it grows — when something breaks, you know which file to open based on *what
kind* of thing broke (a bad value from a client → schemas; wrong data in the
DB → models/router logic; a connection problem → database.py/config.py).

For protected routes, there's one more layer sitting in front of the router:

```
Client
  │
  ▼
Authorization: Bearer <token> header
  │
  ▼
core/deps.py  → get_current_user_id()   ← verifies JWT signature + expiry
  │
  ▼
routers/*.py                             ← only runs if the dependency succeeded
```

Agent-facing routes (`/agent/heartbeat`, `/agent/config`) use a **different**
credential entirely — `agent_id` + `api_key`, sent as headers, not a JWT. See
Section 3.

### 1.1 A Second Flow: Telemetry Events (WebSocket)

Admin and agent-management traffic (login, registration, heartbeat, config
fetch) follows the HTTP request/response flow above. Telemetry events follow a
**parallel, persistent-connection flow**, because they're a continuous stream
rather than discrete operations, and — as of this update — they carry **raw,
unparsed records**, not finished, meaningful events:

```
Agent (Go)
   │
   │  1. Collector reads a raw record (a journald line, a Windows Event
   │     Log entry) — it does NOT decide what the record means
   ▼
Local durable outbox (SQLite, WAL mode)   ← event written here BEFORE send is attempted,
   │                                         wrapped in the standard envelope with the
   │                                         untouched record under data.raw
   │  2. Attempt delivery over the open WebSocket
   ▼
WebSocket connection (persistent, authenticated once at connect time)
   │
   ▼
Backend WebSocket manager (routers/ws.py)   ← lives alongside routers/*.py,
   │                                           but is not a normal HTTP router
   │  3. Server dispatches data.source to a decoder (decoders/), which fills
   │     in the REAL class_uid/category_uid/username/data — see Section 4A
   ▼
Postgres `events` table
   │
   │  4. Server sends an acknowledgement back over the same connection —
   │     ALWAYS, even if the decoder didn't recognize the record (D4:
   │     unrecognized records are acked-and-dropped, not retried forever)
   ▼
Agent outbox                                ← event is only deleted here,
                                               after ack is received
```

The two flows share the same database and the same agent identity/auth model
(`agent_id` + `api_key`), but the transport mechanics are different enough
that it's worth keeping them mentally separate: HTTP routers are stateless
per-request; the WebSocket manager holds a live connection per agent and has
to track connection state, not just handle one request and return.

### 1.2 A Third Flow: Remote Configuration (piggybacked on heartbeat)

New since the last version of this doc. An admin can change **which sources**
an agent collects from (e.g. add `sudo` monitoring, or narrow a noisy Windows
machine down to just failed logons) without touching that machine by hand.
This does **not** use the WebSocket or a new message type — it rides on the
existing heartbeat, deliberately, so a broken event stream never blocks a
config update and vice versa:

```
Agent heartbeat loop (every ~30s)
   │
   │  1. POST /agent/heartbeat  →  response includes config_version
   ▼
Agent compares config_version to its own locally saved version
   │
   │  match → nothing happens, loop continues
   │  mismatch ↓
   ▼
GET /agent/config  (headers: X-Agent-ID, X-API-Key)
   │
   ▼
Server resolves the agent's actual source list (routers/agent.py) and
returns { sources, version, source }
   │
   ▼
Agent writes it atomically (tmp file → fsync → rename) to
config/auth.json, then calls Reload() on the running collector
   │
   ▼
Collector tears down and relaunches ONLY its own internal subsystem
(the journalctl subprocess on Linux; the Event Log subscription on
Windows) — the agent PROCESS itself never restarts, and neither does
the WebSocket connection
```

See Section 4B for the full detail, including why this design was chosen over
a new WebSocket message.

---

## 2. What Each File Actually Does

### `config.py`
**Job:** Read `.env`, expose `settings.DATABASE_URL` and `settings.JWT_SECRET_KEY`.
**Contains:** Zero logic. Just environment loading.
**You touch this:** Almost never, after initial setup — maybe to add new
settings as your app grows (e.g. a `WS_HEARTBEAT_INTERVAL_SECONDS` setting
once the WebSocket heartbeat timing needs to be tunable rather than
hardcoded).

### `db/database.py`
**Job:** Create the async engine (connection pool), the session factory, the
shared `Base` class, and the `get_db()` dependency used by every router.
**Contains:** Zero business logic. Just plumbing.
**You touch this:** Almost never — maybe to tune pool settings (`pool_size`,
`echo=False` for production) later. Worth revisiting once the WebSocket
manager is issuing DB writes at a higher frequency than the admin/HTTP side
ever did — a pool sized for occasional heartbeats may not be sized for a
continuous event stream from many agents.

### `db/models.py`
**Job:** Mirror your Postgres tables as Python classes (SQLAlchemy ORM).
**Contains:** Column definitions, types, constraints, foreign keys,
relationships. **No logic** — no validation, no business rules, no request
handling.
**You touch this:** Every time you add or change a table. `Agent` now has
`config_version` (Integer, NOT NULL, default 1) and `auth_config_sources`
(JSONB, nullable — `NULL` means "use the server default for this OS"), added
for remote configuration (Section 4B).

### `schemas/*.py`
**Job:** Define what a valid API *request* and *response* look like —
independent from the DB structure.
**Contains:** Pydantic classes. No DB queries, no business logic — just shape
+ validation rules.
**You touch this:** Every time you add an endpoint, or change what an
endpoint accepts/returns.
**Why it's separate from `models.py`:** the DB table often has fields the
client should never send (`agent_id`, `created_at`) or never see again
(`api_key`, `password_hash`). Schemas let you control exactly what's
exposed, per-endpoint. The same principle applies to the WebSocket event
envelope: the inbound message an agent sends and the row that ends up in
Postgres are related but **not** identical anymore — the inbound message
now typically carries `data.raw` (an unparsed record) and placeholder
envelope fields the agent doesn't have the information to fill in
correctly, while the stored row has the real, decoder-filled
`class_uid`/`category_uid`/`username`/`data`. See Section 4A.

### `routers/*.py`
**Job:** This is where actual logic lives — the real "what happens when this
endpoint is called."
**Contains:**
- Reading input (validated automatically via the schema type hint)
- Querying/writing to the DB via `db.execute(select(...))`, `db.add(...)`,
  `db.commit()`
- Business rules (token expiry checks, credential checks, generating secrets)
- Raising `HTTPException` for error cases
- Returning data shaped by a response schema

**You touch this:** Every time you build a new feature/endpoint.

`routers/agent.py` now has three agent-facing concerns instead of two:
registration/heartbeat (unchanged in shape), and `GET /agent/config`
(new — see Section 4B). It authenticates via `X-Agent-ID`/`X-API-Key`
**headers**, not query parameters — an earlier version put credentials in
the URL, which leaks into access logs; that's now deprecated but still
accepted for backward compatibility until every deployed agent is confirmed
updated.

`routers/ws.py` is not a normal `@router.post(...)` — it's defined with
`@router.websocket(...)` and has a different lifecycle: it doesn't return
once and finish, it stays open, loops on incoming messages, and needs
explicit handling for disconnects. Its `_handle_event` function is the
entry point into the decoder framework (Section 4A) — it dispatches on
`data.source`, awaits the matching decoder, and acks-and-drops (never
crashes) on anything the decoder doesn't recognize.

### `decoders/` (new package)
**Job:** Turn an agent's raw, unparsed record into the real OCSF envelope
fields (`class_uid`, `category_uid`, `activity_id`, `severity_id`,
`type_uid`, `username`, `data`). This is the other half of the architecture
decision described in Section 4A: **the agent ships raw data; the server
decides what it means.**
**Contains:**
- `__init__.py` — the `@register(source)` decorator, the `DECODERS` dict it
  populates, and `decode_event(source, raw, db)`, the single entry point
  `routers/ws.py` calls. Decoder modules are imported at the **bottom** of
  this file, after `register`/`DecodedFields`/`PARSER_VERSION` are defined —
  importing them earlier causes a circular-import crash at startup, since
  each decoder module imports `register` back from `__init__.py`.
- `journald.py` — decodes Linux `sshd`/`sshd-session`/`sudo` raw lines
  (regex-based; journald messages are free text).
- `windows_security.py` — decodes Windows Security Event Log records
  (mostly field extraction; the XML is already structured, so this is
  easier than the journald side). Dispatches on the numeric Event ID
  carried in the raw record.
- `unmatched.py` — records a raw record that no decoder recognized. Per
  decision D4, an unrecognized record is **acked** (so the agent doesn't
  retry it forever) and **dropped** from the main pipeline; it's currently
  also stored in a separate `unmatched_raw` table for visibility/replay,
  which is a superset of what D4 originally specified (a process-lifetime
  counter) — worth a deliberate decision on whether to keep doing that
  long-term (Section 4A.3).
**You touch this:** Every time you add a new raw source (a new Linux
identifier, a new Windows Event ID, eventually file/network events) — you
add a decoder function with `@register("that-source")`, not a change to
`routers/ws.py`.

### `core/security.py`
**Job:** Password hashing and JWT create/verify — pure functions, no request
handling, no DB access.
**Contains:**
- `hash_password()` / `verify_password()` — bcrypt via passlib
- `create_access_token()` / `decode_access_token()` — JWT via python-jose,
  signed with `settings.JWT_SECRET_KEY`, 24h expiry
**You touch this:** Rarely — maybe to change token expiry duration or add
a refresh-token flow later.

### `core/deps.py`
**Job:** FastAPI dependencies that guard routes — currently just
`get_current_user_id`.
**Contains:** Reads the `Authorization: Bearer <token>` header, calls
`decode_access_token()`, raises `401 not authenticated` on anything missing/
invalid/expired, otherwise returns the `user_id` from the token.
**You touch this:** To protect any new route, add
`Depends(get_current_user_id)` as a parameter — no changes needed to this
file itself unless you add new kinds of guards (e.g. role-based checks
later). This dependency is for the human/admin JWT flow only — agent
authentication (`agent_id` + `api_key`) is a separate, simpler check that
lives inline in `routers/agent.py` and `routers/ws.py`, not bolted onto
`get_current_user_id`.

### `detection/` and `response/`
**Job (once you build into them):**
- `detection/` — turns raw event data into a `score` / `verdict` (rules,
  thresholds, eventually ML). **Not built yet.** The decoder framework
  (Section 4A) gives this its first real input: normalized, structured
  events with a consistent `data.status`/`data.reason` shape across both
  OSes, which is exactly what a rule like "5+ failed logins from one
  source in 60 seconds" needs.
- `response/` — takes a verdict and *does something* (isolate a host, kill a
  process, alert an admin).

These are kept separate from `routers/` so your "receive a request" logic
never gets tangled up with your "decide if this is malicious" logic. A
router (or the WebSocket handler) should call into `detection/` or
`response/`, not contain that logic itself. Concretely: `_handle_event`'s
job ends once a raw record is decoded, stored, and acknowledged — it should
not itself decide "this looks like a brute-force attempt." That belongs in
`detection/`, running either synchronously right after storage or, once
volume grows, as a separate background pass over recently-stored events.

### `main.py`
**Job:** Create the `FastAPI()` app, register (`include_router`) every
router, nothing else.
**Contains:** No business logic. Just wiring.

---

## 3. How Agent Authentication Works (`api_key`)

Every agent needs a way to prove "it's still me" on every request after it
first connects — without the server having to trust just an IP address or a
hostname (both are easy to spoof). That's what `api_key` is for: a long,
random, secret string that acts like a password, but for a machine instead
of a human.

### 3.1 The problem it solves

Your admin uses a **username + password** to log in as a human, once per
session, and gets a temporary JWT back.

An **agent** is different — it's an unattended process running on a
customer's endpoint that needs to check in repeatedly (heartbeats, a
persistent event stream, and now periodic config fetches) with no human
present to type a password. So instead of "login every time," an agent gets
issued **one long-lived secret** at the moment it registers, and it sends
that secret with every future request instead of logging in.

### 3.2 Where credentials travel, and how (updated)

| Call | Credentials sent as |
|---|---|
| `POST /agent/register` | body (`enrollment_token`, one-time, separate secret) |
| `POST /agent/heartbeat` | body (`agent_id`, `api_key`) |
| `GET /agent/config` | **headers** (`X-Agent-ID`, `X-API-Key`) — see note below |
| `WS /agent/ws` connect | once, at connect time, same pair |

**Why `/agent/config` uses headers and the others use the body:** this was a
deliberate fix. An earlier version of `/agent/config` accepted
`?agent_id=...&api_key=...` as URL query parameters, which get written
verbatim into access logs (uvicorn, any reverse proxy in front of it) —
silently leaking the agent's long-lived secret into plaintext logs on every
single config check, which happens roughly every 30 seconds. Headers are not
logged by default. The query-parameter form is still accepted as a
**deprecated fallback** so agents on an older build don't lose config
updates mid-rollout, but it logs a warning server-side when used, and should
be deleted entirely once every deployed agent is confirmed on headers.
Moving the heartbeat and register bodies to headers too would be a
reasonable future consistency pass, though request bodies aren't logged by
default the way URLs are, so it's lower priority.

### 3.3 The full flow, end to end

```
1. Admin (you) generates a one-time enrollment token
      POST /admin/generate-token   (requires admin JWT — protected route)
      → returns a random token, valid for 1 hour, single-use

2. That token gets baked into the install script/binary
   you hand to the customer (e.g. as a --token flag or embedded
   in a downloaded install command)

3. Customer downloads and runs the script on their machine
      .\edr-agent.exe --server=http://<your-server-ip>:8000 --token=<the enrollment token>

4. The agent calls the server to register itself
      POST /agent/register
      body: { hostname, os, enrollment_token }

      Server checks (routers/agent.py -> register_agent):
        - does this enrollment token exist?
        - has it expired? (1 hour window)
        - has it already been used?
      If all checks pass:
        - server generates a NEW random api_key (secrets.token_urlsafe(32))
        - creates the Agent row, storing that api_key, with
          config_version defaulting to 1 and auth_config_sources NULL
          (meaning: use the server-side default source list for this OS)
        - marks the enrollment token as used (so it can't be reused
          to register a second agent)
        - returns { agent_id, api_key } to the agent — ONE TIME ONLY

5. The agent saves { agent_id, api_key } locally, in its own
   identity file (config.json / identity.json on the customer's
   machine) — this is now the agent's permanent credential

6. Every heartbeat after that, the agent sends its saved credentials
   AND receives back the current config_version
      POST /agent/heartbeat
      body: { agent_id, api_key }
      response: { status, config_version }

      If the returned config_version differs from what the agent has
      saved locally, it follows up with GET /agent/config (see 3.2 and
      Section 4B) — this is a SEPARATE, independent check from the
      pass/fail credential check below.

      Server checks (routers/agent.py -> heartbeat):
        - does an agent with this agent_id exist, and is it active?
        - does its stored api_key match the one just sent?
      If either check fails -> same generic 401 "invalid credentials"

7. Separately, the agent opens a persistent WebSocket connection for
   telemetry, authenticating once at connect time with the same
   { agent_id, api_key } pair. This connection stays open independently
   of both the heartbeat cycle and any config fetch — a dropped
   WebSocket does not mean the agent is unregistered, a config push
   does not touch the WebSocket, and vice versa.
```

### 3.4 Why the enrollment token and the api_key are two different things

| | `enrollment_token` | `api_key` |
|---|---|---|
| Purpose | One-time proof "an admin authorized this machine to join" | Ongoing proof "this is the same agent that registered before" |
| Lifespan | 1 hour, single-use, then dead | Lives as long as the agent is enrolled |
| Where it's used | Only once, in `POST /agent/register` | Every heartbeat, every config fetch, and once at WebSocket connect time |
| Who generates it | The **server**, on admin request | The **server**, automatically, at registration time |

### 3.5 How `api_key` is generated (the actual code)

`routers/agent.py`, inside `register_agent()`:
```python
new_api_key = secrets.token_urlsafe(32)
```
`secrets.token_urlsafe(32)` uses Python's cryptographically secure random
number generator to produce a 32-byte random value, encoded as a URL-safe
base64 string. This key is:
- **Never chosen or influenced by the client**
- **Shown to the agent exactly once**, in the `POST /agent/register` response
- **Stored in plaintext in Postgres** (`agents.api_key`) — heartbeat and
  config-fetch verification do a direct string comparison, not a
  hash-and-compare like login does
- **Excluded from every other response** — `GET /agents` uses
  `AgentResponse`, which has no `api_key` field
- **Also stored in plaintext on the endpoint's local disk** (its identity
  file) — a pre-production hardening item, same as before

### 3.6 Rotating a compromised `api_key`

`POST /agents/{agent_id}/rotate-key` (protected, admin-only) exists and
works: it issues a fresh key and invalidates the old one in the same
transaction, so there's no window where both are valid. After calling it,
the agent's *own* saved identity file still has the old key and will start
failing every request immediately — you have to update it by hand, or
re-enroll the machine with a fresh token. **This has real operational
weight now that it's actually been used**: rotating a key on a live agent
causes a burst of `invalid credentials` heartbeat failures (harmless — the
agent just retries next cycle and queues events locally in the meantime)
until the new key reaches that machine's config file and the service is
restarted. There is currently no way to push the new key to the agent
remotely (chicken-and-egg: the config-fetch mechanism itself requires valid
credentials) — this has to be done by hand, once, per rotation.

---

## 4. How Telemetry Events Flow From Agent to Database

### 4.1 On the agent side

The agent never lets an event depend on the server being reachable at the
moment it happens: a **raw record** — not a finished, meaningful event — is
written to a local durable outbox (SQLite, WAL mode) *before* delivery is
attempted, and is only deleted from that outbox once the server has
confirmed receipt. If the WebSocket connection drops, queued events remain
locally and are resent — selectively, via reconciliation, not by re-sending
the entire history — once the connection is restored.

**A collector's job stops at "which pile do I read from," never "what does
this line mean."** Concretely, on Linux (`internal/modules/auth/reader_linux.go`):
1. Tail `journalctl` filtered by `SYSLOG_IDENTIFIER` (`-t sshd -t sudo ...`)
   — pure source selection.
2. A loose, non-classifying relevance check trims high-volume routine noise
   (`pam_unix(sudo:session)` open/close lines) without deciding
   pass/fail/reason for anything that gets through.
3. Wrap the kept line's untouched fields under `data.raw`, with placeholder
   `class_uid`/`category_uid`/`activity_id`/`severity_id` values the server
   will overwrite.
4. `Push()` it to the outbox. **Only advance the read cursor once `Push()`
   has actually succeeded** — advancing it on a failed push (or on any
   attempt regardless of outcome) can permanently lose that record; this was
   a real bug, found and fixed, on both the Linux and Windows collectors
   independently.

The Windows collector (`internal/modules/auth/window.go` +
`windows_wevtapi.go`) follows the identical pattern against the Security
Event Log instead of journald: subscribe filtered by Event ID (structured
XML query, not free-text filtering — Windows records are already
structured, which is why this side needed no regex), persist a bookmark
instead of a cursor, and never classify content.

### 4.2 On the server side

The WebSocket manager's responsibilities, per connection:

1. **Authenticate at connect time** — verify `agent_id` + `api_key` once,
   when the socket opens.
2. **Keep the connection alive** — respond to heartbeat pings so the agent
   can distinguish "connection is genuinely open" from "looks open but
   isn't."
3. **Receive each raw record and decode it** — this is the step that
   changed. `_handle_event` (`routers/ws.py`) reads `data.source`, awaits
   `decode_event(source, raw, db)` (Section 4A), and gets back either a
   fully-decoded event or `None`.
4. **Store the event idempotently, OR ack-and-drop** — a decoded event is
   inserted into `events` using `event_id` as the conflict target
   (`ON CONFLICT DO NOTHING`); a `None` result is still **acked** (so the
   agent's outbox clears it and doesn't retry forever) but is not inserted
   into `events` — see Section 4A.3 on where it goes instead.
5. **Acknowledge** — always, on both paths above, so the agent knows it can
   safely delete that record from its local outbox. **This must happen
   even when the decoder crashes** — an unhandled exception inside a
   decoder (or, as happened once during development, forgetting to `await`
   an async decode call) will otherwise kill the WebSocket connection for
   *every* subsequent event on that socket, not just the one that failed.
6. **Handle disconnects gracefully** — unchanged: the agent's own durable
   outbox is the source of truth for what still needs delivering.

### 4.3 Idempotency in practice

Unchanged: the `event_id` primary key constraint on `events` is what makes
a duplicate delivery safe, regardless of how many times or how the retry
happens on the agent side.

### 4.4 Multi-tenancy and multi-agent isolation

Unchanged: every row in `events` carries both `agent_id` and `tenant_id`,
and every query should filter by `tenant_id` as a matter of habit.

---

## 4A. The Decoder Framework: Why the Server, Not the Agent, Decides What an Event Means

This is the single biggest architectural change since the last version of
this doc, and it's worth understanding *why*, not just *what*.

### 4A.1 The problem it solves

The very first working version of Linux auth collection had the agent parse
meaning out of log lines itself — regex on the agent side decided "this is
a failed login" before the record ever left the machine. That worked, but
it has a fatal scaling problem: **teaching the system to recognize a new
failure-message format (a different Linux distro's phrasing, a new Windows
patch changing a field name) requires rebuilding and redeploying every
single agent in the field.** For a security product meant to run
unattended on customer machines, that's an unacceptable release cycle for
what should be a same-day fix.

### 4A.2 The fix: agents ship raw, the server decodes

- **The agent still does:** pick a scoped source (which journald
  identifiers / which Windows Event IDs to watch), track its own read
  position so a restart doesn't replay or skip records, and wrap each kept
  record in the standard envelope with the untouched data under `data.raw`.
- **The agent does not do, ever:** run a regex meant to extract meaning,
  decide pass/fail/reason, or fill in a username from log content.
- **The server does:** everything else. `decoders/__init__.py` dispatches
  on `data.source` to the matching decoder function, which fills in the
  real `class_uid`/`category_uid`/`activity_id`/`severity_id`/`type_uid`,
  `username`, and a normalized `data` dict.

A decoder bug fix, or support for a brand-new source, is now a **server-only
deploy** — no agent rebuild, no waiting for every customer's endpoint to
pick up a new binary.

### 4A.3 What happens to a record no decoder recognizes (decision D4)

`decode_event` returns `None`, `_handle_event` acks it (so the agent's
outbox clears it and doesn't retry forever) and does not insert it into
`events`. As currently implemented, it's also written to a separate
`unmatched_raw` table (`decoders/unmatched.py`) rather than only
incrementing an in-memory counter as originally specified — this gives
visibility into what's being silently dropped (useful for catching a
decoder gap, like a real Windows logon event being mistaken for noise) and
the option to **replay** those raw records later if a decoder is taught to
recognize them, since the original bytes are preserved. The tradeoff is
that this table currently has no `agent_id` column (so a dropped record
can't be traced to its source machine) and no retention policy — it will
grow unbounded, holding both genuinely-unrecognized records and records
that were *intentionally* filtered as routine noise (these are currently
indistinguishable in the table). Both are worth fixing before this table
is relied on for anything beyond ad-hoc debugging.

### 4A.4 Why this makes "what does content-aware filtering even mean" a hard boundary

Because the agent no longer parses meaning at all, it genuinely **cannot**
tell a successful login from a failed one in a raw line — that
classification only exists after a decoder runs. This is why remote
configuration (Section 4B) can only ever control *which sources* an agent
watches, never *which outcomes* it reports ("only failures, not
successes"). Implementing outcome-level filtering on the agent would
require reintroducing the exact content-aware parsing this section just
removed, just for one field. Outcome-level filtering, if wanted, belongs in
the decoder/policy layer, server-side — the agent ships everything from its
enabled sources, and the decoder or a future storage policy decides what to
keep or surface.

---

## 4B. Remote Agent Configuration

### 4B.1 The problem it solves

Before this existed, changing what an agent watches meant editing a config
file on that one machine by hand — fine for a lab VM, doesn't scale past a
handful of real customer endpoints.

### 4B.2 Why it rides on the heartbeat instead of a new WebSocket message

`wsclient.go` (the WebSocket client) has exactly one responsibility today:
event delivery with ack/reconcile. Adding config delivery as a second
responsibility to that same code path would tangle two genuinely different
failure modes together — a broken event stream would start affecting
config delivery, or a bad config push could interfere with event delivery,
for no real benefit. Piggybacking on the existing heartbeat instead keeps
them on separate failure paths: a broken WebSocket never blocks a config
update, and a bad config push can never block event delivery. The
tradeoff, stated plainly, is latency — a config change takes up to one
heartbeat interval (~30 seconds) to reach an agent. That's fine for routine
configuration; an "act immediately" mechanism, if ever needed, would be a
different, separate mechanism.

### 4B.3 File layout on the agent

```
identity.json     — AgentID, APIKey, Server. Written once, at registration.
                     Never touched by a remote config push.
config/
    auth.json     — { "sources": [...], "version": N, "source": "server"|"local" }
```

Identity and runtime config are deliberately two separate files: a
malformed or malicious config push can only ever touch `config/*.json`,
never `identity.json`, so it can't strand an agent unable to even
reconnect and get corrected. Writes are atomic everywhere (write to
`.tmp`, `fsync`, `os.Rename()`) — there is never a moment where
`auth.json` is missing or half-written, on either OS.

### 4B.4 What actually happens on the agent when a new config lands — the part that matters

**Hard requirement: no agent process restart, ever, for a config change.
No server restart either.** The only thing that restarts is the
collector's own internal subsystem — the `journalctl` subprocess on Linux,
the Event Log subscription on Windows. This is a narrow, purpose-built
reload, not a general live-config-reload system, and it's achievable
specifically because the only piece of "hot" state that changes on a
config update is one source list feeding one subprocess/subscription.

The sequence, as actually implemented:
1. Heartbeat detects `config_version` mismatch.
2. One follow-up call, `GET /agent/config`.
3. Atomic write of the new `auth.json`.
4. The heartbeat code calls `.Reload()` directly on the running collector
   (both live in the same process, so this is a plain method call, not IPC).
5. Inside `Reload()`: read the new sources, compare to what's currently
   running (no-op if identical), save the current read position, kill the
   old subprocess/subscription, launch a new one from that saved position.
6. Everything else in the agent — the heartbeat loop itself, the WebSocket
   connection and its outbox, any other collector — is completely
   unaffected throughout. From the outside (`systemctl status`,
   `Get-Service`, the server's own view of the connection), nothing about
   the agent looks different at all.

**This has been verified against a running agent, not just designed.** On
Linux: widening a source list and narrowing it back both took effect within
one heartbeat, with the agent's process ID unchanged throughout — only the
internal `journalctl` child process was replaced. Events queued from
*before* the reload continued to arrive correctly *after* it, proving the
saved read-position handoff has no gap. The Windows implementation follows
the identical shape (`Reload()` cancels the subscription's context; the
supervising loop resubscribes from the last saved bookmark) but has not yet
had this same live verification run against it.

### 4B.5 Two real bugs this surfaced, worth knowing about if you touch this code

- **A source-name mismatch silently drops an entire source.** The
  identifier map that turns a config `sources` entry (e.g. `"sshd"`) into
  what the collector actually filters on (`journalctl -t sshd -t
  sshd-session`, or a specific Windows Event ID) is a single point of
  failure — if a name in that map doesn't exactly match what the server
  sends, that source is silently never collected, with **no error
  anywhere**. This happened in practice (`"ssh"` in the map vs. `"sshd"`
  from the server) and cost real debugging time before being found by
  directly inspecting the collector's live subprocess arguments
  (`pgrep -a journalctl` / equivalent).
- **A failed `Push()` must stop the read-position from advancing on the
  NEXT successful record, not just the failed one.** The naive
  implementation (`continue` on push failure, keep reading) looks correct
  but has a subtle data-loss bug: if the *next* line's push succeeds, its
  cursor/bookmark write moves the saved position **past** the earlier
  failed record, and that record is now unrecoverable — the agent will
  never look at it again. The fix, on both OSes, is: on a push failure,
  stop reading entirely and force a restart of the whole read loop from
  the last position that was *actually* successfully saved.

### 4B.6 Not yet built

- A defaults layer more granular than "one hardcoded list per OS" — no
  group- or tenant-level defaults yet, so every deviation from the OS
  default currently requires a per-agent database update.
- Any admin UI for this — it's SQL today.
- TLS on the transport carrying all of this (see the main project status
  report's open items).

---

## 5. The Checklist: Adding a New Table

Every time you add a table, walk through these steps **in this order**:

### Step 1 — Create the table in Postgres (source of truth)
```sql
CREATE TABLE alerts (
  alert_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id uuid NOT NULL REFERENCES agents(agent_id),
  severity text NOT NULL,
  message text NOT NULL,
  created_at timestamp NOT NULL DEFAULT now(),
  resolved boolean NOT NULL DEFAULT false
);
```
Verify with `\dt` and `\d alerts` in psql.

### Step 2 — Add the model in `db/models.py`
```python
class Alert(Base):
    __tablename__ = "alerts"

    alert_id = Column(UUID(as_uuid=True), primary_key=True, server_default=func.gen_random_uuid())
    agent_id = Column(UUID(as_uuid=True), ForeignKey("agents.agent_id"), nullable=False)
    severity = Column(Text, nullable=False)
    message = Column(Text, nullable=False)
    created_at = Column(TIMESTAMP, nullable=False, server_default=func.now())
    resolved = Column(Boolean, nullable=False, default=False)

    agent = relationship("Agent", back_populates="alerts")
```
If you add a `relationship(...)`, remember to add the reverse side on the
related model too. **Gotcha you already hit once:** if you comment out or
remove a model class, also comment out/remove any `relationship("ThatClass",
...)` pointing at it elsewhere — SQLAlchemy fails at mapper-configuration
time with a `KeyError`/`InvalidRequestError` if a relationship references a
class name that isn't registered.

**New gotcha, from this update:** when adding a new column that a
NOT-NULL constraint or default depends on matching between Postgres and
`models.py` (like `config_version`), add it in **both places in the same
sitting**, and confirm the type matches exactly (a `JSON` vs `JSONB`
mismatch, or a missing `server_default` for a NOT NULL column, will surface
at the worst possible time — mid-heartbeat, as a 500 on every single
agent — rather than immediately).

### Step 3 — Add schemas in `schemas/alert.py`
Ask: what should a client be allowed to **send**, and what should they be
allowed to **see back**? These are often NOT identical to the full model.
```python
class AlertCreate(BaseModel):
    agent_id: UUID
    severity: str
    message: str

class AlertResponse(BaseModel):
    alert_id: UUID
    agent_id: UUID
    severity: str
    message: str
    created_at: datetime
    resolved: bool

    class Config:
        from_attributes = True
```

### Step 4 — Build the router in `routers/alerts.py`
This is where the actual behavior lives:
```python
@router.post("/alerts", response_model=AlertResponse)
async def create_alert(payload: AlertCreate, db: AsyncSession = Depends(get_db)):
    new_alert = Alert(**payload.model_dump())
    db.add(new_alert)
    await db.commit()
    await db.refresh(new_alert)
    return new_alert
```

If the route should be admin-only, add the auth dependency too:
```python
from core.deps import get_current_user_id

@router.post("/alerts", response_model=AlertResponse)
async def create_alert(
    payload: AlertCreate,
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),
):
    ...
```

### Step 5 — Wire the router into `main.py`
```python
from routers import alerts
app.include_router(alerts.router, tags=["alerts"])
```

### Step 6 — Test via `/docs`
Run the server, open `http://localhost:8000/docs`, and manually exercise the
new endpoint(s) before building anything on top of them. For protected
routes, test both without and with a valid `Authorization: Bearer <token>`
header. (The WebSocket endpoint isn't testable through Swagger UI the same
way — use a small test client/script instead.)

---

## 6. The Checklist: Modifying an Existing Table

Say you want to add `ip_address` to `agents`:

1. **Postgres:** `ALTER TABLE agents ADD COLUMN ip_address text;`
2. **`db/models.py`:** add `ip_address = Column(Text)` to the `Agent` class
3. **`schemas/agent.py`:** add `ip_address: Optional[str] = None` to
   `AgentResponse` (and to `AgentCreate` only if clients should be allowed
   to set it themselves)
4. **`routers/agent.py`:** update logic if the new field needs to be set/used
   somewhere (e.g. captured during registration)

**Order matters:** always change Postgres first, then work back up through
the layers. SQLAlchemy does not auto-sync with the database — you keep
`models.py` in sync by hand (or via Alembic, see below).

---

## 7. Rules of Thumb As You Scale

- **`models.py` = structure only.** If you're tempted to write an `if`
  statement in there, it belongs in a router or a service module instead.
- **`schemas/` = contract with the outside world.** Never expose secrets
  (`api_key`, `password_hash`) in a general-purpose response schema.
- **Routers should stay thin.** The same discipline applies to the
  WebSocket handler and now to decoder dispatch: `_handle_event` should
  route to a decoder and store/ack the result — the actual field-mapping
  logic for a given source lives in that source's decoder module, not
  inline in the router.
- **Decoders should stay pure and defensive.** A decoder function takes a
  raw dict and returns either `DecodedFields` or `None` — it should never
  raise on malformed input (wrap risky parsing so a single bad record can't
  take down the WebSocket handler for every other event on that
  connection) and should never perform I/O itself (the `db` parameter that
  flows into `decode_event` is used for the unmatched-record path, not for
  decoder logic).
- **Same error for different failure reasons, when it matters for security.**
  E.g. `heartbeat` returns the same `401 invalid credentials` whether the
  `agent_id` doesn't exist or the `api_key` is wrong. `/agent/config`
  should follow the same rule (confirm it does, since it was added after
  this pattern was first established elsewhere).
- **Never build raw SQL strings with f-strings/concatenation.** Always use
  SQLAlchemy's `select(...)`/`.where(...)` or parameterized queries.
- **One-time secrets get their own response schema.**
- **A valid JWT proves "who you were when the token was issued," not
  "confirm this sensitive change right now."**
- **Postgres columns here are `timestamp without time zone`** on most
  tables — but `events.time`/`events.created_at` are `timestamptz`,
  deliberately different, since agent-reported event times need to be
  unambiguous across machines in different timezones (and, as observed in
  practice, agent clocks that drift slightly relative to the server —
  small but nonzero skew has been measured between at least one agent VM
  and the server, worth an NTP check before this matters for detection
  ordering).
- **At-least-once delivery means your database, not your application
  code, is the real duplicate-prevention layer.**
- **A persistent connection has state a stateless HTTP request doesn't.**
- **A background service has no console.** Anything using `log.Printf` or
  `fmt.Fprintf(os.Stderr, ...)` for diagnostics is invisible once an agent
  runs as an installed service rather than an interactive process — this
  cost significant debugging time on the Windows collector specifically,
  where a real API failure was silently swallowed for an extended period
  before file-based logging was added. If you write a new long-running
  background component on either OS, give it a log file from the start,
  not after you're already stuck debugging it blind.
- **When a Win32 API call wrapped via `syscall`/`golang.org/x/sys` fails,
  capture the error from the call's own return values, not from a
  separate `GetLastError()` call afterward** — through Go's calling
  convention the latter can come back `nil` even on a genuine failure,
  which turns every error message into a useless `%!w(<nil>)` with no
  indication of what actually went wrong.

---

## 8. When Your Schema Starts Changing Often: Alembic

Unchanged from before — you're still keeping Postgres and `models.py` in
sync by hand. That's increasingly worth revisiting: the `agents` table
alone has picked up two new columns (`config_version`,
`auth_config_sources`) in ad-hoc `ALTER TABLE` statements this update, on
top of everything added previously. Once you're past the "single developer,
single local database" stage, Alembic removes the need to manually remember
and re-run those statements against every environment.

---

## 9. Current Project Snapshot (as of this update)

**Tables in Postgres:** `enrollment_tokens`, `agents`, `users`, `events`,
`alerts` (schema exists; not yet populated by any detection logic), and
`unmatched_raw` (new — see Section 4A.3).

**`agents` gained two columns this update:** `config_version` (Integer, NOT
NULL, default 1) and `auth_config_sources` (JSONB, nullable) — see Section
4B.

**Telemetry ingestion has moved from "validated with a test harness" to
"validated with real production traffic on real endpoints."** Both the
Linux and Windows collectors are wired into the actual agent process (not
a standalone test binary) and have shipped real events end-to-end,
including a genuine SSH credential-guessing burst captured and correctly
decoded on one Linux VM — the first real-world validation data this
project has, as opposed to synthetic test events.

**New: `decoders/` package** — see Section 4A. Two decoders exist
(`journald`, `windows_security`), covering Linux `sshd`/`sudo`/`su` and 25
Windows Security Event IDs respectively. `PARSER_VERSION` is tracked per
decoded record (`metadata.parser_version`) so a future decoder fix can be
identified as applying (or not) to already-stored rows.

**New: remote agent configuration** — see Section 4B. Proven working
end-to-end on Linux (config push applies within one heartbeat, zero agent
restarts, verified across widen/narrow cycles on two separate machines).
Built but not yet proven the same way on Windows.

**Known gaps, verified from real data, not yet fixed:**
- On Windows, successful logons (Event ID 4624) and privileged-use events
  (4672) are currently being dropped as "unmatched" rather than stored —
  this needs to be fixed before the Windows side is trustworthy for
  anything beyond failed-login monitoring.
- `unmatched_raw` has no way to trace a dropped record back to its source
  agent, and no retention policy.

**Endpoints, current full list:**
- `POST /auth/login` — admin login, returns JWT
- `PUT /auth/change-password` — admin changes their own password (protected)
- `POST /admin/generate-token` — admin creates a one-time enrollment token (protected)
- `POST /agent/register` — new agent registers using a valid enrollment token
- `POST /agent/heartbeat` — registered agent proves it's alive, receives `config_version`
- `GET /agent/config` — **new** — agent fetches its resolved source list (headers, or deprecated query-param fallback)
- `GET /agents` — list all agents, excludes `api_key` (not yet protected)
- `POST /agents/{agent_id}/rotate-key` — **new**, protected, admin-only
- `WS /agent/ws` — persistent telemetry ingestion channel

**Next natural additions**, in likely order:
1. Fix the Windows decoder coverage gap (4624/4672) — see above.
2. Protect `GET /agents`.
3. Remove the deprecated query-param fallback on `/agent/config` once all
   deployed agents are confirmed on headers.
4. `detection/` — a first real rule (repeated failed logins from one
   source) has ready-made validation data waiting for it already.
5. TLS — real usernames, IPs, and domain names are now flowing over the
   wire in plaintext; this is no longer a someday item.
6. A defaults layer for remote config (group/tenant, not just per-agent
   and per-OS) — see Section 4B.6.
7. Alembic, once schema changes become frequent (arguably: already true).
8. `response/` actions.
9. Admin dashboard frontend.
10. Production-grade admin bootstrapping (`create_admin.py` is dev/testing only).
11. `api_key` at-rest protection on the endpoint's local identity file.