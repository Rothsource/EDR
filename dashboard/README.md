# EDR Console — Admin Dashboard

React (Vite) frontend for the EDR admin panel. Talks to your existing
FastAPI backend (`server/`) — no backend logic lives here, only UI.

Design carried over from a Stitch mockup, adapted to only show fields your
backend actually returns. Fields the mockup invented (agent version, IP
address, CSV export, tenant name, 2FA, audit log, latency/ping) were
intentionally left out — add them here once (and only once) the backend
actually supports them.

## Setup

```powershell
cd dashboard
npm install
cp .env.example .env
```

Edit `.env` if your server isn't on `localhost:8000` (e.g. running on
another machine on your network):
```
VITE_API_URL=http://192.168.1.10:8000
```

Run it:
```powershell
npm run dev
```
Opens at `http://localhost:5173`.

## IMPORTANT — backend change required first: enable CORS

Your FastAPI server currently has no CORS configuration. Without it, the
browser will block every request this dashboard makes to your API (you'll
see "Failed to fetch" / CORS errors in the browser console, even though the
backend itself is running fine).

Add this to `server/app/main.py`, **before** your routers are included:

```python
from fastapi.middleware.cors import CORSMiddleware

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:5173"],  # the dashboard's dev URL
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)
```

If you later deploy the dashboard somewhere else (a real domain, or another
machine's IP), add that origin to `allow_origins` too.

## Pages

| Route | Page | Backend endpoint(s) used |
|---|---|---|
| `/login` | Login | `POST /auth/login` |
| `/` | Dashboard | `GET /agents` |
| `/agents` | Agents list + generate token | `GET /agents`, `POST /admin/generate-token` |
| `/settings` | Change password | `PUT /auth/change-password` |

## How auth works here

- On login, the JWT is stored in `localStorage` (see `src/api.js`). This is
  a pragmatic choice for a local-network student prototype — see the main
  project's architecture doc for the httpOnly-cookie alternative and why
  it's a "pre-production hardening" item, same category as HTTPS.
- Every API call automatically attaches `Authorization: Bearer <token>`
  (see `request()` in `src/api.js`).
- If any call comes back `401`, the token is cleared and the user is
  redirected to `/login` — this covers both "never logged in" and "token
  expired" cases with the same code path.
- Logout is purely client-side (clears the stored token) — matches the
  backend's stateless JWT design; there's nothing to tell the server.

## What's deliberately NOT here yet

- Agent detail page (click into a single agent) — your backend doesn't
  expose a `GET /agents/{id}` yet
- Revoke/deactivate an agent — no backend endpoint exists yet either
- Any events/alerts UI — `events` table isn't built yet (Phase 1.1+ in the
  main roadmap)

Build the backend endpoint first, then come back and add the matching page
— same order the rest of this project has followed throughout.
