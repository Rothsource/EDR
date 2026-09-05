\# EDR Project (Cambodia SME-focused, AI-enhanced)



An open-source-based, AI-enhanced Endpoint Detection \& Response (EDR) prototype, built as a 3rd-year student project. The long-term vision is a lightweight, affordable security stack for small-to-medium businesses in Cambodia that currently have no endpoint or email threat monitoring.



\## Project Vision



Commercial EDR/SIEM tools (CrowdStrike, SentinelOne, VNCS, Check Point) are priced for enterprises with dedicated security teams — out of reach for most Cambodian SMEs. This project prototypes an alternative: an agent-based detection system built on open-source foundations (and eventually Wazuh for log collection), with a custom AI layer for phishing detection and alert triage, packaged to be realistically deployable by a non-specialist IT person.



\*\*Reference model:\*\* AhnLab (South Korea) — a domestic EDR vendor that started small and grew into a national leader by serving a market international vendors didn't prioritize. The long-term ambition here is similar, starting from SMEs rather than enterprises.



\## Tech Stack



| Component | Technology | Why |

|---|---|---|

| Agent (runs on endpoints) | \*\*Go\*\* | Compiles to a single dependency-free binary, cross-platform (Windows/Linux), low resource usage — matches how real EDR agents (and Wazuh's core, written in C) are built |

| Server (API + detection) | \*\*Python + FastAPI\*\* | Fast to build, native ML/AI library support (scikit-learn, transformers), auto-generated API docs |

| Database | \*\*PostgreSQL\*\* | Reliable, relational, JSONB support lets us store flexible event data without rigid per-type schemas |

| Log/endpoint monitoring engine (Phase 2+) | \*\*Wazuh\*\* (planned) | Open-source SIEM/XDR — used unmodified (GPLv2-compliant) as the log collection backbone; our AI layer sits on top as separate, original code |



\## Project Structure



```

edr-project/

├── agent/                          # Go codebase — runs on monitored endpoints

│   ├── cmd/agent/main.go             # Entrypoint: loads config, runs enabled modules in a loop

│   ├── internal/

│   │   ├── config/                     # Loads --server/--token flags, reads/writes local config file

│   │   ├── core/                        # Shared building blocks used by every module

│   │   │   ├── event.go                   # Generic Event struct — the shared data shape for ALL event types

│   │   │   ├── sender.go                   # HTTP client: register(), heartbeat(), sendEvent()

│   │   │   └── module.go                    # Module interface (Name() + Run()) every module implements

│   │   ├── modules/                      # One folder per capability — add new ones without touching existing

│   │   │   ├── email/                      # Phase 1: IMAP fetch + phishing feature extraction

│   │   │   ├── process/                    # Phase 2: process monitoring

│   │   │   ├── network/                    # Phase 2: network connection metadata

│   │   │   ├── file/                       # Phase 2: file integrity monitoring

│   │   │   └── response/                   # Phase 3: kill process / quarantine file / isolate endpoint

│   │   └── platform/                     # OS-specific service registration (Windows Service / systemd)

│   ├── go.mod / go.sum

│

├── server/                          # Python/FastAPI codebase — receives data, runs detection, serves API

│   ├── app/

│   │   ├── main.py                       # FastAPI entrypoint, mounts all routers

│   │   ├── config.py                      # Environment variables, DB connection settings

│   │   ├── db/

│   │   │   ├── database.py                  # DB connection/session setup

│   │   │   └── models.py                     # Agent, EnrollmentToken, Event tables

│   │   ├── schemas/                        # Pydantic request/response models

│   │   ├── routers/

│   │   │   ├── admin.py                      # POST /admin/generate-token

│   │   │   ├── agent.py                       # POST /agent/register, /agent/heartbeat, GET /agents

│   │   │   └── events.py                       # POST /agent/events, GET /events (generic, type-agnostic)

│   │   ├── detection/

│   │   │   ├── dispatcher.py                    # Routes each event to the correct scorer by event\_type

│   │   │   ├── email\_scorer.py                   # Phase 1: phishing scoring logic

│   │   │   ├── process\_scorer.py                  # Phase 2

│   │   │   └── network\_scorer.py                   # Phase 2

│   │   └── response/actions.py               # Phase 3: response action dispatch

│   ├── requirements.txt / .env

│

├── dashboard/                       # (Planned) Web UI for viewing agents/alerts

├── docs/

│   ├── api-contract.md                # Source of truth: Event schema + all endpoint request/response shapes

│   └── architecture.md                 # Notes/diagrams on how agent + server + detection fit together

├── scripts/                          # Install/uninstall automation (built in Phase 1.5, after core works)

│   ├── install.sh / install.ps1

│   └── uninstall.sh / uninstall.ps1

├── .gitignore

└── README.md

```



\### Why this structure scales



\- \*\*One generic `Event` shape\*\* (`agent\_id`, `event\_type`, `timestamp`, `raw\_data`, `extracted\_features`) is the contract between agent and server. Adding new capabilities (Phase 2 process/network/file monitoring, Phase 3 response) means adding a new module folder and a new scorer file — never editing existing ones.

\- \*\*`events` table uses JSONB\*\* for `raw\_data`/`extracted\_features`, so new event types don't require schema migrations.

\- \*\*`tenant\_id`/`organization\_id`\*\* should be included on `agents` and `events` from the start, even with a single test deployment — this is the field that's painful to add retroactively once multiple SME customers exist.



\## Roadmap



\- \*\*Phase 1.0\*\* — Agent-server connectivity: enrollment tokens, registration, heartbeat, no detection logic yet

\- \*\*Phase 1.1\*\* — Generic event pipeline + real email phishing detection (IMAP fetch, feature extraction, scoring)

\- \*\*Phase 1.5\*\* — Deployment polish: binary hosting, one-line install scripts, background service registration

\- \*\*Phase 2\*\* — Log collection (process/file/network), likely via osquery or Wazuh integration

\- \*\*Phase 3\*\* — Response actions (kill process, quarantine file, isolate endpoint) — human-approved first, autonomous later



\---



\## Setup Guide



\### Prerequisites (install once)



\- \*\*Go\*\* (1.21+) — \[https://go.dev/dl/](https://go.dev/dl/)

\- \*\*Python\*\* (3.10+) — \[https://www.python.org/downloads/](https://www.python.org/downloads/)

\- \*\*PostgreSQL\*\* (15+) — \[https://www.postgresql.org/download/](https://www.postgresql.org/download/)

\- \*\*Git\*\* (for version control)



Verify installs:

```powershell

go version

python --version

psql --version

```



\---



\### Server Setup (Python + FastAPI + PostgreSQL)



Run these from the `server/` folder.



\*\*1. Create the database\*\*



Open `psql` (or pgAdmin) and run:

```sql

CREATE DATABASE edr\_project;

```



\*\*2. Create and activate a virtual environment\*\*

```powershell

cd server

python -m venv venv

.\\venv\\Scripts\\Activate.ps1

```

> If PowerShell blocks activation with an execution policy error, run this once (as your user, not admin):

> ```powershell

> Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser

> ```



\*\*3. Install dependencies\*\*



`requirements.txt` should contain:

```

fastapi

uvicorn\[standard]

asyncpg

python-dotenv

sqlalchemy

```



Install:

```powershell

pip install -r requirements.txt

```



\*\*4. Configure environment variables\*\*



Create `.env` in `server/` with:

```

DATABASE\_URL=postgresql+asyncpg://<db\_user>:<db\_password>@localhost:5432/edr\_project

```

Replace `<db\_user>`/`<db\_password>` with your actual PostgreSQL credentials.



\*\*5. Run the server\*\*

```powershell

uvicorn app.main:app --reload --host 0.0.0.0 --port 8000

```

\- `--host 0.0.0.0` makes it reachable from other machines on your local network (needed so the agent on PC1 can reach it)

\- Visit `http://localhost:8000/docs` to see the interactive API docs once endpoints are built



\*\*6. Find your server's local IP\*\* (needed for the agent to connect)

```powershell

ipconfig

```

Look for the `IPv4 Address` under your active network adapter (e.g. `192.168.1.10`).



\---



\### Agent Setup (Go)



Run these from the `agent/` folder.



\*\*1. Confirm the Go module is initialized\*\*

```powershell

cd agent

type go.mod

```

Should show `module edr-agent` and a Go version line.



\*\*2. Add dependencies as needed\*\* (as you build each sub-phase)

```powershell

go get gopkg.in/yaml.v3

go get github.com/emersion/go-imap/v2

go get github.com/emersion/go-message

```

Each `go get` updates `go.mod` and generates/updates `go.sum` automatically.



\*\*3. Build the agent binary\*\*

```powershell

go build -o edr-agent.exe ./cmd/agent

```



\*\*4. Run the agent, pointing it at the server\*\*

```powershell

.\\edr-agent.exe --server=http://192.168.1.10:8000 --token=<token-from-server>

```

(Replace the IP with your actual server machine's IP found via `ipconfig`, and the token with one generated via the server's `/admin/generate-token` endpoint.)



\*\*5. Cross-compile for other platforms\*\* (when ready to test on Linux too)

```powershell

$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o edr-agent-linux ./cmd/agent

$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o edr-agent.exe ./cmd/agent

```



\---



\### Running Both Together (local network test)



1\. Start PostgreSQL (usually runs as a background service automatically after install)

2\. Start the server on PC2: `uvicorn app.main:app --reload --host 0.0.0.0 --port 8000`

3\. Generate a token from the server (via `/docs` or `curl`)

4\. Copy the compiled agent binary to PC1

5\. Run the agent on PC1 with `--server=http://<PC2-IP>:8000 --token=<token>`

6\. Confirm on PC2 (via `GET /agents` or `/docs`) that the agent registered and is heartbeating



\---



\## Notes



\- This is a \*\*student prototype (Project Alpha)\*\* — not audited, not intended for production security use yet. See `docs/architecture.md` for design rationale and `docs/api-contract.md` for the exact event/API schema every module must follow.

\- Multi-tenancy (`tenant\_id`) and encryption in transit (HTTPS) are required before any real SME deployment — both are noted as pre-deployment requirements, not needed for local development/testing.

