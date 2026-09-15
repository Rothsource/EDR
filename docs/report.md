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

### 3.1 Database Multi-Tenancy Anchor

- Create an `organizations` table as the root tenant entity.
- Seed a static default UUID (`00000000-0000-0000-0000-000000000001`, "Default SME").
- Add `tenant_id` foreign keys to `agents`, `enrollment_tokens`, and `events`.
- **Why**: Model 1 runs cleanly as a single-tenant instance. Model 2 simply registers additional organization rows, and queries filter via `WHERE tenant_id = :current_tenant` without requiring schema migrations.

### 3.2 Time-Based Partitioning on Telemetry

- High-volume security logs rapidly degrade standard B-Tree indexing.
- Partition the `events` table by `RANGE (timestamp)` in monthly increments (`events_2026_09`).
- Telemetry past the Cambodian regulatory 90-day retention window can be dropped instantly via `DROP TABLE events_YYYY_MM`, avoiding database vacuum locks.

### 3.3 Ingestion Transport — Superseded Design Note

The original plan for this section was edge-side batching (agent buffers up to 50 events or 20 seconds, then flushes over REST into an `asyncio.Queue`). **That plan has since been replaced.** The batching approach is fine for routine telemetry, but too slow for urgent detections — ransomware or brute-force login activity needs the server to know immediately, not up to 20 seconds later. Sections 3.4–3.7 below describe the design that replaced it, and its real, current implementation status.

### 3.4 The Problem: What Happens When a Persistent Connection Dies

Moving to a persistent WebSocket solves the latency problem — events stream immediately, with zero agent-side classification logic. But it introduces a new one: a WebSocket doesn't degrade gracefully the way batched HTTP does. It just dies — from wifi loss, laptop sleep, a proxy, or the server itself going down — and anything in flight at that moment is lost unless handled explicitly. Designing for that failure mode was the core of this phase's work.

### 3.5 The Agreed Design

- **Durable local outbox (SQLite):** every event is written to disk on the endpoint the moment it's generated, before any send is attempted. It is never deleted on send — only once the server confirms receipt. Status flow: pending → sent-but-unacknowledged → acknowledged (row removed).
- **On disconnect:** nothing special has to happen — the event was already safe on disk the instant it was written.
- **On reconnect:** the agent sends the server its list of currently-unacknowledged event IDs; the server replies with which of those it already has; the agent resends only what's genuinely missing. One round trip, not one request per event.
- **Idempotent insert is the real safety net:** the server's insert is keyed on the event ID with "do nothing on conflict," so even a duplicate resend is a harmless no-op. Correctness never depends on the reconcile step working perfectly.
- **Heartbeat is server-initiated:** the server pings roughly every 10 seconds; the agent just replies and resets a timer. If the agent misses 2-3 pings in a row (roughly 20-30 seconds), it assumes the connection is dead, stops trying to push, and lets events accumulate safely in the outbox.
- **Reconnection is agent-initiated**, independent of the heartbeat, using exponential backoff with random jitter — this avoids every agent in the fleet retrying in the same instant if the server itself is what went down.
- **REST keeps a narrow, secondary role:** it only helps in the specific case where a proxy blocks the WebSocket upgrade but the network is otherwise fine. A full outage defeats REST too — the durable outbox is what actually protects against that case, not a REST fallback.

### 3.6 Status: Server Side — Done and Tested

| Component | File | Status |
|---|---|---|
| Message schemas | `server/app/schemas/ws.py` | Done — auth, event, ack, reconcile request/response, ping/pong, and error message shapes |
| Connection registry | `server/app/core/ws_manager.py` | Done |
| WebSocket route | `server/app/routers/ws.py` | Done — handles the auth handshake, idempotent event insert, sending acks, and reconciliation |
| Heartbeat loop | `server/app/core/ws_heartbeat.py` | Done — pings roughly every 10 seconds, disconnects an unresponsive agent after about 30 |

Verified end-to-end: the auth handshake accepts good credentials and rejects bad or malformed ones; heartbeat ping/pong keeps a connection alive; an idle connection is correctly evicted after missed pongs; an event sent over the socket is inserted and acknowledged, with the row confirmed directly in the database; reconciliation was tested by sending one real event ID and one fake one, and only the real one came back as known; the idempotent insert was confirmed to make duplicate sends harmless.

One real bug was found and fixed during testing: the database's event-time column doesn't store timezone information, but incoming timestamps parse as timezone-aware on the server side, which the database driver rejects. The fix strips the timezone before insert. This is a good example of the kind of subtle mismatch that only surfaces once both sides are actually wired together and tested — worth keeping in mind for the Go agent's own timestamp handling as that work continues.

### 3.7 Status: Go Agent Side — In Progress

**Important correction to an earlier version of this status:** an earlier internal status note claimed the agent-side outbox file and a supporting config helper were "already written," and referenced two agent source files (`core/net.go`, `core/http.go`) that turned out not to exist under those names in the real repository (the actual files are `core/netinfo.go` and `core/sender.go`). Before doing any further work, the actual file tree was checked directly, and the claimed-done outbox work did not, in fact, exist yet. It has since been written for real, listed below. The lesson carried forward: status write-ups describe intent until verified against the real repository.

**Actually done and confirmed building successfully:**

- A small helper (`Dir()`) was added to `agent/internal/config/config.go` so other packages can locate the same directory as the agent's existing `config.json` / `state.json` without duplicating the OS-specific path logic.
- The durable local outbox was written at `agent/internal/store/store.go` — a SQLite-backed store with the write/mark-sent/mark-acknowledged/list-unacknowledged/list-pending operations the design in 3.5 calls for. The new SQLite dependency was fetched and the agent module was confirmed to build cleanly with it in place.
- The WebSocket client was written at `agent/internal/wsclient/wsclient.go` — handling the connect-and-authenticate handshake, replying to server pings and detecting a dead connection when they stop arriving, the reconcile-on-reconnect exchange described in 3.5, and the reconnect loop with exponential backoff and jitter. Its message formats were matched directly against the real `server/app/schemas/ws.py` field names rather than assumed. **This file has not yet been build-tested** — it depends on a UUID library that has not yet been fetched into the agent module (see 3.8, step 1).

**Not yet done:**

- The WebSocket client has not yet been wired into the agent's main run loop (`agent/internal/core/run.go`), so it isn't actually running as part of the agent yet.
- No detection module currently generates events to send — the email, process, network, file, and response modules under `agent/internal/modules/` don't yet call into the new WebSocket client. A temporary way to generate a test event will be needed before real integration testing can happen.
- No integration testing has been performed yet — this is the most important gap. Nothing described here has been proven to work end-to-end against a live server.

### 3.8 What's Left — Step by Step

1. **Fetch the remaining Go dependency and confirm the WebSocket client actually builds.** This is the first real build test of the new client code and the immediate next action.
2. **Wire the WebSocket client into the agent's run loop**, so it starts alongside the existing heartbeat cycle, sharing the same agent credentials and shutdown handling.
3. **Add a temporary way to generate a test event**, since no detection module produces one yet, so the integration test in step 4 has something real to send.
4. **Run a full integration test against a live server:** disconnect the agent mid-stream (wifi off, or the server itself stopped), confirm the event lands safely in the local outbox file, bring the connection back, and confirm the server ends up with exactly one copy of the event — no loss, no duplication.
5. **Run a reconnect-storm test:** start several agent instances at once, stop the server, bring it back, and confirm from the logs that their reconnect attempts are spread out rather than all landing in the same instant.
6. **Known follow-ups, not blocking but worth tracking:** the local outbox needs a bounded-size policy so a very long outage can't fill the endpoint's disk unnoticed; the reconcile exchange should be chunked if a backlog gets very large; and the fact that a disconnected agent pauses detection entirely (rather than falling back to anything) is an accepted tradeoff that should be written down somewhere visible, such as an operations runbook.

## 4. The Standard Event Contract

**Note:** the field names below reflect the original design intent for this contract, but the event schema actually implemented and tested in `server/app/schemas/ws.py` uses different field names (classification-code style fields such as class/category/activity/type/severity identifiers, plus a generic data payload) rather than the `event_type` / `raw_data` / `extracted_features` shape shown here. The two should be reconciled — either update this section to match the real schema, or confirm the real schema needs to change to match this one — before more telemetry modules are built against either version.

Every future telemetry module (email phishing today; process execution, network sockets, and file integrity monitoring in Phase 2) was originally intended to adhere to this standard event envelope:

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

- **Service Precedence Resolution**: Remove hardcoded `--server=` arguments from `/etc/systemd/system/khemstrix-agent.service`. Ensure `cmd/agent/main.go` gives precedence to `config.json` over CLI flag defaults. *(Not started.)*
- **Windows Service Verification**: Audit Windows service execution parameters to ensure runtime settings are not overridden. *(Not started.)*
- **Durable Outbox** (`agent/internal/store/store.go`): SQLite-backed local store so no event is lost when the connection drops. *(Done, confirmed building.)* Supersedes the earlier ring-buffer plan — the outbox replaces in-memory buffering with something that survives a crash or restart, not just a network blip.
- **WebSocket Client** (`agent/internal/wsclient/wsclient.go`): Persistent connection, authentication, heartbeat handling, reconnect with backoff and jitter, and reconciliation against the outbox on every reconnect. *(Written, not yet build-tested or wired in — see section 3.8.)* Supersedes the earlier `SendEvents()` batch-dispatcher plan — events stream immediately instead of waiting on a batch window.
- **IMAP Telemetry Module** (`agent/internal/modules/email/`): Connect via IMAP, extract headers, extract links, compute attachment SHA-256 hashes in memory, and purge raw bytes. *(Not started — this is also the first real source of events the WebSocket client will have once it exists.)*
- **Binary Pipeline**: Cross-compile updated binaries and maintain files in `server/app/static/binaries/`. *(Not started for this phase.)*

### AI Teammate Responsibilities (Machine Learning & Ingestion)

- **Dataset Curation & Preprocessing**: Assemble public phishing corpora (Nazario, SpamAssassin, Kaggle, Hugging Face). Generate synthetic samples modeling local Cambodian lures (ABA Bank, Canadia, Wing, Telegram verification).
- **Model Training & Evaluation**: Train a baseline tabular classifier (TF-IDF vectorizer + Random Forest / XGBoost) on phishing language, urgency indicators, and link patterns. Evaluate precision, recall, and false-positive rates for her academic defense.
- **Export Trained Pipeline**: Serialize the model and vectorizer to `.joblib` artifacts for integration into FastAPI.
- **FastAPI Scoring Pipeline** (`server/app/detection/email_scorer.py`): Load artifacts into memory on startup; accept incoming event text and features; return threat probabilities, risk tiers (safe, suspicious, malicious), and threat indicators.
- **VirusTotal Hash Cache Client**: Build `server/app/detection/virustotal.py` to query SHA-256 hashes against a local cache table before consuming public API quotas.
- **Database Migration & Schemas**: Execute SQL migration for `organizations` and partitioned `events`; implement `schemas/event.py` with the UTC serializer. *(Not started.)* The `asyncio.Queue` worker item is superseded — events now arrive over the persistent WebSocket route (`server/app/routers/ws.py`) rather than a queued REST endpoint, so no separate queue worker is needed for this path.

## 7. Next Implementation Steps (Priority Order)

This replaces the previous step list, which was written against the retired REST-batching design (`asyncio.Queue`, `POST /agent/events`, agent-side ring buffer). The transport decision has since moved to the persistent WebSocket design in section 3, so the steps below reflect that instead.

### Step 1: Finish and Prove Out the WebSocket Agent Path
- Fetch the one remaining Go dependency the WebSocket client needs and confirm the agent module still builds cleanly.
- Wire the WebSocket client into the agent's main run loop (`agent/internal/core/run.go`) so it runs alongside the existing heartbeat cycle.
- Add a temporary way to generate a test event, since no real detection module exists yet.
- Run the full disconnect/reconnect integration test described in section 3.8, confirming no event is lost and none is duplicated.
- Run the reconnect-storm test with several agent instances to confirm jitter is actually spreading out reconnect attempts.

### Step 2: Configuration & Service Precedence Fix
- Remove the hardcoded `--server=` flag from the Linux systemd unit file and reload the daemon.
- Ensure `cmd/agent/main.go` gives priority to the values already saved in `config.json` over CLI flag defaults.

### Step 3: Database Multi-Tenancy & Partitioning
- Run the SQL migration to create the `organizations` table and add `tenant_id` to the existing tables.
- Create the monthly partitioned events table with indexes appropriate for tenant- and agent-scoped queries.
- Update the SQLAlchemy models in `server/app/db/models.py` to match.

### Step 4: Reconcile the Event Contract
- Resolve the mismatch flagged in section 4 between the originally-designed event envelope and the event schema actually implemented in `server/app/schemas/ws.py`, so every future telemetry module (email, process, network, file integrity) is built against one agreed shape rather than two conflicting ones.

### Step 5: First Real Telemetry Module — Email Phishing Detection
- Build the IMAP telemetry module (`agent/internal/modules/email/`) as the agent's first real event source, feeding the now-working WebSocket path from Step 1 instead of sitting idle behind it.
- Curate and preprocess phishing datasets, train the baseline classifier, and export the trained pipeline as planned in section 6.
- Build the FastAPI scoring pipeline (`server/app/detection/email_scorer.py`) and the VirusTotal hash-cache client, and hook scoring into the event path that now exists.

### Step 6: End-to-End System Verification
- Trigger a real phishing-style test event from the IMAP module and confirm it streams over the WebSocket, lands in PostgreSQL, and is correctly scored.
- Confirm raw email bodies are never written to disk, per the zero-persistence requirement in section 5.
- Confirm the durable-delivery guarantees from section 3 still hold with a real detection module in the loop, not just a synthetic test event.