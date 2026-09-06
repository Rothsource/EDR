# API Contract

This is the source of truth for every endpoint currently implemented on the
FastAPI server. Every shape below is taken directly from the actual
`schemas/*.py` and `routers/*.py` files — not aspirational.

**Base URL (local dev):** `http://localhost:8000`
**Auth header format (protected routes):** `Authorization: Bearer <jwt>`

---

## Auth Routes — `routers/auth.py` (mounted at `/auth`)

### `POST /auth/login`
**Protected:** No
**Called by dashboard:** Yes — `api.login()`, used in `Login.jsx` via `AuthContext`

Request body (`LoginRequest`):
```json
{
  "username": "string",
  "password": "string"
}
```

Success response `200` (`TokenResponse`):
```json
{
  "access_token": "string",
  "token_type": "bearer"
}
```

Error response `401`:
```json
{ "detail": "invalid credentials" }
```
Returned identically whether the username doesn't exist or the password is
wrong — deliberate, to avoid leaking which one was incorrect.

---

### `PUT /auth/change-password`
**Protected:** Yes
**Called by dashboard:** Yes — `api.changePassword()` (wired in `api.js`; not
yet confirmed which page's UI triggers it — verify `Settings.jsx` if
building out a password-change form there)

Request body (`ChangePasswordRequest`):
```json
{
  "current_password": "string",
  "new_password": "string"
}
```

Success response `200`:
```json
{ "status": "password updated" }
```

Error responses:
- `401 {"detail": "not authenticated"}` — missing/invalid/expired JWT, or
  (edge case) the user tied to the token no longer exists
- `401 {"detail": "current password is incorrect"}`
- `500 {"detail": "failed to update password"}` — DB write failure

---

## Agent Routes — `routers/agent.py` (no shared prefix; each route specifies its full path)

### `POST /agent/register`
**Protected:** No (authenticates via the enrollment token in the body instead of a JWT)
**Called by:** The Go agent binary, not the dashboard

Request body (`AgentCreate`):
```json
{
  "hostname": "string",
  "os": "string",
  "enrollment_token": "string"
}
```

Success response `200` (`AgentRegisterResponse`) — **the only time `api_key`
is ever returned**:
```json
{
  "agent_id": "uuid",
  "api_key": "string"
}
```

Error responses (all `400`):
```json
{ "detail": "invalid token" }
{ "detail": "token expired" }
{ "detail": "token already used" }
```

---

### `POST /agent/heartbeat`
**Protected:** No (authenticates via `agent_id` + `api_key` in the body)
**Called by:** The Go agent binary, periodically

Request body (`AgentHeartbeat`):
```json
{
  "agent_id": "uuid",
  "api_key": "string"
}
```

Success response `200`:
```json
{ "status": "ok" }
```

Error response `401` — returned identically for **all three** failure
cases (unknown `agent_id`, wrong `api_key`, or `status != "active"`):
```json
{ "detail": "invalid credentials" }
```

Side effect on success: `agents.last_seen_at` is updated to the current
UTC time.

---

### `GET /agents`
**Protected:** Yes
**Called by dashboard:** Yes — `api.listAgents()`, used in `Agents.jsx`

No request body.

Success response `200` (`list[AgentResponse]`) — **excludes `api_key`**:
```json
[
  {
    "agent_id": "uuid",
    "hostname": "string",
    "os": "string",
    "status": "active",
    "created_at": "2026-09-06T03:00:00Z",
    "last_seen_at": "2026-09-06T03:58:12Z"
  }
]
```
`last_seen_at` may be `null` if the agent has registered but never
heartbeated.

**Dashboard-side note:** the "online/offline" badge shown in `Agents.jsx`
(`StatusBadge`) is computed **client-side only**, in `agentStatus.js`,
using a 90-second window against `last_seen_at`. This is a UI convenience,
not a value stored in or returned by this endpoint — the backend's own
heartbeat-staleness threshold (`HEARTBEAT_THRESHOLD_SECONDS = 60`, defined
in `routers/agent.py`) is currently unused by any endpoint and exists only
as a placeholder constant.

---

## Admin Routes — `routers/admin.py` (mounted at `/admin`)

### `POST /admin/generate-token`
**Protected:** Yes
**Called by dashboard:** Yes — `api.generateToken()`, used in
`GenerateTokenModal.jsx`

No request body.

Success response `200` (`TokenResponse`):
```json
{
  "token": "string",
  "expires_at": "2026-09-06T04:48:43.951152Z"
}
```
Token is valid for 1 hour (`TOKEN_VALIDITY_DURATION` in `admin.py`) and
single-use.

Error response `500`:
```json
{ "detail": "failed to create enrollment token" }
```

**Dashboard behavior:** `GenerateTokenModal.jsx` calls this automatically
the moment it opens, displays the token with a copy button, and also
renders a ready-to-run install command:
```
edr-agent.exe --server=http://<your-server-ip>:8000 --token={token}
```
The `<your-server-ip>` placeholder is not currently auto-filled with the
real server address — it's shown literally, for the admin to replace by
hand.

---

### `PATCH /admin/agents/{agent_id}/revoke`
**Protected:** Yes
**Called by dashboard:** **Not yet** — endpoint is built and testable via
`/docs`, but no button or `api.js` function currently calls it. See
`report.md`.

Path parameter: `agent_id` (UUID)
No request body.

Success response `200` (`AgentActionResponse`):
```json
{ "status": "revoked" }
```
Side effect: sets `agents.status = "revoked"` for that row. A revoked
agent's subsequent `POST /agent/heartbeat` calls will fail with the generic
`401 invalid credentials` (see above).

Error responses:
```json
404 { "detail": "agent not found" }
500 { "detail": "failed to revoke agent" }
```

---

### `DELETE /admin/agents/{agent_id}`
**Protected:** Yes
**Called by dashboard:** **Not yet** — endpoint is built and testable via
`/docs`, but no button or `api.js` function currently calls it. See
`report.md`.

Path parameter: `agent_id` (UUID)
No request body.

Success response `200` (`AgentActionResponse`):
```json
{ "status": "deleted" }
```
Side effect: permanently removes the row from `agents`. This is a hard
delete — there is no soft-delete/undo.

Error responses:
```json
404 { "detail": "agent not found" }
500 { "detail": "failed to delete agent" }
```

---

## Utility Route

### `GET /`
**Protected:** No

Success response `200`:
```json
{ "status": "running" }
```
Basic liveness check — not part of the versioned API contract, just
confirms the server process is up.

---

## Summary Table

| Method | Path | Protected | Dashboard wired? |
|---|---|---|---|
| POST | `/auth/login` | No | ✅ |
| PUT | `/auth/change-password` | Yes | ✅ (in `api.js`; confirm UI trigger) |
| POST | `/agent/register` | No (token-based) | N/A — agent-only |
| POST | `/agent/heartbeat` | No (key-based) | N/A — agent-only |
| GET | `/agents` | Yes | ✅ |
| POST | `/admin/generate-token` | Yes | ✅ |
| PATCH | `/admin/agents/{agent_id}/revoke` | Yes | ❌ **not yet wired** |
| DELETE | `/admin/agents/{agent_id}` | Yes | ❌ **not yet wired** |
| GET | `/` | No | N/A |

---

## Not Yet Implemented (planned, per roadmap)

- `POST /agent/events` and `GET /events` — generic event ingestion.
  `db/models.py` has `Event` fully written but commented out; no router
  file exists yet. See `developer.md` §7 for the exact steps to bring
  this online.