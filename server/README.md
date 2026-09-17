# Project Blueprint: KhemStrix EDR

## 1. Executive Summary

KhemStrix EDR is an open-source-based, AI-enhanced Endpoint Detection & Response (EDR) platform designed specifically for small-to-medium enterprises (SMEs) in Cambodia. While commercial enterprise solutions (e.g., CrowdStrike, SentinelOne) are cost-prohibitive, complex to manage, and extract telemetry outside domestic borders, KhemStrix provides an affordable, sovereign cyber defense alternative.

Inspired by South Korea's AhnLab model—starting with domestic, underserved sectors and growing into a national defense standard—KhemStrix pairs a lightweight, dependency-free Go endpoint agent with a FastAPI/PostgreSQL ingestion engine and localized AI models. It is designed to be easily deployed by non-specialist IT administrators in a private office setting (Model 1) while remaining architecturally ready to scale into a centralized, sovereign threat sensor grid for national authorities such as MPTC and CamCERT (Model 2).

**Architectural note (current revision):** the original design assumed periodic HTTP-based telemetry batching. Development has since moved to a **persistent WebSocket transport with a durable local outbox**, because continuous endpoint telemetry (process, file, auth, network events) cannot rely on the same request/response model used for agent registration and management. Section 6 documents this change and why it happened.

## 2. Problem Statement

**The Cost and Expertise Barrier:** Enterprise EDR solutions require five-figure annual budgets in foreign currency and dedicated Security Operations Center (SOC) personnel. Cambodian SMEs (accounting firms, clinics, logistics hubs, educational institutions) lack both, leaving them completely unmonitored against ransomware and business email compromise (BEC).

**Evolving Local Attack Vectors:** Local organizations run high risks on consumer and enterprise communication channels that standard tools rarely correlate simultaneously—namely business email (phishing/spoofing) and Telegram, which serves as the de facto operational and document-sharing backbone across Cambodian businesses and government agencies.

**Regulatory and Sovereignty Gaps:** Emerging frameworks (MPTC Draft Cybersecurity Law, Draft Personal Data Protection Law, NBC-TCRMG) mandate auditable security monitoring, log retention, and strict data privacy. Small businesses currently have no accessible platform that satisfies compliance without exposing sensitive communications to overseas commercial clouds.

## 3. Deployment Models: On-Premise SME vs. Centralized Sovereign Cloud

KhemStrix is engineered from a single codebase to support two deployment realities without requiring architectural rewrites as the project scales.

### Model 1: Autonomous On-Premise (Private SME Node)

Designed for individual Cambodian businesses—such as clinics, accounting firms, and local logistics offices—that require total data privacy and operate on strict hardware budgets. In this model, the entire backend (FastAPI, PostgreSQL, and the management dashboard) runs locally on an existing office workstation or mini-PC inside the company LAN. The Go agent reports directly to this local node with zero outbound cloud dependencies. To run reliably on low-spec hardware without crashing, event ingestion uses an in-process asynchronous queue rather than external brokers like Redis, and detection relies on lightweight, offline-trained models. No company telemetry, email metadata, or file hashes ever leave the physical office network.

### Model 2: Sovereign Managed Cluster (Centralized Multi-Tenant Hub)

Designed for managed service providers, domestic telecom operators, or national cybersecurity authorities (such as MPTC and CamCERT) to deliver managed threat detection to micro-businesses that lack on-premise servers. In this model, the backend is hosted centrally in a domestic cloud facility. Hundreds of external organizations deploy the lightweight Go agent and point outbound over HTTPS/WSS to this central cluster. Strict multi-tenancy is enforced at the database layer via organization identifiers, ensuring complete data isolation between businesses while enabling the central platform to aggregate anonymized threat signatures into a collective national threat radar.

### Unified Architecture Strategy

To support both environments from day one, the team builds against the multi-tenant database schema and decoupled event pipeline immediately. For local SME use (Model 1), the system assigns all endpoints to a default organization identifier, operating as a self-contained node. When scaling to a centralized provider (Model 2), the identical server code simply registers additional organization records, eliminating the need to refactor database models or agent networking later.

## 4. Telemetry Transport Architecture (WebSocket)

### 4.1 Why the Architecture Moved to WebSocket

The original plan treated telemetry the same way as agent registration and management: discrete HTTP requests. That model works fine for infrequent operations (register agent, get status, request configuration), but endpoint security telemetry is fundamentally different — an endpoint can generate a continuous stream of process, file, login, and network events during normal operation.

A periodic batch-over-HTTP approach introduced three problems:
- **Latency** — an important event generated right after a batch was sent would wait for the next cycle.
- **Connection overhead** — repeated HTTP requests for a continuous stream is wasteful.
- **Failure handling** — if the server was briefly unreachable, there was no defined behavior for what happens to the event. Silently discarding it was not acceptable, since telemetry's entire value is a complete record of what happened on the endpoint.

This led to the current design: a **persistent WebSocket connection**, backed by **durable local storage**, so the agent never depends on the server being reachable at the exact moment an event occurs.

### 4.2 Architecture

```
Telemetry Collector
        ↓
   Event Model
        ↓
Durable Local Outbox (SQLite, WAL mode)
        ↓
 WebSocket Transport
        ↓
KhemStrix Server (WebSocket Manager + Event Processing)
        ↓
     PostgreSQL
```

Collectors do not talk to the network directly. A process monitor produces a process event; a file monitor produces a file event. The shared pipeline (event model → outbox → WebSocket) handles persistence and delivery, so every telemetry source benefits from the same reliability guarantees without reimplementing them.

### 4.3 Reliability Mechanisms

| Mechanism | Purpose |
|---|---|
| Persistent connection | Avoids per-event connection overhead; events stream continuously rather than in isolated request/response cycles |
| Durable local outbox | Events are written locally *before* transmission is attempted, and are only removed once the server acknowledges receipt |
| Reconnection | Agent detects a dropped connection and retries, with randomized jitter to avoid reconnect storms when many agents drop at once |
| Reconciliation | On reconnect, the agent determines which queued events the server is missing and resends only those |
| Duplicate protection | Each event carries a unique `event_id`; the server treats a repeat delivery of the same ID as a no-op rather than a new row, since at-least-once delivery implies duplicates are possible |
| Heartbeat | ~10s interval ping/response used to distinguish a genuinely alive connection from one that appears open but isn't |

### 4.4 Current Validation Status

The mechanisms above have moved from "designed" to **tested against a real running backend and real agents**, including:

- ✅ Durable local storage — events survive a server outage instead of being dropped
- ✅ Reconnection — agent detects the outage and reconnects once the server returns, without manual intervention
- ✅ Reconciliation — validated with both a single queued event and a batch (15 events queued during an outage, all delivered cleanly on reconnect)
- ✅ Duplicate protection — verified directly in PostgreSQL; no repeated `event_id` rows despite retries across multiple test runs
- ✅ Multi-agent isolation — validated with two independently registered agents (one Windows, one Linux/Kali) pushing events, including overlapping backlogs reconciling at the same time; all events landed under the correct `agent_id` with no cross-contamination

**Still open, before the transport layer is considered production-ready:**
- Full integration into the real agent runtime (current validation used a standalone test harness, not the production collectors)
- Backlog behavior at much larger scale (the design target is on the order of 100,000 queued events; only tens of events have been tested so far) and **chunked reconciliation** to avoid resending a huge backlog as one operation
- Outbox size limits, disk usage behavior, and retention policy for long outages (hours to days)
- Heartbeat/timeout behavior under degraded (not just cleanly killed) network conditions

## 5. Event Contract

Before telemetry collectors are built out, the project needs one consistent event structure shared across the outbox, WebSocket messages, backend validation, database storage, and future detection rules. Earlier prototypes used inconsistent field naming (`event_type`/`raw_data` vs. `class_uid`/`category_uid`); this needs to be finalized to avoid every collector producing a slightly different shape.

Proposed common envelope:

```json
{
  "event_id": "unique-event-id",
  "agent_id": "agent-id",
  "event_type": "process_start",
  "timestamp": "2026-09-15T10:30:00Z",
  "data": {
    "process": "powershell.exe",
    "pid": 4820,
    "parent_pid": 3210,
    "command_line": "..."
  },
  "schema_version": 1
}
```

Different telemetry sources share the outer envelope while storing source-specific detail under `data`.

## 6. Phased Implementation Roadmap

```
┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐
│  Phase 1  │ ──► │  Phase 2  │ ──► │  Phase 3  │ ──► │  Phase 4  │
│ Server &  │     │ WebSocket │     │ Endpoint  │     │  Rule-    │
│ Agent Reg │     │ Transport │     │ Telemetry │     │  Based    │
│           │     │ (current) │     │           │     │ Detection │
└───────────┘     └───────────┘     └───────────┘     └─────┬─────┘
                                                            │
┌───────────┐     ┌───────────┐     ┌───────────┐           │
│  Phase 7  │ ◄── │  Phase 6  │ ◄── │  Phase 5  │ ◄─────────┘
│ Active    │     │ Incident  │     │ Email /   │
│ Response  │     │ Reporting │     │ Telegram/ │
│           │     │           │     │ AI / ML   │
└───────────┘     └───────────┘     └───────────┘
```

The roadmap has been reordered from the original plan. Email and Telegram telemetry, AI/ML detection, and active response all remain part of the product, but they are pushed later because none of them are useful without a reliable telemetry foundation underneath them. A detection rule that correlates process and network events cannot exist if those events aren't being collected reliably in the first place.

### Phase 1: Server Infrastructure, Agent Registration & Lifecycle Validation

**Objective:** Establish the foundational client-server communication, authentication, database schemas, and background execution loops.

**Key Deliverables:**
- FastAPI backend with PostgreSQL persistence and full UTC timezone alignment.
- Multi-tenancy anchor (`organizations`/`tenant_id`) with default tenant scoping.
- Go binary with static compilation for Windows (`.exe`) and Linux (cross-compiled via `GOOS=linux`).
- One-line chained installer for PowerShell and Bash.
- Background service registration: Linux systemd unit and Windows Service execution.
- Dynamic network metadata sync (`ip_address` and `mac_address`) on active heartbeats (`POST /agent/heartbeat`).
- Web dashboard for fleet visibility, agent revocation, reactivation, and hard deletion.

**Current Status:** Complete. Validated with two independently registered agents (Windows + Linux) both active and reporting.

### Phase 2: WebSocket Transport & Durable Telemetry Pipeline *(current phase)*

**Objective:** Establish a reliable, persistent, failure-tolerant communication path between agent and server, capable of carrying a continuous stream of security events without loss — the prerequisite for all telemetry collection that follows.

**Key Deliverables:**
- Persistent WebSocket connection between agent and backend, with authentication.
- Durable local outbox (SQLite, WAL mode) so events survive disconnects.
- Reconnection with jittered backoff to avoid reconnect storms across a fleet.
- Reconciliation of queued events on reconnect.
- Idempotent, duplicate-safe event delivery via unique `event_id`.
- Heartbeat-based connection health detection.
- Finalized, versioned event contract shared across collector → outbox → transport → backend → database.

**Current Status:** Core mechanisms implemented and validated against a live backend (see Section 4.4). Real-agent runtime integration, large-scale backlog handling, and chunked reconciliation remain open.

### Phase 3: Endpoint Core System Telemetry

**Objective:** Expand the Go agent into a true system monitor capturing host-level activity, using the Phase 2 pipeline.

**Key Deliverables:**
- **Process Monitoring:** process spawn events, parent-child relationships (e.g., `winword.exe` spawning `powershell.exe`), and command-line execution flags.
- **File Integrity Monitoring (FIM):** file creation, modification, deletion, and rename events across sensitive directories (`/etc`, `System32`, user Desktop/Downloads), starting with a curated directory set rather than the whole filesystem.
- **Authentication & Login Auditing:** interactive and remote login attempts, failed logon spikes, privilege escalations, and SSH/RDP session states.
- **Network Socket Logging:** active outbound socket connections (remote IP, target port, binary binding) to detect Command-and-Control (C2) callbacks.

The goal at this stage is accurate collection, not classification — answering "what happened on the endpoint," not yet "was it suspicious."

### Phase 4: Rule-Based Detection

**Objective:** Build deterministic detection rules on top of validated telemetry — e.g., suspicious process relationships (Office app → PowerShell), brute-force login patterns, suspicious file-then-execute sequences, and unexpected outbound network activity.

### Phase 5: Email, Telegram, and AI/ML Detection

**Objective:** Extend telemetry coverage to email (Gmail/IMAP phishing detection) and Telegram (Cambodia's dominant workplace communication channel), and layer AI/ML analysis — NLP-based social engineering classification, behavioral anomaly detection, and MITRE ATT&CK tagging — on top of the by-then-mature telemetry and detection foundation.

### Phase 6: Automated Incident Reporting

**Objective:** Provide actionable, plain-language intelligence tailored for non-specialist SME administrators and regulatory compliance audits — end-to-end attack timelines, remediation playbooks, and audit-ready export aligned with MPTC/NBC guidelines.

### Phase 7: Active Response & Automated Containment

**Objective:** Move from passive detection to active threat neutralization — remote process termination, endpoint network isolation, artifact quarantine, and a human-in-the-loop confirmation flow on the dashboard before any automated containment action executes.

## 7. Team

| Name | Focus Area | Role |
|---|---|---|
| Rong Sovannorth | Cybersecurity | Lead, Dev, Assists Research, AI and ML |
| Roth Monyreach | Cybersecurity | Security Research, Assists Dev |
| Kea Sophanh | AI and ML | AI and ML, Assists Research |