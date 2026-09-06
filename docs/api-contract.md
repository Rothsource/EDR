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
**Called by dashboard:** Yes — `api.changePassword()` (wired in `api.js`; still
not confirmed which page's UI triggers it — verify `Settings.jsx` if
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
  "enrollment_token": "string",
  "ip_address": "string | null",
  "mac_address": "string | null"
}
```
`ip_address` and `mac_address` are both optional. The Go agent detects both
from the same active, non-loopback network interface (so they describe the
same physical NIC) and sends them if detection succeeds; `null`/omitted if
not.

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
**Called by:** The Go agent binary, periodically (every ~30s)

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
cases (unknown `agent_id`, wrong `api_key`, or `status != "active"` —
i.e. including a revoked agent):
```json
{ "detail": "invalid credentials" }
```
A revoked agent's `api_key` is untouched and still matches — the rejection
comes purely from the status check, but the identical response means the
agent (or anyone probing) can't distinguish "revoked" from "wrong key" from
"never existed."

Side effect on success: `agents.last_seen_at` is updated to the current
UTC time. **Not** updated on any failure path, including revoked agents —
so `last_seen_at` effectively freezes at the last successful check-in once
an agent is revoked or its credentials otherwise stop working.

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
    "ip_address": "string | null",
    "mac_address": "string | null",
    "created_at": "2026-09-06T15:26:06.789975+00:00",
    "last_seen_at": "2026-09-06T15:26:06.789975+00:00"
  }
]
```
`last_seen_at` may be `null` if the agent has registered but never
heartbeated. `ip_address`/`mac_address` may be `null` for agents registered
before the Go agent supported detecting them, or if detection failed on
that machine.

**Timestamp format note:** `created_at` and `last_seen_at` are explicitly
serialized with a UTC offset (`+00:00`) via a `@field_serializer` on
`AgentResponse`, even though they're stored as naive `timestamp without
time zone` values in Postgres. This was a real bug earlier in development —
without the explicit UTC stamp, browsers outside UTC would misinterpret
"just happened" timestamps as being hours off. Any new response schema
returning a `datetime` field should follow this same pattern.

**Dashboard-side note:** the online/offline/revoked badge shown in
`Agents.jsx` (`StatusBadge`, via `getAgentState()` in `agentStatus.js`) is
computed **client-side only**, using a 90-second window against
`last_seen_at`, with `status === "revoked"` checked first and taking
priority over the time-based check. This is a UI convenience, not a value
stored in or returned by this endpoint — the backend's own
heartbeat-staleness threshold (`HEARTBEAT_THRESHOLD_SECONDS = 60`, defined
in `routers/agent.py`) remains unused by any endpoint and exists only as a
placeholder constant.

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
the moment it opens, displays the token with a copy button, and renders a
real, ready-to-run chained install command (download + execute in one
step), toggleable between Windows and Linux:

```powershell
Invoke-WebRequest -Uri "<API_URL>/download/agent/windows" -OutFile "$env:TEMP\khemstrixAgent.exe"; & "$env:TEMP\khemstrixAgent.exe" --server=<API_URL> --token=<token>
```
```bash
curl -o /tmp/khemstrixAgent <API_URL>/download/agent/linux && chmod +x /tmp/khemstrixAgent && /tmp/khemstrixAgent --server=<API_URL> --token=<token>
```
`<API_URL>` is the dashboard's own `VITE_API_URL` env value (exported from
`api.js`) — no more manual placeholder editing required. This must be set
to a real, LAN- or internet-reachable address (not `localhost`, which
would tell the *target* machine to call itself) — see `report.md` for the
bug this caused during testing.

---

### `PATCH /admin/agents/{agent_id}/revoke`
**Protected:** Yes
**Called by dashboard:** Yes — `api.revokeAgent()`, wired into `Agents.jsx`'s
Actions column

Path parameter: `agent_id` (UUID)
No request body.

Success response `200` (`AgentActionResponse`):
```json
{ "status": "revoked" }
```
Side effect: sets `agents.status = "revoked"` for that row. The `api_key`
itself is untouched — a revoked agent's credentials are still technically
correct, they're just no longer authorized. A revoked agent's subsequent
`POST /agent/heartbeat` calls fail with the generic `401 invalid
credentials` (see above). The agent process itself is not notified or
stopped — it will keep retrying its heartbeat loop indefinitely, failing
quietly each time, until either the agent is un-revoked or the process is
manually stopped on the endpoint.

Error responses:
```json
404 { "detail": "agent not found" }
500 { "detail": "failed to revoke agent" }
```

---

### `PATCH /admin/agents/{agent_id}/unrevoke`
**Protected:** Yes
**Called by dashboard:** Yes — `api.unrevokeAgent()`, wired into
`Agents.jsx`'s Actions column (shown as "Reactivate" when an agent's
status is `"revoked"`)

Path parameter: `agent_id` (UUID)
No request body.

Success response `200` (`AgentActionResponse`):
```json
{ "status": "active" }
```
Side effect: sets `agents.status = "active"` for that row. Reuses the same
`AgentActionResponse` schema as revoke/delete. Reversal is transparent to
the agent — the same `api_key` it already has saved locally starts working
again on its very next heartbeat attempt, with no re-registration and no
new token required.

Error responses:
```json
404 { "detail": "agent not found" }
500 { "detail": "failed to unrevoke agent" }
```

---

### `DELETE /admin/agents/{agent_id}`
**Protected:** Yes
**Called by dashboard:** Yes — `api.deleteAgent()`, wired into `Agents.jsx`'s
Actions column with an inline two-step confirmation before the request fires

Path parameter: `agent_id` (UUID)
No request body.

Success response `200` (`AgentActionResponse`):
```json
{ "status": "deleted" }
```
Side effect: permanently removes the row from `agents`. This is a hard
delete — there is no soft-delete/undo. As with revoke, the agent process
itself is unaffected and will continue retrying its heartbeat loop,
failing with `401` indefinitely, since its `agent_id` no longer exists at
all.

Error responses:
```json
404 { "detail": "agent not found" }
500 { "detail": "failed to delete agent" }
```

---

## Download Routes — `routers/downloads.py` (no shared prefix)

### `GET /download/agent/windows`
**Protected:** No — the caller is a fresh, unregistered machine; the
binary itself contains no secrets
**Called by:** The install one-liner generated in `GenerateTokenModal.jsx`,
or manually via browser/curl

No request body, no parameters.

Success response `200`: the compiled `khemstrixAgent.exe` binary, served
via `FileResponse` with `Content-Disposition: attachment` and
`media_type=application/octet-stream`.

Error response `404`:
```json
{ "detail": "agent binary not available" }
```
Returned if no compiled binary currently exists at
`server/app/static/binaries/khemstrixAgent.exe`.

**Operational note:** the binary must be manually rebuilt (`go build`) and
copied into `static/binaries/` every time the Go agent source changes —
there is no build automation for this yet. The path is resolved relative
to the router file's own location (via `__file__`), not the current
working directory, to avoid breaking depending on where `uvicorn` is
launched from.

---

### `GET /download/agent/linux`
**Protected:** No
**Called by:** Same as above, Linux variant

Success response `200`: the compiled `khemstrixAgent` binary (no
extension), same `FileResponse` pattern as the Windows route.

Error response `404`: same shape as above, same cause (binary not present
at `static/binaries/khemstrixAgent`).

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
| PUT | `/auth/change-password` | Yes | ✅ (in `api.js`; UI trigger still unconfirmed) |
| POST | `/agent/register` | No (token-based) | N/A — agent-only |
| POST | `/agent/heartbeat` | No (key-based) | N/A — agent-only |
| GET | `/agents` | Yes | ✅ |
| POST | `/admin/generate-token` | Yes | ✅ |
| PATCH | `/admin/agents/{agent_id}/revoke` | Yes | ✅ |
| PATCH | `/admin/agents/{agent_id}/unrevoke` | Yes | ✅ |
| DELETE | `/admin/agents/{agent_id}` | Yes | ✅ |
| GET | `/download/agent/windows` | No | N/A — install-command target |
| GET | `/download/agent/linux` | No | N/A — install-command target |
| GET | `/` | No | N/A |

---

## Not Yet Implemented (planned, per roadmap)

- `POST /agent/events` and `GET /events` — generic event ingestion.
  `db/models.py` has `Event` fully written but commented out; no router
  file exists yet. See `developer.md` §7 for the exact steps to bring
  this online. **Note for whenever this is built:** it should apply the
  same `agent_id` + `api_key` + `status == "active"` check that
  `/agent/heartbeat` already does — otherwise a revoked agent could still
  push event data through even though its heartbeats correctly fail.