# Developer Guide — Adding Features & Scaling

This is your working playbook for extending the project. Read `architecture.md`
first for the "why"; this doc is the "how" — the exact checklist to follow
every time you add a table, an endpoint, or a protected feature.

---

## 1. The Golden Rule

**Every layer has one job. Never let a layer do another layer's job.**

| If you're tempted to... | ...it actually belongs in |
|---|---|
| Write an `if` statement in `models.py` | `routers/*.py` or a service module |
| Build a SQL string with f-strings/concatenation | Use SQLAlchemy's `select(...).where(...)` instead — always |
| Put request validation in a router | `schemas/*.py` |
| Put scoring/response logic directly in a router | `detection/` or `response/` |
| Expose a secret (`api_key`, `password_hash`) in a general response schema | A dedicated one-time response schema, or never |

---

## 2. Checklist: Adding a New Table

Follow these steps **in this exact order** — each one depends on the last.

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
Verify with `\dt` and `\d alerts` in psql before moving on.

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
If you add a `relationship(...)`, add the reverse side on the related model
too (`alerts = relationship("Alert", back_populates="agent")` on `Agent`).

**Gotcha already hit once in this project:** if a model class is commented
out (like `Event` currently is), any `relationship(...)` elsewhere pointing
at it must be commented out too — SQLAlchemy fails at mapper-configuration
time with a confusing error if a relationship references an unregistered
class name. When you uncomment `Event`, remember to uncomment
`Agent.events` in the same commit.

### Step 3 — Add schemas in `schemas/alert.py`
Ask: what should a client be allowed to **send**, and what should they be
allowed to **see back**? These are often NOT identical to the full model —
e.g. `agent_id` might be set by the URL path rather than the request body,
or a field might exist in the DB but never need to be exposed.
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
```python
from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession
from db.database import get_db
from db.models import Alert
from schemas.alert import AlertCreate, AlertResponse
from core.deps import get_current_user_id

router = APIRouter()

@router.post("/alerts", response_model=AlertResponse)
async def create_alert(
    payload: AlertCreate,
    db: AsyncSession = Depends(get_db),
    current_user_id: str = Depends(get_current_user_id),  # omit if this route should be public
):
    new_alert = Alert(**payload.model_dump())
    db.add(new_alert)
    await db.commit()
    await db.refresh(new_alert)
    return new_alert
```

**Decide public vs. protected up front.** Agent-facing routes
(`/agent/register`, `/agent/heartbeat`) are public because the agent
authenticates via its own credentials in the body, not a JWT. Admin-facing
routes (anything the dashboard calls to manage data) should almost always
include `Depends(get_current_user_id)`.

### Step 5 — Wire the router into `main.py`
```python
from routers import alerts
app.include_router(alerts.router, tags=["alerts"])
```

### Step 6 — Test via `/docs`
Run the server, open `http://localhost:8000/docs`, exercise the new
endpoint(s) manually before building anything on top of them. For protected
routes, test **both** without and with a valid `Authorization: Bearer
<token>` header — confirm you get `401 not authenticated` in the first case.
(Note: since auth here uses a plain `Header(...)` dependency rather than
FastAPI's `OAuth2PasswordBearer` scheme, the Swagger "Authorize" lock icon
won't auto-attach the header — set it manually per request in `/docs`, or
test with curl/Postman.)

### Step 7 — Wire it into the dashboard (if applicable)
1. Add a function to `dashboard/src/api.js`:
   ```js
   createAlert: (agent_id, severity, message) =>
     request("/alerts", { method: "POST", body: { agent_id, severity, message } }),
   ```
2. Call it from the relevant page/component, following the existing pattern
   in `Agents.jsx` / `GenerateTokenModal.jsx`: `try/catch`, check
   `err instanceof AuthError` → `forceLogout()`, otherwise show `err.message`.

---

## 3. Checklist: Modifying an Existing Table

Example: adding `ip_address` to `agents`.

1. **Postgres:** `ALTER TABLE agents ADD COLUMN ip_address text;`
2. **`db/models.py`:** add `ip_address = Column(Text)` to `Agent`
3. **`schemas/agent.py`:** add `ip_address: Optional[str] = None` to
   `AgentResponse` (and to `AgentCreate` only if clients should set it
   themselves)
4. **`routers/agent.py`:** update logic if the new field needs to be
   captured/used somewhere
5. **`dashboard/src/pages/Agents.jsx`:** add a column if it should be
   visible in the table

**Order matters — always Postgres first**, then work back up through the
layers. SQLAlchemy does not auto-sync with the database; you keep
`models.py` in sync by hand (or via Alembic — see section 6).

---

## 4. Checklist: Adding a New Protected Endpoint (No New Table)

Sometimes you just need a new action on existing data (like `revoke` and
`delete` were added to `agents` without a new table):

1. Add the schema if the response shape is new (e.g. `AgentActionResponse`
   was added for revoke/delete — a minimal `{ status: str }` shape, reused
   for both actions rather than writing two nearly-identical schemas).
2. Add the route to the relevant router:
   ```python
   @router.patch("/agents/{agent_id}/revoke", response_model=AgentActionResponse)
   async def revoke_agent(
       agent_id: UUID,
       db: AsyncSession = Depends(get_db),
       current_user_id: str = Depends(get_current_user_id),
   ):
       ...
   ```
3. Test via `/docs` first — confirm the backend works in isolation before
   touching the dashboard.
4. Only then, wire it into `api.js` and the relevant page.

**This exact situation exists right now in this project**: `PATCH
/admin/agents/{agent_id}/revoke` and `DELETE /admin/agents/{agent_id}` are
built and tested on the backend, but step 4 (dashboard wiring) hasn't
happened yet. See `report.md` for tracking this as an open item — this
section is the checklist to follow when you pick it back up.

---

## 5. Rules of Thumb As You Scale

- **`models.py` = structure only.** No logic, no validation, no request
  handling.
- **`schemas/` = contract with the outside world.** Never expose
  `api_key` or `password_hash` in a general-purpose response schema — only
  in a dedicated one-time response schema (`AgentRegisterResponse`), or
  never at all.
- **Routers should stay thin.** If a router function starts accumulating
  scoring logic or response-action logic, pull it out into `detection/` or
  `response/` and have the router just call those functions.
- **Same generic error for different failure reasons, when it matters for
  security.** `heartbeat` returns the same `401 invalid credentials`
  whether the `agent_id` doesn't exist, the `api_key` is wrong, or the
  agent is revoked — this prevents an attacker from enumerating valid
  agent IDs or probing for their status. `POST /auth/login` follows the
  same rule for username vs. password.
- **A valid JWT proves "who you were when the token was issued," not
  "confirm this sensitive change right now."** That's why
  `PUT /auth/change-password` still requires the current password even
  though the caller already passed the JWT check.
- **Postgres columns here are `timestamp without time zone`.** Always
  strip `tzinfo` (`.replace(tzinfo=None)`) before storing a
  `datetime.now(timezone.utc)` value, but return the tz-aware version in
  API responses. This bit the project once already (see the
  `admin/generate-token` timezone bug in early development) — don't
  reintroduce it in new endpoints that write timestamps.
- **Never build raw SQL strings with f-strings/concatenation.** Always use
  SQLAlchemy's `select(...)`/`.where(...)`. This is what protects against
  SQL injection by default.
- **In the dashboard, always route new API calls through `api.js`'s
  `request()` helper**, not a raw `fetch()` call elsewhere. This is what
  gives you automatic `Authorization` header attachment and centralized
  401-handling (`AuthError` → `forceLogout()`) for free.

---

## 6. When Your Schema Starts Changing Often: Alembic

Right now, Postgres and `models.py` are kept in sync by hand. That's fine
at this stage, but once you're changing the schema frequently — especially
with teammates, or once you deploy to a second environment — introduce
**Alembic**:

- Tracks every schema change as a versioned Python script
- Lets you upgrade/downgrade the database automatically
  (`alembic upgrade head`)
- Removes the need to manually re-type `ALTER TABLE` statements
- Keeps a history of every schema change

Signal to introduce it: manual syncing starts feeling error-prone, or
you're setting up a second (e.g. deployed) environment for the first time.

---

## 7. Bringing the `events` Table Online (Next Planned Step)

`db/models.py` already has the `Event` class fully written but commented
out, along with the `Agent.events` relationship. When you're ready to start
Phase 1.1 (event pipeline):

1. Uncomment `class Event(Base): ...` in `db/models.py`
2. Uncomment `events = relationship("Event", back_populates="agent")` on
   `Agent`
3. Run the matching `CREATE TABLE events (...)` in Postgres (the exact SQL
   is preserved as a comment block above the model — same shape)
4. Follow the "Adding a New Table" checklist from Step 3 onward (schemas,
   router, `main.py`, `/docs` testing, dashboard wiring)

This table uses `JSONB` for `raw_data`/`extracted_features` specifically so
that new event types (email phishing now, process/network/file monitoring
in Phase 2) don't require schema migrations — only new `detection/*_scorer.py`
files that know how to interpret that event type's JSON shape.

---

## 8. Quick Reference: Where Things Live

| I want to... | Start here |
|---|---|
| Add a new table | `db/models.py`, then follow section 2 above |
| Add a new admin-only action on existing data | `routers/admin.py` (or the relevant router), follow section 4 |
| Change what a response includes/excludes | `schemas/*.py` |
| Add a new protected dashboard page | New file in `dashboard/src/pages/`, add to `App.jsx`'s `<Routes>` wrapped in `<ProtectedRoute>`, add API calls to `api.js` |
| Change token/session expiry | `core/security.py` (`ACCESS_TOKEN_EXPIRE_HOURS`) or `routers/admin.py` (`TOKEN_VALIDITY_DURATION`) |
| Add scoring/detection logic | `detection/dispatcher.py` + a new `detection/<type>_scorer.py` |
| Add an automated response action | `response/actions.py` |