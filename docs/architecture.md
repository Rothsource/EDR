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

- **The Go agent** — runs on a monitored endpoint. Registers once with an
  enrollment token, then heartbeats periodically using a permanent `api_key`.
- **The React dashboard** — runs in an admin's browser. Logs in as a human
  user (`users` table) and gets a JWT, used for every subsequent request.
- **PostgreSQL** — the single source of truth for all data. Both the agent
  and dashboard flows ultimately read/write the same tables.

---

## 2. Request Flow Through the Server

Every request into the FastAPI server passes through the same layered
pipeline, regardless of whether it came from the Go agent or the dashboard:

```
Client (agent / dashboard)
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

2. Token is handed to the new machine (via the dashboard's
   "Run this on the target machine" command, or manually)

3. Agent registers itself using that token
      POST /agent/register
      body: { hostname, os, enrollment_token }

      Server:
        - looks up the enrollment token
        - rejects if missing / expired / already used
        - generates a NEW random api_key (secrets.token_urlsafe(32))
        - creates the Agent row, storing that api_key
        - marks the enrollment token as used (can't be reused)
        - returns { agent_id, api_key } — the ONE TIME the api_key
          is ever sent back to a client

4. Agent saves { agent_id, api_key } locally as its permanent credential

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

**Why two separate secrets instead of one?** The enrollment token is
deliberately short-lived and single-use — if it leaks, it's worthless after
an hour and can't be reused to enroll a second, rogue device. The `api_key`
that actually matters long-term is never transmitted until the one moment
it's created, and from then on travels only between the agent and the
server.

**Where `api_key` is generated:**
```python
new_api_key = secrets.token_urlsafe(32)
```
`secrets.token_urlsafe()` uses Python's cryptographically secure random
generator (not the predictable `random` module) — the same approach used
for enrollment tokens, so both secrets are equally unguessable.

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
should be allowed to make a sensitive account change right now. Requiring
the current password protects against a scenario where a token leaked (e.g.
copied from browser storage) but the attacker doesn't actually know the
account password.

**Logout is stateless.** JWTs aren't tracked server-side in a sessions
table — there's nothing to revoke. `logout()` in the dashboard's
`AuthContext.jsx` simply calls `clearToken()`, deleting the token from
`localStorage`. If a JWT is compromised, it remains valid until it
naturally expires (24 hours) — there is currently no server-side
revocation mechanism for admin tokens. (This is different from agents,
which do have a revoke mechanism — see 3.4.)

**There is no public signup route.** The first admin user is created by a
one-time script (`create_admin.py`), run manually. This is a development
tool, not a production onboarding flow — see `report.md`/`developer.md` for
what a real deployment would need instead.

### 3.3 How the dashboard uses the JWT

`dashboard/src/api.js` centralizes every backend call. A single `request()`
helper:
- Attaches `Authorization: Bearer <token>` automatically to any call marked
  `auth: true` (the default) — pulling the token from `localStorage`.
- Special-cases a `401` response: clears the stored token and throws a
  distinct `AuthError`, which every page catches to redirect back to
  `/login` via `forceLogout()`.

This means individual pages/components never manually attach the header or
handle expiry — it's centralized once in `api.js`.

### 3.4 Agent status: active vs. revoked

`agents.status` currently supports `"active"` and `"revoked"`. An admin can
revoke an agent via `PATCH /admin/agents/{agent_id}/revoke` — after that,
`POST /agent/heartbeat` will reject the agent's requests with the same
generic `401 invalid credentials` as an invalid key (see 3.1). This is the
mechanism for cutting off a compromised or decommissioned machine without
needing to delete its history. A separate `DELETE /admin/agents/{agent_id}`
permanently removes the row. **Both endpoints exist and are tested on the
backend; the dashboard UI does not yet call either one** — see `report.md`.

---

## 4. Folder Responsibilities

| Path | Responsibility |
|---|---|
| `server/app/config.py` | Loads `.env`, exposes `settings.DATABASE_URL`, `settings.JWT_SECRET_KEY`. No logic. |
| `server/app/db/database.py` | Async engine, session factory, shared `Base`, `get_db()` dependency. No business logic. |
| `server/app/db/models.py` | SQLAlchemy classes mirroring Postgres tables. Structure only — no validation, no request handling. |
| `server/app/schemas/*.py` | Pydantic request/response shapes. Controls exactly what's accepted from clients and exposed back — deliberately narrower than the full DB model where needed (e.g. excluding `api_key`, `password_hash`). |
| `server/app/core/security.py` | Password hashing (bcrypt via passlib) and JWT create/verify (python-jose). Pure functions — no DB access. |
| `server/app/core/deps.py` | `get_current_user_id` — the FastAPI dependency that guards protected routes. |
| `server/app/routers/*.py` | Actual endpoint logic: reading input, querying/writing the DB, business rules, shaping responses. |
| `server/app/detection/` | (Planned) Turns raw event data into a `score`/`verdict`. |
| `server/app/response/` | (Planned) Takes a verdict and acts on it (isolate host, alert, etc.). |
| `server/app/main.py` | Creates the FastAPI app, registers routers, CORS middleware. No business logic. |
| `agent/` (Go) | Compiles to a single binary; registers, heartbeats, and (in later phases) collects/sends events. |
| `dashboard/` (React + Vite) | Admin-facing UI: login, agent list, enrollment token generation. See `api-contract.md` for exactly which endpoints it currently calls. |
| `docs/` | This documentation set. |

---

## 5. Data Model Snapshot

**Tables in Postgres:** `enrollment_tokens`, `agents`, `users`.
`events` is defined in `db/models.py` but currently **commented out**
(along with the matching `Agent.events` relationship) — not yet created in
Postgres. See `developer.md` for the exact steps to bring it online when
Phase 1.1 (event pipeline) begins.

```
enrollment_tokens          agents                      users
─────────────────          ──────                      ─────
token (PK)          ◀────  enrollment_token (FK)        user_id (PK)
created_at                 agent_id (PK)                username (unique)
expires_at                 hostname                     password_hash
used                       os                           created_at
                           api_key (unique)
                           status ("active"/"revoked")
                           created_at
                           last_seen_at
```

**Why `enrollment_token` isn't the primary key of `agents`:** it's a
foreign key used purely for audit trail (which token authorized this
agent's enrollment) — the agent's real identity going forward is
`agent_id` + `api_key`.