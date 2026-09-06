# Architecture

This document explains how the pieces of the EDR project fit together: the
Go agent, the FastAPI server, the React dashboard, and Postgres — and the
authentication model that connects all three.

---

## 1. System Overview

```
┌─────────────┐        HTTP/JSON        ┌──────────────────┐
│  Go Agent   │ ─────────────────────▶  │  FastAPI Server   │
│ (endpoint)  │ ◀───────────────────── │  (server/app/)      │
└─────────────┘                         └──────────┬─────────┘
                                                     │
┌─────────────┐        HTTP/JSON                   │  SQLAlchemy (async)
│  Dashboard  │ ─────────────────────▶ ─────────────┤
│  (React)    │ ◀─────────────────────              │
└─────────────┘                                     ▼
                                            ┌──────────────────┐
                                            │    PostgreSQL      │
                                            │  (erp / edr_project)│
                                            └──────────────────┘
```

Three independent clients talk to one FastAPI server:

- **The Go agent (KhemStrix agent)** — runs on a monitored endpoint.
  Registers once with an enrollment token, then heartbeats periodically
  using a permanent `api_key`. Now a real, compiled, tested binary
  (`agent/`, module `khemstrix-agent`) rather than a design-only plan.
- **The React dashboard** — runs in an admin's browser. Logs in as a human
  user (`users` table) and gets a JWT, used for every subsequent request.
- **PostgreSQL** — the single source of truth for all data. Both the agent
  and dashboard flows ultimately read/write the same tables.

A fourth, lighter-weight flow now exists too — the **binary distribution
path** (see section 5) — where a fresh, unregistered machine downloads the
agent binary directly from the server before it has any credentials at
all.

---

## 2. Request Flow Through the Server

Every request into the FastAPI server passes through the same layered
pipeline, regardless of whether it came from the Go agent or the dashboard:

```
Client (agent / dashboard / fresh install target)
        │
        ▼
[ Optional ] Authorization: Bearer <token> header
        │
        ▼
core/deps.py → get_current_user_id()   ← verifies JWT signature + expiry
        │        (only on protected routes)
        ▼
routers/*.py        ← receives HTTP request, validates input, runs logic
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
PostgreSQL
```

Every layer has ONE job. A router never builds raw SQL strings; a model
never contains business logic; a schema never touches the database
directly. This separation is what makes it possible to add new features
without breaking existing ones — see `developer.md` for the concrete
step-by-step process.

**Lesson learned this phase:** this layering is only as strong as its
weakest link — the `ip_address` column existed in Postgres, in
`schemas/agent.py`, and was referenced in `routers/agent.py`, but was
missing from `db/models.py`. SQLAlchemy silently dropped the value on
every insert, with no error anywhere in the stack. `/docs` still showed
the field being "accepted," because Pydantic validates and echoes
input/output independently of whether the ORM layer actually persists it.
**The only way this was caught was querying Postgres directly.** Treat a
direct `SELECT` against the real table as the actual proof a new field
works — not a clean-looking `/docs` response.

---

## 3. Authentication Model — Two Different Kinds of "Identity"

This project has two completely separate authentication systems, because it
has two completely different kinds of caller:

| | **Agents** (machines) | **Admin users** (humans) |
|---|---|---|
| Who | An unattended process running on a customer's endpoint | You, logging into the dashboard |
| How it proves identity | A long-lived, randomly generated `api_key`, sent with every request | A username + password login, exchanged for a short-lived JWT |
| Where credentials come from | Issued once by the server, at registration time | Chosen by the admin, stored as a bcrypt hash |
| Lifespan of the credential | Indefinite (until revoked/deleted) | JWT expires after 24 hours; must log in again |

### 3.1 Agent identity: enrollment token → api_key

An agent can't "log in" the way a human does — there's no one sitting at
the keyboard to type a password every time it phones home. So agent
authentication is split into two distinct secrets with two different
lifespans:

```
1. Admin generates a one-time enrollment token
      POST /admin/generate-token   (protected — requires admin JWT)
      → random token, valid 1 hour, single-use

2. Token is handed to the new machine — now via the real one-line
   install command generated in GenerateTokenModal.jsx, which
   downloads the agent binary AND runs it with the token in one step

3. Agent registers itself using that token
      POST /agent/register
      body: { hostname, os, enrollment_token, ip_address, mac_address }

      Server:
        - looks up the enrollment token
        - rejects if missing / expired / already used
        - generates a NEW random api_key (secrets.token_urlsafe(32))
        - creates the Agent row, storing that api_key + ip/mac address
        - marks the enrollment token as used (can't be reused)
        - returns { agent_id, api_key } — the ONE TIME the api_key
          is ever sent back to a client

4. Agent saves { agent_id, api_key } locally as its permanent credential
   (config.json, under ProgramData on Windows or /etc on Linux)

5. Every heartbeat afterward re-uses these saved credentials
      POST /agent/heartbeat
      body: { agent_id, api_key }

      Server checks:
        - does an agent with this agent_id exist?
        - does its stored api_key match?
        - is its status "active" (not "revoked")?
      Any failure → the same generic 401 "invalid credentials"
      (never reveals which check failed, so a probing attacker can't
      enumerate valid agent_ids or infer an agent's revoked status)
```

**IP + MAC collection, added this phase:** the Go agent's
`netinfo.go` walks the machine's network interfaces once, skips
loopback/disabled interfaces, and returns the IP and MAC from the *first*
active interface it finds — both from the *same* interface, guaranteeing
they describe one physical NIC rather than two mismatched ones. Both
fields are optional on the wire; a machine with unusual networking that
fails detection still registers successfully, just with `null` values.

**Why two separate secrets instead of one?** The enrollment token is
deliberately short-lived and single-use — if it leaks, it's worthless after
an hour and can't be reused to enroll a second, rogue device. The `api_key`
that actually matters long-term is never transmitted until the one moment
it's created, and from then on travels only between the agent and the
server.

**`api_key` is stored in plaintext** in `agents.api_key` — unlike
`users.password_hash`. This is intentional for now: heartbeat verification
does a direct string comparison, not a hash-and-compare like login does.
This is a known simplification worth hardening before any real deployment
(see `report.md` for the list of pre-production items).

### 3.2 Admin identity: username/password → JWT

```
1. Admin submits credentials
      POST /auth/login
      body: { username, password }

      Server:
        - looks up the user by username
        - hashes the submitted password (bcrypt) and compares to
          the stored hash
        - wrong username OR wrong password → same generic
          401 "invalid credentials" (never reveals which was wrong)
        - on success: signs a JWT containing { sub: user_id, exp: +24h }
          using a server-only secret (settings.JWT_SECRET_KEY, HS256)
        - returns { access_token, token_type: "bearer" }

2. Dashboard stores the token in localStorage (see 3.3 below)

3. Every subsequent request attaches it:
      Authorization: Bearer <token>

      core/deps.py → get_current_user_id():
        - missing header, or doesn't start with "Bearer " → 401
        - signature invalid or token expired → 401
        - otherwise → returns the user_id embedded in the token
```

Any route that should require login adds one line:
```python
current_user_id: str = Depends(get_current_user_id)
```

**Changing a password requires the current password**, even though the
caller already has a valid JWT (`PUT /auth/change-password`). A valid JWT
proves "who you were when the token was issued" — it does not prove you
should be allowed to make a sensitive account change right now.

**Logout is stateless.** JWTs aren't tracked server-side in a sessions
table — there's nothing to revoke. If a JWT is compromised, it remains
valid until it naturally expires (24 hours) — there is currently no
server-side revocation mechanism for admin tokens. (This is different from
agents, which do have a revoke *and* un-revoke mechanism — see 3.4.)

**There is no public signup route.** The first admin user is created by a
one-time script (`create_admin.py`), run manually — a development tool,
not a production onboarding flow.

### 3.3 How the dashboard uses the JWT and the server address

`dashboard/src/api.js` centralizes every backend call. A single `request()`
helper:
- Attaches `Authorization: Bearer <token>` automatically to any call marked
  `auth: true` (the default) — pulling the token from `localStorage`.
- Special-cases a `401` response: clears the stored token and throws a
  distinct `AuthError`, which every page catches to redirect back to
  `/login` via `forceLogout()`.

The same file also exports `API_URL` — the single source of truth for
where the FastAPI server lives, read from `VITE_API_URL` (falling back to
`http://localhost:8000` in dev). This value now does double duty: it's
used both for the dashboard's own API calls *and* to build the real
one-line install command in `GenerateTokenModal.jsx` (see section 5).
**These two uses have different correctness requirements** — the
`localhost` fallback is fine for the dashboard's own calls when viewed
locally, but is actively wrong for the install command, since that command
runs on a *different* machine, where `localhost` means "call yourself."
In practice this means `VITE_API_URL` should always be set explicitly to a
real, externally-reachable address (LAN IP in dev/testing, public domain
in production) as soon as the install command needs to leave the machine
running the dashboard.

### 3.4 Agent status: active, revoked, and back again

`agents.status` supports `"active"` and `"revoked"`. An admin can revoke
an agent via `PATCH /admin/agents/{agent_id}/revoke` — after that,
`POST /agent/heartbeat` will reject the agent's requests with the same
generic `401 invalid credentials` as an invalid key. This is the mechanism
for cutting off a compromised or decommissioned machine without needing to
delete its history.

**Un-revoke, added this phase**: `PATCH /admin/agents/{agent_id}/unrevoke`
sets status back to `"active"`. Nothing about the `api_key` changes at any
point in this cycle — revoke and un-revoke are purely a status flag flip.
Confirmed behavior: an agent that's still running (still attempting its
quiet-retry heartbeat loop even while revoked) picks the reactivation up
automatically on its very next heartbeat attempt, typically within the
~30s loop interval, with zero action needed on the agent side.

**Neither revoke nor delete stops the agent process on the actual
endpoint.** Both are purely server-side database operations. A revoked or
deleted agent's binary, if still running, will continue attempting
heartbeats indefinitely — quietly failing every cycle by design (the
agent's error-handling philosophy treats heartbeat failures as "log and
retry," never "crash"). This is a known, expected property of the current
design, not a bug — genuinely stopping a rogue or decommissioned agent
requires action on the physical machine itself (killing the process,
uninstalling, or — once built — a remote-kill capability, which doesn't
exist yet).

A separate `DELETE /admin/agents/{agent_id}` permanently removes the row —
this action has no "undo," unlike revoke.

---

## 4. Folder Responsibilities

| Path | Responsibility |
|---|---|
| `server/app/config.py` | Loads `.env`, exposes `settings.DATABASE_URL`, `settings.JWT_SECRET_KEY`. No logic. |
| `server/app/db/database.py` | Async engine, session factory, shared `Base`, `get_db()` dependency. No business logic. |
| `server/app/db/models.py` | SQLAlchemy classes mirroring Postgres tables. Structure only — no validation, no request handling. |
| `server/app/schemas/*.py` | Pydantic request/response shapes. Controls exactly what's accepted from clients and exposed back — including UTC-safe datetime serialization (see `api-contract.md`). |
| `server/app/core/security.py` | Password hashing (bcrypt via passlib) and JWT create/verify (python-jose). Pure functions — no DB access. |
| `server/app/core/deps.py` | `get_current_user_id` — the FastAPI dependency that guards protected routes. |
| `server/app/routers/agent.py` | Agent-facing lifecycle: register, heartbeat. Public routes, credential-based auth in the body. |
| `server/app/routers/admin.py` | Admin-facing agent management: generate-token, revoke, unrevoke, delete. All JWT-protected. |
| `server/app/routers/downloads.py` | **New this phase.** Public binary distribution: serves the compiled Go agent binaries for the one-line install flow. |
| `server/app/routers/auth.py` | Human login and password management. |
| `server/app/detection/` | (Planned) Turns raw event data into a `score`/`verdict`. |
| `server/app/response/` | (Planned) Takes a verdict and acts on it (isolate host, alert, etc.). |
| `server/app/static/binaries/` | **New this phase.** Compiled Go agent binaries served by `downloads.py`. Manually rebuilt/replaced — no automation yet. |
| `server/app/main.py` | Creates the FastAPI app, registers routers, CORS middleware. No business logic. |
| `agent/` (Go, module `khemstrix-agent`) | Compiles to a single binary; registers, heartbeats, collects IP/MAC. Fully built this phase (was empty scaffolding before). Runs in the foreground only — no background service support yet (see `report.md`). |
| `dashboard/` (React + Vite) | Admin-facing UI: login, agent list (with IP/MAC/actions), enrollment token generation with real install command. See `api-contract.md` for exactly which endpoints it currently calls. |
| `docs/` | This documentation set. |

---

## 5. Binary Distribution — the One-Line Install Flow

New this phase: a Wazuh-style single chained command that downloads and
immediately runs the agent, replacing the earlier two-step "download the
`.exe` manually, then run a separate command" placeholder.

```
1. Admin opens "Generate Enrollment Token" in the dashboard
      → POST /admin/generate-token fires automatically
      → GenerateTokenModal.jsx builds a real command using:
          - API_URL (the dashboard's own server address, from
            VITE_API_URL — must be reachable from the target machine,
            not "localhost")
          - the freshly generated, single-use token

2. Admin copies the command (Windows PowerShell or Linux bash,
   toggle in the modal) and runs it on the target machine

3. That command:
      a. Downloads the compiled binary from
         GET /download/agent/windows (or /linux) — public, no auth
      b. Immediately executes it with --server and --token flags

4. The downloaded binary follows the normal registration flow
   (section 3.1) — the download step and the registration step are
   independent; the binary itself doesn't know or care how it got
   onto the machine
```

**Binaries must be manually rebuilt and copied into
`static/binaries/`** any time the Go agent's source changes — there's no
CI/build pipeline connecting the two yet. A stale binary in that folder
will keep being served even after source changes, silently — worth a
manual checklist step ("did I rebuild and copy the binary?") any time
`agent/` code changes before testing a fresh install.

**Path resolution gotcha, fixed this phase:** `downloads.py` originally
built the binaries folder path as a string relative to the current working
directory, which broke depending on where `uvicorn` was launched from.
Fixed by resolving relative to the router file's own location via
`__file__`. This is now the pattern to follow for any future file-serving
code in this project — never assume a particular working directory.

**AV/heuristic flagging is expected, not a bug.** The compiled Windows
binary was flagged by 2 of 60 VirusTontal engines, both generic
ML-heuristic detections rather than signature matches. This is normal for
any new, unsigned, zero-reputation executable that behaves like a
monitoring agent (persistent background process, outbound network calls,
system info collection) — legitimate EDR tools face the same friction
before code-signing. Not a blocker for testing; a real requirement before
any production distribution.

---

## 6. Data Model Snapshot

**Tables in Postgres:** `enrollment_tokens`, `agents`, `users`.
`events` is defined in `db/models.py` but currently **commented out**
(along with the matching `Agent.events` relationship) — not yet created in
Postgres. See `developer.md` for the exact steps to bring it online when
Phase 1.1 (event pipeline) begins.

```
enrollment_tokens          agents                       users
─────────────────          ──────                       ─────
token (PK)          ◀────  enrollment_token (FK)         user_id (PK)
created_at                 agent_id (PK)                 username (unique)
expires_at                 hostname                      password_hash
used                       os                             created_at
                           api_key (unique)
                           status ("active"/"revoked")
                           created_at
                           last_seen_at
                           ip_address    ← added this phase
                           mac_address   ← added this phase
```

**Why `enrollment_token` isn't the primary key of `agents`:** it's a
foreign key used purely for audit trail (which token authorized this
agent's enrollment) — the agent's real identity going forward is
`agent_id` + `api_key`.

**Both `ip_address` and `mac_address` are nullable text columns**, added
following the "Modifying an Existing Table" checklist in `developer.md`
§3 — Postgres first, then `models.py`, then `schemas/agent.py`, then
`routers/agent.py`. Both are optional by design: existing rows predating
the change have no way to retroactively gain a value, and detection can
legitimately fail on some network configurations.