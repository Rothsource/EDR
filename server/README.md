# EDR Project — Architecture & Scaling Guide

This is your reference doc for understanding what each file does, and the exact
checklist to follow every time you add a new table or feature.

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
inside `models.py`).

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

---

## 2. What Each File Actually Does

### `config.py`
**Job:** Read `.env`, expose `settings.DATABASE_URL` and `settings.JWT_SECRET_KEY`.
**Contains:** Zero logic. Just environment loading.
**You touch this:** Almost never, after initial setup — maybe to add new
settings as your app grows.

### `db/database.py`
**Job:** Create the async engine (connection pool), the session factory, the
shared `Base` class, and the `get_db()` dependency used by every router.
**Contains:** Zero business logic. Just plumbing.
**You touch this:** Almost never — maybe to tune pool settings (`pool_size`,
`echo=False` for production) later.

### `db/models.py`
**Job:** Mirror your Postgres tables as Python classes (SQLAlchemy ORM).
**Contains:** Column definitions, types, constraints, foreign keys,
relationships. **No logic** — no validation, no business rules, no request
handling.
**You touch this:** Every time you add or change a table.

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
exposed, per-endpoint.

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
file itself unless you add new kinds of guards (e.g. role-based checks later).

### `detection/` and `response/`
**Job (once you build into them):**
- `detection/` — turns raw event data into a `score` / `verdict` (rules,
  thresholds, eventually ML).
- `response/` — takes a verdict and *does something* (isolate a host, kill a
  process, alert an admin).

These are kept separate from `routers/` so your "receive a request" logic
never gets tangled up with your "decide if this is malicious" logic. A router
should call into `detection/` or `response/`, not contain that logic itself.

### `main.py`
**Job:** Create the `FastAPI()` app, register (`include_router`) every
router, nothing else.
**Contains:** No business logic. Just wiring.

---

## 3. The Checklist: Adding a New Table

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
related model too (e.g. `alerts = relationship("Alert", back_populates="agent")`
on `Agent`). **Gotcha you already hit once:** if you comment out or remove a
model class, also comment out/remove any `relationship("ThatClass", ...)`
pointing at it elsewhere — SQLAlchemy fails at mapper-configuration time with
a `KeyError`/`InvalidRequestError` if a relationship references a class name
that isn't registered.

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
header — confirm you get `401 not authenticated` in the first case.

---

## 4. The Checklist: Modifying an Existing Table

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

## 5. Rules of Thumb As You Scale

- **`models.py` = structure only.** If you're tempted to write an `if`
  statement in there, it belongs in a router or a service module instead.
- **`schemas/` = contract with the outside world.** Never expose secrets
  (`api_key`, `password_hash`) in a general-purpose response schema — only
  in a dedicated one-time response schema (like `AgentRegisterResponse`),
  or never at all (there is no `UserResponse` that includes `password_hash`,
  and there shouldn't be).
- **Routers should stay thin.** If a router function starts getting long
  (validating, scoring, deciding a response action, sending alerts...),
  pull the scoring/response logic out into `detection/` or `response/` and
  have the router just call those functions.
- **Same error for different failure reasons, when it matters for security.**
  E.g. `heartbeat` returns the same `401 invalid credentials` whether the
  `agent_id` doesn't exist or the `api_key` is wrong — this prevents an
  attacker from enumerating valid agent IDs. `POST /auth/login` follows the
  same rule: wrong username and wrong password both return the same
  `401 invalid credentials`.
- **Never build raw SQL strings with f-strings/concatenation.** Always use
  SQLAlchemy's `select(...)`/`.where(...)` or parameterized queries (`$1`,
  `$2` with asyncpg). This is what protects you from SQL injection by
  default — don't work around it.
- **One-time secrets get their own response schema.** Anything like
  `api_key` or a freshly generated token should have a dedicated `*Response`
  schema used only at creation time, separate from the general "list/get"
  response schema.
- **A valid JWT proves "who you were when the token was issued," not
  "confirm this sensitive change right now."** That's why
  `PUT /auth/change-password` still requires the current password even
  though the caller already passed the JWT check — a stolen/leaked token
  shouldn't be enough on its own to lock the real owner out of their account.
- **Postgres columns here are `timestamp without time zone`.** Always strip
  `tzinfo` (`.replace(tzinfo=None)`) before storing a
  `datetime.now(timezone.utc)` value, but return the tz-aware version in API
  responses. JWT expiry (`exp` claim) doesn't hit this issue — `python-jose`
  handles the conversion internally.

---

## 6. When Your Schema Starts Changing Often: Alembic

Right now you're manually keeping Postgres and `models.py` in sync by hand.
That's fine at this stage, but once you have several tables and are changing
things frequently (especially with teammates, or once you deploy), consider
introducing **Alembic** — a migration tool for SQLAlchemy that:

- Tracks every schema change as a versioned Python script
- Lets you upgrade/downgrade the database automatically (`alembic upgrade head`)
- Removes the need to manually remember and re-type `ALTER TABLE` statements
- Keeps a history of every schema change, which is invaluable once this
  isn't just a single local database anymore

You don't need it yet — but when manual syncing starts feeling error-prone
or you're setting up a second environment (e.g. deploying to a real server),
that's the signal to introduce it.

---

## 7. Current Project Snapshot (as of this guide)

**Tables in Postgres:** `enrollment_tokens`, `agents`, `users`.
`events` is defined in `models.py` but currently commented out (along with
the matching `Agent.events` relationship) — not yet created in Postgres.
Uncomment both sides and create the table when you're ready to build event
ingestion.

**New folder: `core/`**
- `core/security.py` — password hashing (bcrypt via passlib) and JWT
  create/verify helpers (python-jose, `HS256`, 24h expiry, signed with
  `settings.JWT_SECRET_KEY`). Pure functions — no DB access, no request
  handling.
- `core/deps.py` — `get_current_user_id`, a FastAPI dependency that reads
  the `Authorization: Bearer <token>` header, verifies it, and either
  returns the `user_id` or raises `401 not authenticated`. Add this as a
  `Depends()` on any route that should require login.

**New schema file: `schemas/auth.py`**
- `LoginRequest` — `username`, `password`
- `TokenResponse` — `access_token`, `token_type` (defaults to `"bearer"`)
- `ChangePasswordRequest` — `current_password`, `new_password`

**New router: `routers/auth.py`, mounted at `/auth`**
- `POST /auth/login` — looks up the user, verifies the password hash,
  returns a JWT on success. Same generic `401 invalid credentials` for
  "user doesn't exist" and "wrong password."
- `PUT /auth/change-password` — protected route (requires a valid JWT).
  Also requires the caller to supply their *current* password before
  setting a new one, even though they're already authenticated — this
  prevents a stolen token alone from being enough to lock out the real
  admin.

**Bootstrapping the first admin: `create_admin.py`**
A one-time CLI script (run manually, not exposed via any endpoint) that
prompts for a username/password and inserts the first row into `users`,
with the password hashed via `core/security.hash_password()`. There is no
public signup route — this is intentional. This script is a **development/
testing tool**, not a production onboarding flow; see the note at the end of
this section for what a real deployment would need instead.

**Endpoints built:**
- `POST /auth/login` — admin login, returns JWT
- `PUT /auth/change-password` — admin changes their own password (protected)
- `POST /admin/generate-token` — admin creates a one-time enrollment token
  (**protected** — requires `Authorization: Bearer <token>`)
- `POST /agent/register` — new agent registers using a valid enrollment token
- `POST /agent/heartbeat` — registered agent proves it's alive
- `GET /agents` — list all agents, excludes `api_key`
  (**not yet protected** — see "Next natural additions" below)

**How to test the full auth flow via `/docs`:**
1. `POST /admin/generate-token` with no `Authorization` header →
   expect `401 {"detail": "not authenticated"}`
2. `POST /auth/login` with your admin username/password →
   copy the `access_token` from the response
3. Call `POST /admin/generate-token` again, this time manually adding header
   `Authorization: Bearer <token>` → expect a `200` with the enrollment token
   (note: since this dependency is a plain `Header(...)` check rather than
   FastAPI's `OAuth2PasswordBearer` scheme, the Swagger "Authorize" lock icon
   in `/docs` won't auto-attach the header — set it manually per-request, or
   test with curl/Postman)

**Next natural additions**, in likely order:
1. Protect `GET /agents` the same way `/admin/generate-token` is protected
   — currently it's the one remaining open route that probably shouldn't be
2. `events` table + `routers/events.py` — agent event ingestion
3. `detection/` scoring logic — turn raw events into `score` + `verdict`
4. `response/` actions — act on verdicts (isolate host, alert, etc.)
5. Alembic, once schema changes become frequent
6. Admin dashboard frontend (`dashboard/`, currently just planned) — login
   page, protected routes on the frontend side, token storage
   (localStorage is fine for a local-network student prototype; note it as
   a pre-production hardening item alongside HTTPS and multi-tenancy)
7. Production-grade admin bootstrapping — `create_admin.py` is a dev/testing
   tool, not something you'd hand to a real SME customer. Before any real
   deployment, replace or supplement it with one of: (a) auto-generate a
   random admin password on first startup and print it once to the console/
   logs, (b) force a password change on first login via a
   `must_change_password` flag, or (c) a first-run setup wizard in the
   dashboard that prompts the user to choose their own admin credentials.