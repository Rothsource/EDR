# KhemStrix EDR: Master Project Status & Scalable Architecture Report

## 1. Executive Summary & Cambodia Context

KhemStrix EDR is an open-source-based, AI-enhanced Endpoint Detection & Response (EDR) platform designed for small-to-medium enterprises (SMEs) in Cambodia. Commercial EDR products (CrowdStrike, SentinelOne, Microsoft Defender for Endpoint) impose enterprise-tier USD licensing, heavy endpoint footprints, and require dedicated SOC analysts that local organizations cannot afford.

The project is modeled after South Korea's AhnLab — establishing a domestic, cost-effective detection foundation that aligns directly with Cambodian regulatory demands:

- **MPTC Draft Law on Cybersecurity**: Mandatory audit logging, threat monitoring, and incident reporting for critical information infrastructure and digital service operators.
- **MPTC Draft Law on Personal Data Protection**: Clear boundaries around data minimization, requiring that employee personal communications are never intercepted or exported without statutory cause.
- **National Bank of Cambodia (NBC) TCRMG Guidelines**: Technology and Cyber Risk Management requirements mandating endpoint auditability and rapid incident containment.

```
┌────────────────────────────────────────────────────────┐
│             DUAL-MODEL ARCHITECTURAL BRIDGE            │
│                                                        │
│  MODEL 1: PRIVATE SME NODE (NOW)                       │
│  • Fully local LAN deployment inside SME office        │
│  • Single default organization in database             │
│  • In-process Python queue (no Redis/broker daemons)   │
│  • Real-time email inference (raw body dropped)        │
│                                                        │
│                           │ Upgrades seamlessly        │
│                           ▼ with zero code rewrite     │
│                                                        │
│  MODEL 2: CENTRALIZED MPTC/CamCERT HUB (FUTURE)        │
│  • National sovereign threat grid across many SMEs     │
│  • Native multi-tenant isolation (organizations table) │
│  • Swap in-process queue for distributed message bus   │
│  • Aggregated threat telemetry for national defense    │
└────────────────────────────────────────────────────────┘
```

## 2. Done and Tested (Phase 1.0 Milestone)

Every item below is implemented, compiled, and verified across separate physical machines and virtual environments.

```
[ Kali Linux / Windows Endpoint ]                 [ Windows Host Server ]
┌───────────────────────────────┐                 ┌───────────────────────┐
│ khemstrix-agent (Go)          │                 │ FastAPI + PostgreSQL  │
│ • Static single binary        │   HTTP JSON     │ • Token Registration  │
│ • Systemd & Windows Service   ├────────────────►│ • Dynamic IP/MAC sync │
│ • Auto-persists config.json   │                 │ • UTC Serializer      │
└───────────────────────────────┘                 └───────────────────────┘
                                                              │
                                                              ▼
                                                  ┌───────────────────────┐
                                                  │ React + Vite Shell    │
                                                  │ • One-line installer  │
                                                  │ • Revoke/Delete/Live  │
                                                  └───────────────────────┘
```

### Backend — Core Connectivity & Life Cycle

- **Database Pipeline**: PostgreSQL tables (`enrollment_tokens`, `agents`, `users`) verified active and responsive via `psql`.
- **ORM Mapping**: Synchronized schema chain: `config.py` → `db/database.py` → `db/models.py`.
- **Storage Timezone Normalization**: Eliminated naive vs. aware datetime conversion mismatches across PostgreSQL `timestamp without time zone` columns.
- **UTC Serialization Fix**: Added explicit `@field_serializer` to `schemas/agent.py` forcing naive UTC datetimes to render ISO-8601 strings with an explicit `+00:00` offset. Resolved browser timeline bugs where UTC+7 clients (Phnom Penh) displayed events as occurring "7 hours ago".
- **Registration & Enrollment**: `POST /admin/generate-token` issues single-use, 1-hour expiration tokens. `POST /agent/register` converts tokens to permanent `api_keys` and records hardware metadata.
- **Dynamic IP/MAC Heartbeat Refresh**: `POST /agent/heartbeat` validates `agent_id` + `api_key` + active status. It dynamically refreshes stored `ip_address` and `mac_address` whenever an endpoint renews its DHCP lease, activates a VPN, or switches subnets.
- **Administrative Controls**: `PATCH /admin/agents/{agent_id}/revoke` cuts off heartbeat checks with immediate 401 Unauthorized responses. `PATCH /admin/agents/{agent_id}/unrevoke` seamlessly reinstates agents without requiring re-registration. `DELETE /admin/agents/{agent_id}` removes records from inventory.
- **Binary Distribution**: `routers/downloads.py` serves compiled agents (`khemstrixAgent.exe`, `khemstrixAgent`) with absolute filesystem pathing derived from `__file__`, eliminating uvicorn working-directory errors.

### Go Endpoint Agent (khemstrix-agent)

- **Clean Single Binary**: Dependency-free compilation on Windows (`.exe`) and Linux (cross-compiled via `GOOS=linux` from Windows).
- **Unified Network Extraction**: `internal/core/netinfo.go` traverses interfaces and extracts the IPv4 address and MAC address simultaneously from the active NIC, skipping loopback and inactive links.
- **Local Persistence**: `internal/config/config.go` persists runtime settings to `C:\ProgramData\khemstrix-agent\config.json` (Windows) or `/etc/khemstrix-agent/config.json` (Linux).
- **Persistent Execution**: Verified running as a native systemd unit on Linux and as a background service on Windows without holding open interactive terminal windows.
- **State Machine**: Automatically chooses startup mode: runs the heartbeat loop if `config.json` is present; requires `--token`, enrolls with the server, writes configuration, and transitions into the heartbeat loop if absent.

### Management Web Dashboard (React + Vite)

- **Live Fleet Table**: Real-time visibility into hostnames, OS, dynamically refreshed IP/MAC addresses, and connection status badges (Online, Offline, Revoked).
- **Fleet Actions**: Wired interactive confirmation modals for agent revocation, reactivation, and permanent deletion.
- **Chained One-Line Installer**: Dynamic generation of copy-paste installation commands for Windows PowerShell and Linux Bash using the backend server's LAN-accessible IP address.

## 3. Scalable Architecture Foundations (Phase 1.1)

To transition from machine inventory (heartbeats) to security threat monitoring (events) without future code rewrites, four architectural seams must be established:

```
                                  INGESTION & BUFFERING PIPELINE

 [ Go Endpoint Agent ]                    [ FastAPI Backend ]                 [ PostgreSQL ]
┌─────────────────────┐                 ┌────────────────────┐             ┌────────────────────┐
│ In-Memory Buffer    │  Batch Flush    │  asyncio.Queue      │  Bulk       │  organizations     │
│ • Max 50 items      ├────────────────►│  • Buffer bursts    ├────────────►│  agents            │
│ • 20s Jitter Ticker │  HTTP 202 (<5ms)│  • Background task  │  Insert     │  events (Partition │
└─────────────────────┘                 └────────────────────┘             │   by Month)        │
                                                                            └────────────────────┘
```

### 1. Database Multi-Tenancy Anchor

- Create an `organizations` table as the root tenant entity.
- Seed a static default UUID (`00000000-0000-0000-0000-000000000001`, "Default SME").
- Add `tenant_id` foreign keys to `agents`, `enrollment_tokens`, and `events`.
- **Why**: Model 1 runs cleanly as a single-tenant instance. Model 2 simply registers additional organization rows, and queries filter via `WHERE tenant_id = :current_tenant` without requiring schema migrations.

### 2. Time-Based Partitioning on Telemetry

- High-volume security logs rapidly degrade standard B-Tree indexing.
- Partition the `events` table by `RANGE (timestamp)` in monthly increments (`events_2026_09`).
- Telemetry past the Cambodian regulatory 90-day retention window can be dropped instantly via `DROP TABLE events_YYYY_MM`, avoiding database vacuum locks.

### 3. Asynchronous In-Process Queue (asyncio.Queue)

To prevent incoming telemetry spikes from starving database connections, decouple API receipt from database writes.

- Avoid external broker installations (Redis, RabbitMQ, Celery) to keep Model 1 lightweight on low-spec SME hardware.
- Use Python's native `asyncio.Queue(maxsize=5000)`:
  - `POST /agent/events` validates caller authentication and schema.
  - Enqueues the batch into process memory via `.put_nowait()`.
  - Returns `202 Accepted` in <5ms.
- An asynchronous worker task tied to the FastAPI lifespan consumes batches, runs heuristic detection, and executes bulk database commits (`session.add_all()`).

### 4. Edge Batching & Jitter on the Endpoint

- The Go agent buffers security observations in a thread-safe slice rather than making an HTTP call per event.
- Flushes occur when the buffer reaches 50 events or when a 20-second ticker fires.
- Includes random time jitter (±3 seconds) to prevent simultaneous connections from crowding local office network routers.
- **Offline Durability**: Retains buffered logs in memory during temporary network drops and retries on subsequent flush cycles.

### 5. Real-Time Fast-Path for Critical Events (Planned)

The 20-second batch window in Item 4 is correct for routine telemetry but too slow for a detection that needs an immediate response (e.g. ransomware-pattern file activity, a brute-force authentication burst). Rather than shortening the batch window for everyone — which reintroduces the request-overhead problem Item 4 exists to solve — a second, low-volume path is added alongside it:

- A persistent WebSocket connection (`/agent/stream`) is opened by the agent at startup and held open, reusing the same FastAPI app and agent-credential authentication as the existing REST endpoints.
- Routing is mechanical, not a threat judgment made on the agent: events belonging to a small set of high-signal classes (Process Activity, failed Authentication) or crossing a `severity_id` threshold skip the 50-item/20s buffer and are sent immediately over the socket. Everything else stays on the existing batch path.
- When a fast-path event fires, the agent drains whatever is currently sitting in the routine buffer and sends it in the same frame, giving the server forensic context (the preceding seconds of file/network activity) alongside the alert at no extra cost.
- The agent still performs zero classification — it is only ever asking "which lane does this event type belong to," never "is this malicious." That judgment stays entirely server-side, consistent with Section 5's detection pipeline.
- Both paths write into the same `events` table using the same idempotent insert (`event_id` conflict key) — this is additive to the existing pipeline, not a replacement, and the REST batch endpoint stays in place as a fallback for agents on networks that cannot sustain a persistent connection.

## 4. The Standard Event Contract

Every future telemetry module (email phishing today; process execution, network sockets, and file integrity monitoring in Phase 2) must adhere to this standard event envelope:

```json
{
  "event_type": "email_phishing",
  "timestamp": "2026-09-09T07:13:05Z",
  "raw_data": {
    "sender": "security@aba-verify-kh.com",
    "subject": "Urgent: Your account is locked",
    "links": ["http://login.aba-verify-kh.com/auth"],
    "attachment_hashes": ["e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"]
  },
  "extracted_features": {
    "has_brand_homoglyph": true,
    "urgency_score": 0.92,
    "attachment_is_executable": false
  }
}
```

- **Schema Independence via JSONB**: Metadata is indexed in formal relational columns, while dynamic fields reside in `raw_data` and `extracted_features`.
- New endpoint monitoring capabilities can be added without running table migrations on existing deployments.

## 5. Threat Detection Strategy: AI NLP + VirusTotal

### The Dual-Risk Reality of Email Threats

- **Malware Attachments**: Inbound payloads, malicious macros, and weaponized archives.
- **Social Engineering / Pure Phishing**: Attacks containing no attachments at all (e.g., credential theft links, deceptive payment redirections, bank verification lures).

### Why VirusTotal Alone Is Insufficient

- **No File = No Value**: Over 70% of modern phishing attacks carry no attachment. VirusTotal provides zero visibility on pure social engineering text and novel phishing URLs.
- **Zero-Day Blind Spots**: New malware or modified payloads produce unseen SHA-256 hashes, returning 0/70 Clean from VirusTotal.
- **Severe Rate Limits**: VirusTotal's free public API allows only 4 requests/minute and 500 requests/day. A small office of 15 employees exhausts this quota within hours, resulting in HTTP 429 API lockouts.
- **Privacy Risk of Direct File Uploads**: Uploading actual business files to VirusTotal exposes proprietary documents to security researchers globally.

### The Hybrid Multi-Layer Defense Engine

```
                          INBOUND EMAIL TELEMETRY
                                     │
                                     ▼
                    ┌─────────────────────────────────┐
                    │  LAYER 1: Hard Heuristic Rules  │
                    │  • Double extensions (.pdf.exe) │ ──► High Severity (Instant)
                    │  • Known malicious TLDs         │
                    └────────────────┬────────────────┘
                                     │ Passed
                                     ▼
                    ┌─────────────────────────────────┐
                    │  LAYER 2: VirusTotal Hash Check │
                    │  • Query SHA-256 in local cache │ ──► Known Malware
                    │  • If VT detections >= 3        │
                    └────────────────┬────────────────┘
                                     │ Clean / Unknown / No Attachment
                                     ▼
                    ┌─────────────────────────────────┐
                    │  LAYER 3: Local NLP Model       │
                    │  • Social engineering intent    │ ──► Score: 0.0 to 1.0
                    │  • Urgency & psychological lure │     (Phishing Verdict)
                    │  • Homoglyph/brand mismatch     │
                    └─────────────────────────────────┘
```

- **Layer 1 (Fast Heuristics)**: Blocks dangerous file extensions (`.exe`, `.scr`, `.vbs`, `.iso`) and spoofed executable tricks locally in zero time.
- **Layer 2 (Cached VirusTotal Lookups)**: Computes the SHA-256 hash of attachments in memory on the Go agent. Queries a local database hash cache first; queries VirusTotal only on cache misses, staying within rate limits while keeping raw files private.
- **Layer 3 (Offline-Trained NLP Model)**: The primary brain. Evaluates message text and structure to detect credential phishing, urgency manipulation, and domain typosquatting.

### Data Privacy & Regulatory Compliance (Zero Body Storage)

To comply with Cambodia's draft data protection standards, the system operates on a zero-persistence principle for email bodies:

1. The Go agent extracts plain text, links, and headers in memory.
2. The backend runs inference to determine threat probability and identify specific attack indicators (e.g., "brand_impersonation", "credential_harvesting").
3. The raw email body is immediately purged from memory. It is never written to PostgreSQL or persisted to disk. Only the threat score, verdict, and extracted security indicators are stored for compliance audits.

## 6. Team Division of Labor: Systems Lead vs. AI Teammate

```
┌──────────────────────────────────────────────┐  ┌──────────────────────────────────────────────┐
│         SYSTEMS LEAD (PROJECT LEAD)          │  │       AI & BACKEND (YEAR 3 TEAMMATE)         │
├──────────────────────────────────────────────┤  ├──────────────────────────────────────────────┤
│ 1. Fix Linux systemd flag precedence         │  │ 1. Curate public email phishing datasets     │
│ 2. Audit Windows background service          │  │ 2. Generate synthetic Cambodian lures (ABA)  │
│ 3. Build Go agent mutex Ring Buffer          │  │ 3. Train ML/NLP models (TF-IDF + Forest)     │
│ 4. Build agent IMAP attachment hash extractor│  │ 4. Build `server/app/detection/email_scorer` │
│ 5. Implement SendEvents() batch dispatcher   │  │ 5. Implement VirusTotal hash caching layer   │
│ 6. Rebuild static cross-compiled binaries    │  │ 6. Run SQL multi-tenant database migration   │
└──────────────────────┬───────────────────────┘  └──────────────────────┬───────────────────────┘
                       │                                                 │
                       └───────────────────►◄────────────────────────────┘
                                     INTEGRATION TEST
                           Simulated Phishing Event Ingestion
```

### Systems Lead Responsibilities (Core Infrastructure)

- **Service Precedence Resolution**: Remove hardcoded `--server=` arguments from `/etc/systemd/system/khemstrix-agent.service`. Ensure `cmd/agent/main.go` gives precedence to `config.json` over CLI flag defaults.
- **Windows Service Verification**: Audit Windows service execution parameters to ensure runtime settings are not overridden.
- **Agent Ring Buffer** (`agent/internal/core/event.go`): Implement thread-safe buffer slice with dual-condition flushing (50 events or 20s ticker with jitter).
- **IMAP Telemetry Module** (`agent/internal/modules/email/`): Connect via IMAP, extract headers, extract links, compute attachment SHA-256 hashes in memory, and purge raw bytes.
- **Agent Batch Sender** (`agent/internal/core/sender.go`): Implement `SendEvents()` to dispatch JSON batches to `POST /agent/events` with offline retry handling.
- **Binary Pipeline**: Cross-compile updated binaries and maintain files in `server/app/static/binaries/`.

### AI Teammate Responsibilities (Machine Learning & Ingestion)

- **Dataset Curation & Preprocessing**: Assemble public phishing corpora (Nazario, SpamAssassin, Kaggle, Hugging Face). Generate synthetic samples modeling local Cambodian lures (ABA Bank, Canadia, Wing, Telegram verification).
- **Model Training & Evaluation**: Train a baseline tabular classifier (TF-IDF vectorizer + Random Forest / XGBoost) on phishing language, urgency indicators, and link patterns. Evaluate precision, recall, and false-positive rates for her academic defense.
- **Export Trained Pipeline**: Serialize the model and vectorizer to `.joblib` artifacts for integration into FastAPI.
- **FastAPI Scoring Pipeline** (`server/app/detection/email_scorer.py`): Load artifacts into memory on startup; accept incoming event text and features; return threat probabilities, risk tiers (safe, suspicious, malicious), and threat indicators.
- **VirusTotal Hash Cache Client**: Build `server/app/detection/virustotal.py` to query SHA-256 hashes against a local cache table before consuming public API quotas.
- **Database Migration & Schemas**: Execute SQL migration for `organizations` and partitioned `events`; implement `schemas/event.py` with the UTC serializer; build the `asyncio.Queue` worker in `server/app/core/queue.py`.

## 7. Next Implementation Steps (Priority Order)

### Step 1: Configuration & Service Precedence Fix
- Remove `--server=` flag from Linux systemd unit file; reload daemon.
- Ensure `cmd/agent/main.go` gives priority to `config.json`.

### Step 2: Database Multi-Tenancy & Partitioning
- Run the SQL migration to create `organizations` and add `tenant_id` to existing tables.
- Create the monthly partitioned events table (`events_2026_09`) with indexes on `(tenant_id, created_at DESC)` and `(agent_id, timestamp DESC)`.
- Update SQLAlchemy models in `server/app/db/models.py`.

### Step 3: FastAPI Async Ingestion Pipeline
- Create `schemas/event.py` with UTC serializer methods.
- Implement in-process queue and lifespan worker in `core/queue.py` and `main.py`.
- Create `routers/events.py` with heartbeat-style credential validation returning `202 Accepted`.

### Step 4: Go Agent Batching Core
- Implement thread-safe `EventBuffer` in `agent/internal/core/event.go`.
- Add `SendEvents()` to `agent/internal/core/sender.go` with retry resilience.

### Step 5: Offline Model Training & Email Scorer
- Train the baseline NLP phishing model on benchmark corpora and export `model.joblib`.
- Build `server/app/detection/email_scorer.py` and hook it into the background queue worker.

### Step 6: Fast-Path Critical Event Delivery (WebSocket)
- Add `/agent/stream` WebSocket route to the FastAPI app, authenticated the same way as `/agent/events`; keep the REST batch endpoint live in parallel.
- Define the initial fast-lane rule set: Process Activity events, failed/anomalous Authentication events, and any event where `severity_id` crosses an agreed threshold.
- Implement the agent-side router (`agent/internal/core/router.go` or similar) that sends fast-lane events immediately and drains the current routine buffer alongside them, versus depositing everything else into the existing `EventBuffer`.
- Enforce hard caps on the new send path (in-memory send buffer size, disk spool size for extended outages) with a defined drop policy when full — no unbounded growth on the endpoint if the server is unreachable.
- Wire fast-lane detections into the `alerts` table (Detection Finding, class_uid 2004) and fire a notification (webhook or dashboard push) at match time, not on a polling cycle — the database write alone is not fast enough to count as alerting.

### Step 7: End-to-End System Verification
- Dispatch synthetic event batches via `curl` to `POST /agent/events`; verify `202 Accepted`.
- Query PostgreSQL directly (`SELECT * FROM events;`) to confirm unbuffered writes.
- Verify detection scoring outputs and confirm raw email bodies are not written to disk.
- Simulate a fast-lane event (e.g. a synthetic mass file-rename or a failed-login burst) and confirm it reaches the server and appears in `alerts` in well under the 20-second batch window, alongside the routine events sent in the same frame.
