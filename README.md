# Project Blueprint: KhemStrix EDR

## 1. Executive Summary

KhemStrix EDR is an open-source-based, AI-enhanced Endpoint Detection & Response (EDR) platform designed specifically for small-to-medium enterprises (SMEs) in Cambodia. While commercial enterprise solutions (e.g., CrowdStrike, SentinelOne) are cost-prohibitive, complex to manage, and extract telemetry outside domestic borders, KhemStrix provides an affordable, sovereign cyber defense alternative.

Inspired by South Korea's AhnLab model—starting with domestic, underserved sectors and growing into a national defense standard—KhemStrix pairs a lightweight, dependency-free Go endpoint agent with a FastAPI/PostgreSQL ingestion engine and localized AI models. It is designed to be easily deployed by non-specialist IT administrators in a private office setting (Model 1) while remaining architecturally ready to scale into a centralized, sovereign threat sensor grid for national authorities such as MPTC and CamCERT (Model 2).

## 2. Problem Statement

**The Cost and Expertise Barrier:** Enterprise EDR solutions require five-figure annual budgets in foreign currency and dedicated Security Operations Center (SOC) personnel. Cambodian SMEs (accounting firms, clinics, logistics hubs, educational institutions) lack both, leaving them completely unmonitored against ransomware and business email compromise (BEC).

**Evolving Local Attack Vectors:** Local organizations run high risks on consumer and enterprise communication channels that standard tools rarely correlate simultaneously—namely business email (phishing/spoofing) and Telegram, which serves as the de facto operational and document-sharing backbone across Cambodian businesses and government agencies.

**Regulatory and Sovereignty Gaps:** Emerging frameworks (MPTC Draft Cybersecurity Law, Draft Personal Data Protection Law, NBC-TCRMG) mandate auditable security monitoring, log retention, and strict data privacy. Small businesses currently have no accessible platform that satisfies compliance without exposing sensitive communications to overseas commercial clouds.

## 3. The Proposed Solution

KhemStrix solves this by delivering an end-to-end telemetry and automated response pipeline built on four principles:

- **Ultra-Lightweight Endpoint Footprint:** A single static Go binary (<30MB RAM) running as a persistent background daemon (systemd on Linux, Windows Service on Windows) with zero runtime dependencies.
- **Privacy-First Inspection (Zero Raw-Data Storage):** Threat evaluation occurs via structured feature extraction (hashes, heuristics, metadata, intent classification). Private message bodies and confidential documents are discarded from memory immediately after inference, never resting on disk.
- **Dual Ingestion Engine:** Asynchronous, decoupled ingestion queues that buffer telemetry spikes locally on low-resource machines without requiring heavy message brokers like Kafka or Redis.
- **Context-Aware AI & MITRE ATT&CK Mapping:** Combining rule-based heuristics, VirusTotal hash validation, and fine-tuned machine learning models to detect social engineering and correlate system anomalies directly to globally recognized adversarial tactics.

## 4. Phased Implementation Roadmap

```
┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐
│  Phase 1  │ ──► │  Phase 2  │ ──► │  Phase 3  │ ──► │  Phase 4  │
│ Server &  │     │   Gmail   │     │ Telegram  │     │ Endpoint  │
│ Agent Reg │     │ Phishing  │     │ Telemetry │     │ Logs      │
└───────────┘     └───────────┘     └───────────┘     └─────┬─────┘
                                                            │
┌───────────┐     ┌───────────┐     ┌───────────┐           │
│  Phase 7  │ ◄── │  Phase 6  │ ◄── │  Phase 5  │ ◄─────────┘
│ Active    │     │ Incident  │     │ AI Engine │
│ Response  │     │ Reporting │     │ & MITRE   │
└───────────┘     └───────────┘     └───────────┘
```

### Phase 1: Server Infrastructure, Agent Registration & Lifecycle Validation

**Objective:** Establish the foundational client-server communication, authentication, database schemas, and background execution loops.

**Key Deliverables:**
- FastAPI backend with PostgreSQL persistence and full UTC timezone alignment.
- Multi-tenancy anchor (`organizations` table) with default tenant scoping.
- Go binary with static compilation for Windows (`.exe`) and Linux (cross-compiled via `GOOS=linux`).
- One-line chained installer for PowerShell and Bash.
- Background service registration: Linux systemd unit and Windows Service execution.
- Dynamic network metadata sync (`ip_address` and `mac_address`) on active heartbeats (`POST /agent/heartbeat`).
- Web dashboard for fleet visibility, agent revocation, reactivation, and hard deletion.

**Current Status:** Complete & Fully Tested.

### Phase 2: Email Telemetry & Phishing Pipeline (Gmail / IMAP)

**Objective:** Detect inbound email phishing, credential harvesting lures, and malicious attachments targeting employee inboxes.

**Key Deliverables:**
- Lightweight Go module to read incoming mail streams (IMAP / Google Workspace APIs).
- In-memory attachment processor: computes SHA-256 hashes locally and discards raw file bytes to preserve corporate confidentiality.
- Standard event packaging adhering to the project's core JSONB schema envelope.
- In-process `asyncio.Queue` on FastAPI to absorb batch flushes from agents without database connection pool exhaustion.
- Initial detection checks: header anomalies (SPF/DKIM/DMARC status), domain lookalikes, and VirusTotal hash reputation lookups against a local query cache.

### Phase 3: Telegram Security Telemetry (Feasibility & Ingestion Research)

**Objective:** Investigate and prototype security telemetry collection for Telegram desktop environments, addressing Cambodia's primary workplace communication vector.

**Key Deliverables:**
- Technical feasibility study on detecting malicious file downloads, suspicious `.tapp`/bot execution links, and unauthorized session establishment via Telegram Desktop.
- Mechanism design: local download directory monitoring (`Telegram Desktop/Downloads`) vs. Telegram Bot API gateway integrations.
- Feature extraction: hashing incoming files downloaded through the app and flagging lookalike domains distributed in corporate group chats.
- Event contract extension: mapping Telegram-derived alerts into the standard events schema under `event_type: "messaging_threat"`.

### Phase 4: Endpoint Core System Telemetry

**Objective:** Expand the Go agent into a true system monitor capturing host-level activity beyond communication apps.

**Key Deliverables:**
- **Process Monitoring:** Tracking process spawn events, parent-child relationships (e.g., `word.exe` spawning `powershell.exe`), and command-line execution flags.
- **File Integrity Monitoring (FIM):** Tracking unauthorized file modifications, creations, and extension changes across sensitive directories (`/etc`, `System32`, user Desktop).
- **Authentication & Login Auditing:** Capturing interactive and remote login attempts, failed logon spikes, privilege escalations, and SSH/RDP session states.
- **Network Socket Logging:** Recording active outbound socket connections (remote IP, target port, binary binding) to detect Command-and-Control (C2) callbacks.
- **Agent-side ring buffering:** thread-safe caching (50 events / 20s flush with jitter) to ensure zero log loss during network drops.

### Phase 5: AI-Driven Analytics & MITRE ATT&CK Mapping

**Objective:** Transform raw, noisy system and communication logs into categorized, high-confidence security incidents.

**Key Deliverables:**
- **NLP / Social Engineering Classifier:** Offline-trained model (TF-IDF / LightGBM or fine-tuned DistilBERT) evaluating email and chat intent to flag urgency manipulation, brand spoofing (e.g., ABA Bank, Wing), and credential traps without storing raw text.
- **System Anomaly Detection:** Behavioral models spotting anomalous execution chains and Living-off-the-Land Binaries (LOLBins).
- **MITRE ATT&CK Tagging:** Automated normalization engine that labels alerts with precise tactical IDs:
  - Initial Access: Phishing (T1566)
  - Execution: Command and Scripting Interpreter (T1059)
  - Persistence: Create or Modify System Process (T1543)
  - Command and Control: Application Layer Protocol (T1071)
- Monthly time-partitioned PostgreSQL storage (`events_YYYY_MM`) for scalable retention and compliance reporting.

### Phase 6: Automated Incident Reporting, Mitigations & Remediation Guidance

**Objective:** Provide actionable, plain-language intelligence tailored for non-specialist SME administrators and regulatory compliance audits.

**Key Deliverables:**
- Incident report generator synthesizing end-to-end attack timelines from the initial phishing trigger to endpoint persistence.
- Clear remediation playbooks accompanying every alert:
  - **Immediate Containment:** Instructions to revoke compromised credentials or disconnect affected network segments.
  - **Mitigation:** System configuration hardening steps to block recurring techniques.
- Audit export module producing standardized summary PDFs aligned with MPTC incident reporting guidelines and NBC audit checklists.

### Phase 7: Active Response & Automated Containment

**Objective:** Move from passive detection to active threat neutralization, containing attacks before lateral movement occurs.

**Key Deliverables:**
- Server-to-agent command dispatch pipeline (via persistent bidirectional channels or high-frequency polling).
- **Process Termination:** Remote command execution to terminate malicious process trees (`kill -9 <PID>`).
- **Endpoint Isolation:** Local firewall rule manipulation (Windows Filtering Platform / iptables) to sever all network connectivity except the secure heartbeat channel back to the KhemStrix server.
- **Artifact Quarantine:** Moving suspicious downloads and dropped malware into an encrypted, isolated system directory.
- **Safety UX:** Implementation of a "Human-in-the-Loop" confirmation flow on the React dashboard to prevent automated containment actions from accidentally disrupting core business workflows.

## 5. Team

| Name | Focus Area | Role |
|---|---|---|
| Rong Sovannorth | Cybersecurity | Lead, Dev, Assists Research, AI and ML |
| Roth Monyreach | Cybersecurity | Security Research, Assists Dev |
| Kea Sophanh | AI and ML | AI and ML, Assists Research |