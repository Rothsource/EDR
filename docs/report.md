# Project Status Report

Snapshot as of early Phase 1.0 development. This reflects the actual state
of the codebase — not the target state. Cross-reference `api-contract.md`
for exact endpoint shapes and `developer.md` for how to pick up any of the
"not yet done" items below.

---

## ✅ Done and Tested

### Backend — core connectivity (Phase 1.0)
- Postgres schema: `enrollment_tokens`, `agents`, `users` tables created
  and verified via `psql`
- `config.py` → `db/database.py` → `db/models.py` chain confirmed working
  end-to-end (tested via a throwaway async query script before any routes
  existed)
- Timezone handling bug (naive vs. aware `datetime` on `timestamp without
  time zone` columns) found and fixed — documented in `developer.md` so it
  isn't reintroduced in new endpoints

### Backend — agent lifecycle
- `POST /admin/generate-token` — protected, generates a 1-hour single-use
  enrollment token
- `POST /agent/register` — validates the enrollment token (missing /
  expired / already-used all handled with distinct error messages),
  generates a permanent `api_key`, creates the `Agent` row, marks the
  token used — all in one transaction
- `POST /agent/heartbeat` — validates `agent_id` + `api_key` + active
  status, updates `last_seen_at`
- `GET /agents` — protected, lists all agents, correctly excludes `api_key`
- `PATCH /admin/agents/{agent_id}/revoke` — sets an agent's status to
  `"revoked"`, tested via `/docs`
- `DELETE /admin/agents/{agent_id}` — hard-deletes an agent row, tested via
  `/docs`

### Backend — admin authentication
- `users` table + `core/security.py` (bcrypt password hashing, JWT create/
  decode via python-jose, `HS256`, 24h expiry)
- `core/deps.py` — `get_current_user_id` dependency, guards protected
  routes via `Authorization: Bearer <token>`
- `POST /auth/login` — generic `401` for both wrong username and wrong
  password
- `PUT /auth/change-password` — requires current password even with a
  valid JWT already presented
- `create_admin.py` — one-time script to bootstrap the first admin user
  (dev/testing tool only — see "Known Gaps" below)

### Dashboard (React + Vite)
- Login page + `AuthContext` — stores JWT in `localStorage`, exposes
  `login()` / `logout()` / `forceLogout()`
- `api.js` — centralized request helper; auto-attaches the JWT to every
  authenticated call; a `401` response automatically clears the token and
  triggers logout via a dedicated `AuthError`
- `Agents.jsx` — lists all agents (hostname, OS, enrolled date, last seen,
  online/offline badge computed client-side), searchable by hostname
- `GenerateTokenModal.jsx` — generates a token on open, shows it with a
  copy button, and displays a ready-to-run agent install command
- Route protection (`ProtectedRoute.jsx`) on `/`, `/agents`, `/settings`

---

## 🟡 Built on the Backend, Not Yet Connected to the Dashboard

**This is the main gap right now, and it's explicitly called out here so
it doesn't get lost:**

- **`PATCH /admin/agents/{agent_id}/revoke`** — endpoint works, tested via
  `/docs`. The `Agents.jsx` page has no revoke button, and `api.js` has no
  corresponding function.
- **`DELETE /admin/agents/{agent_id}`** — same situation: endpoint works,
  nothing in the UI calls it yet.

**What's needed to close this gap** (see `developer.md` §4 for the general
pattern):
1. Add two functions to `dashboard/src/api.js`:
   ```js
   revokeAgent: (agentId) =>
     request(`/admin/agents/${agentId}/revoke`, { method: "PATCH" }),
   deleteAgent: (agentId) =>
     request(`/admin/agents/${agentId}`, { method: "DELETE" }),
   ```
2. Add action buttons (e.g. a per-row menu or icon buttons) in
   `Agents.jsx`'s table
3. Add a confirmation step before delete, since it's a permanent,
   irreversible action (revoke is reversible in principle — nothing
   currently un-revokes an agent either, worth deciding if that's needed)
4. Refresh the agent list (`loadAgents()`) after either action succeeds

This is a frontend-only task at this point — no backend changes required.

- **`PUT /auth/change-password`** — wired into `api.js`, but it hasn't been
  confirmed whether any page (e.g. `Settings.jsx`) actually has a form that
  calls it yet. Worth a quick check before assuming this is fully done
  end-to-end.

---

## ⬜ Not Started

- **Event pipeline** (`events` table, `routers/events.py`,
  `detection/dispatcher.py`, per-type scorers) — the `Event` model is fully
  written in `db/models.py` but commented out. This is Phase 1.1 per the
  roadmap. See `developer.md` §7 for the exact bring-up steps.
- **Response actions** (`response/actions.py`) — Phase 3 per the roadmap;
  no code written yet.
- **Agent-side Go modules** beyond the connectivity flow — `email/`,
  `process/`, `network/`, `file/`, `response/` module folders exist in the
  planned structure but implementation status wasn't part of this review;
  confirm separately against the Go codebase.
- **Alembic migrations** — schema changes are currently applied by hand
  (manual `CREATE TABLE`/`ALTER TABLE` in psql, then update `models.py` to
  match). Fine at current scale; revisit once schema changes become
  frequent or a second environment is stood up.

---

## Known Gaps / Pre-Production Hardening Items

These are acceptable simplifications for a local student prototype but
should be addressed before any real SME deployment:

- **`api_key` is stored in plaintext** in `agents.api_key`. Heartbeat
  verification does a direct string comparison rather than a hash-and-
  compare. Consider hashing this the same way `password_hash` is handled,
  if/when this matters for the threat model.
- **No server-side JWT revocation.** Logout is purely client-side
  (`localStorage.removeItem`). A leaked/stolen admin JWT remains valid
  until its natural 24-hour expiry. A token blocklist would close this gap
  if needed later.
- **`create_admin.py` is a dev/testing tool**, not a production onboarding
  flow. Before a real deployment, replace or supplement it with one of:
  auto-generating a random admin password on first startup (printed once
  to console/logs), a `must_change_password` flag forcing a change on
  first login, or a first-run setup wizard in the dashboard.
- **No HTTPS** — all traffic (agent↔server, dashboard↔server) is currently
  plain HTTP, fine for local network testing, required before any real
  deployment.
- **No multi-tenancy (`tenant_id`)** — noted in the original project README
  as something that's painful to retrofit later. Not present on `agents`
  or (planned) `events` yet. Worth adding before there's more than one
  customer's data in the same database.
- **CORS is currently locked to `http://localhost:5173`** (the Vite dev
  server) in `main.py` — will need updating when the dashboard is built
  and deployed anywhere else.

---

## Suggested Next Steps (in priority order)

1. Wire the dashboard to the existing `revoke`/`delete` endpoints (closes
   the biggest done-but-disconnected gap, no backend work needed)
2. Confirm/complete the `change-password` UI in `Settings.jsx`
3. Bring the `events` table online and build a minimal ingestion endpoint,
   even before real detection logic exists — unblocks testing the agent's
   event-sending path independently of scoring
4. Begin Phase 1.1 detection logic (email phishing scorer) once events are
   flowing