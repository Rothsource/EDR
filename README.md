🛡️ KhemStrix EDR
Affordable, Localized Cybersecurity Detection and Response for Cambodian Organizations
1. Executive Summary

KhemStrix EDR is an endpoint security and threat detection platform designed to provide affordable cybersecurity monitoring for small and medium-sized organizations, particularly in Cambodia.

Many organizations cannot afford enterprise security platforms such as CrowdStrike or SentinelOne, and they may not have dedicated Security Operations Center (SOC) teams or cybersecurity specialists. As a result, important systems and employee devices may operate with limited security visibility.

KhemStrix aims to provide a practical alternative by combining:

A lightweight endpoint agent
Centralized security event collection
Endpoint behavior monitoring
Rule-based threat detection
AI and machine learning analysis
MITRE ATT&CK mapping
Incident reporting and remediation guidance

The platform will initially focus on monitoring endpoints and detecting suspicious activity such as malicious process execution, suspicious parent-child process relationships, repeated failed login attempts, unusual network connections, and unauthorized file activity.

KhemStrix is not intended to immediately compete directly with large enterprise EDR platforms. Instead, the goal is to provide an accessible cybersecurity monitoring platform that can be deployed and managed by organizations with limited cybersecurity resources.

The long-term vision is to support both:

Private On-Premise Deployment for organizations that want to keep security telemetry inside their own network.
Centralized Managed Deployment for IT service providers or cybersecurity providers that want to manage multiple organizations from a central platform.

The core product value is simple:

KhemStrix helps organizations understand what is happening on their endpoints, detect suspicious activity, and respond before a security incident becomes more serious.

2. Problem Statement
2.1 Limited Cybersecurity Visibility

Many small and medium-sized organizations do not have continuous visibility into what happens on their computers and servers.

They may not know:

Which processes are running
Which applications are connecting to external networks
Whether suspicious PowerShell commands are being executed
Whether repeated login attempts are occurring
Whether important files are being modified
Whether a phishing attack has led to malicious activity on an endpoint

Traditional antivirus software may detect known malware, but organizations may still lack visibility into suspicious behavior and attack chains.

For example:

Employee receives phishing email
        ↓
Employee opens malicious document
        ↓
Document launches PowerShell
        ↓
PowerShell downloads additional payload
        ↓
Suspicious network connection

Without endpoint monitoring and event correlation, these activities may appear as separate and unrelated events.

2.2 Cost and Expertise Barrier

Enterprise security products can be expensive and often require:

Dedicated security personnel
Security Operations Center capabilities
Complex deployment
Continuous monitoring
Security expertise

Many smaller organizations cannot justify these costs.

KhemStrix aims to reduce this barrier by providing a lightweight and practical security monitoring platform.

2.3 Limited Localized Security Solutions

Organizations may prefer security solutions that:

Support local deployment
Minimize unnecessary data exposure
Are understandable for non-specialist IT administrators
Can be managed by local IT or cybersecurity providers
Provide actionable alerts rather than large volumes of raw logs

KhemStrix aims to focus on these requirements.

3. Solution

KhemStrix consists of four main layers:

┌──────────────────────────────┐
│       KhemStrix Agent        │
│                              │
│ Process Monitoring           │
│ File Monitoring              │
│ Authentication Monitoring    │
│ Network Monitoring           │
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│       Event Platform         │
│                              │
│ Event API                    │
│ Event Queue / Buffer         │
│ PostgreSQL Storage           │
│ Agent Management             │
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│      Detection Engine        │
│                              │
│ Rule-Based Detection         │
│ Behavioral Analysis          │
│ AI / ML Detection            │
│ Risk Scoring                 │
│ MITRE ATT&CK Mapping         │
└──────────────┬───────────────┘
               │
               ▼
┌──────────────────────────────┐
│      Security Dashboard      │
│                              │
│ Alerts                       │
│ Attack Timeline              │
│ Endpoint Status              │
│ Incident Reports             │
│ Response Actions             │
└──────────────────────────────┘

The development approach will start with reliable telemetry and simple detection rules before introducing more advanced AI models.

The principle is:

First collect good security data. Then detect suspicious behavior. Then improve detection using AI and machine learning.

4. Deployment Models
Model 1 — Private On-Premise Deployment

KhemStrix can be deployed inside an organization's own network.

Endpoint Agent
      │
      ▼
Local KhemStrix Server
      │
      ├── FastAPI Backend
      ├── PostgreSQL
      └── Security Dashboard

Security events remain within the organization's infrastructure.

This model may be suitable for:

Clinics
Accounting firms
Educational institutions
Organizations handling sensitive information
Businesses with strict privacy requirements
Model 2 — Centralized Managed Platform

A central KhemStrix platform can manage multiple organizations.

Organization A Agents ──┐
Organization B Agents ──┼──► KhemStrix Platform
Organization C Agents ──┘
                              │
                              ▼
                       Multi-Tenant System
                              │
                              ▼
                     Security Management

This model could eventually allow:

IT service providers
Managed Security Service Providers
Cybersecurity companies

to provide managed cybersecurity monitoring to multiple customers.

5. Development Roadmap

The project will follow a revised development roadmap.

Instead of immediately building email, Telegram, and AI features, KhemStrix will first establish a strong endpoint monitoring foundation.

PHASE 1
Agent Registration & Management
        │
        ▼
PHASE 2
Endpoint Telemetry
        │
        ▼
PHASE 3
Rule-Based Detection
        │
        ▼
PHASE 4
Email & Phishing Telemetry
        │
        ▼
PHASE 5
AI / ML Detection & MITRE ATT&CK
        │
        ▼
PHASE 6
Telegram / Additional Threat Telemetry
        │
        ▼
PHASE 7
Incident Reporting & Investigation
        │
        ▼
PHASE 8
Active Response & Containment
6. Phase 1 — Agent Registration and Management
Status: ✅ Completed / Current Foundation

The first phase establishes communication between the KhemStrix endpoint agent and the backend platform.

Features
Endpoint agent registration
Agent authentication
Agent heartbeat
IP address synchronization
MAC address synchronization
Agent status monitoring
Agent activation and revocation
Endpoint management
Dashboard visibility
Architecture
Go Agent
    │
    │ Register / Authenticate
    ▼
FastAPI Backend
    │
    ▼
PostgreSQL
    │
    ▼
Dashboard

This phase provides the foundation for future security telemetry.

7. Phase 2 — Endpoint Core Telemetry
Status: 🚧 Next Development Phase

This is the current priority.

The KhemStrix agent will begin collecting security-relevant events from the endpoint.

7.1 Process Monitoring

The agent will monitor process creation and execution.

Collected information may include:

Process name
Process ID
Parent Process ID
Parent process
Executable path
Command line
Username
Timestamp

Example:

{
  "event_type": "process_start",
  "process": "powershell.exe",
  "pid": 4820,
  "parent_process": "winword.exe",
  "parent_pid": 3210,
  "command_line": "powershell.exe -enc ...",
  "username": "user",
  "timestamp": "2026-09-10T10:30:00Z"
}

Example suspicious behavior:

winword.exe
      ↓
powershell.exe
      ↓
external network connection

This event chain may indicate suspicious activity and can later be analyzed by both rule-based and AI detection.

7.2 File Activity Monitoring

The agent will monitor important directories for file activity.

Events may include:

File created
File modified
File deleted
File renamed
File extension changed

Initial focus may include:

Windows
Downloads
Desktop
Important system directories
Linux
/etc
/tmp
Selected user directories

The goal is initially to collect reliable telemetry rather than immediately classify files as malicious.

7.3 Authentication Monitoring

The system will collect authentication-related events such as:

Successful login
Failed login
Logout
Remote login
SSH sessions
RDP sessions
Privilege-related events

Example:

10 failed logins
        ↓
Successful login
        ↓
Suspicious process execution

Later phases can correlate these events into an attack timeline.

7.4 Network Monitoring

The agent will collect network connection information.

Example fields:

Process
Local IP
Local Port
Remote IP
Remote Port
Protocol
Timestamp

Example:

powershell.exe
      ↓
192.168.1.20:49152
      ↓
Suspicious-Remote-IP:443
8. Event Pipeline

Security collectors should not directly communicate with the database individually.

Instead:

Process Monitor ──┐
File Monitor ─────┤
Login Monitor ────┤
Network Monitor ──┘
                  │
                  ▼
              Event Queue
                  │
                  ▼
             Event Batching
                  │
                  ▼
             FastAPI Backend
                  │
                  ▼
               PostgreSQL
                  │
                  ▼
                Dashboard

The agent should also support temporary buffering if the server is unavailable.

Event Generated
      │
      ▼
Local Buffer
      │
      ├── Server Available ──► Send Event
      │
      └── Server Offline ────► Keep Temporarily
                                      │
                                      ▼
                                Retry Later
9. Phase 3 — Rule-Based Detection

Before building complex AI models, KhemStrix will implement understandable security rules.

Examples:

Rule 1 — Suspicious Parent-Child Process
WINWORD.EXE
       ↓
POWERSHELL.EXE

Result:

Alert: Suspicious Process Execution
Severity: High
Rule 2 — Brute Force Attempt
Multiple failed logins
within a defined time period

Result:

Alert: Possible Brute Force Attempt
Severity: Medium / High
Rule 3 — Suspicious File Activity
Executable created in Downloads
        ↓
Immediately executed

Result:

Alert: Suspicious File Execution
Rule 4 — Suspicious Network Behavior
Unusual process
        ↓
External connection

Result:

Alert: Suspicious Network Connection
10. Phase 4 — Email and Phishing Telemetry

Once endpoint telemetry is working, KhemStrix can begin monitoring phishing-related events.

Possible information:

Sender domain
SPF status
DKIM status
DMARC status
Suspicious URLs
Domain lookalikes
Attachment hashes
Email risk indicators

Example attack chain:

Phishing Email
       ↓
Malicious Attachment
       ↓
File Downloaded
       ↓
File Executed
       ↓
Suspicious Process
       ↓
Network Connection

The objective is not only to detect individual events, but eventually to reconstruct the entire attack chain.

11. Phase 5 — AI, Machine Learning and MITRE ATT&CK

AI and ML will be introduced after sufficient telemetry and datasets are available.

The AI/ML system may include:

Phishing Classification

Example:

Email Content / Metadata
          │
          ▼
   Feature Extraction
          │
          ▼
      ML Model
          │
          ▼
      Risk Score

Output:

Classification: Suspicious
Risk Score: 94%

Indicators:
- Urgency manipulation
- Brand impersonation
- Suspicious domain
- Credential request
Endpoint Behavioral Analysis

The system can analyze events such as:

Parent-child process relationships
Command-line behavior
Process frequency
Login patterns
Network connections
File activity

Example:

Normal Behavior
       │
       ▼
Behavioral Analysis
       │
       ▼
Suspicious / High Risk
Risk Scoring

KhemStrix may combine multiple detection signals.

Rule Detection ───┐
                  │
AI Detection ─────┼──► Risk Scoring
                  │
Threat Intelligence┘
                        │
                        ▼
                      Alert

Example:

Rule Score:        70
AI Score:          85
Network Risk:      60

Final Risk Score:  High
MITRE ATT&CK Mapping

Detected activity can be mapped to relevant MITRE ATT&CK techniques.

Example:

Suspicious PowerShell Execution
        ↓
MITRE ATT&CK
        ↓
T1059
Command and Scripting Interpreter

This helps cybersecurity analysts understand the behavior in a standardized format.

12. Phase 6 — Telegram and Additional Security Telemetry

Telegram may later become a Cambodia-specific research and detection feature.

Possible research areas include:

Monitoring downloaded files
Hashing suspicious downloaded files
Detecting suspicious links
Detecting lookalike domains
Monitoring potentially dangerous file types

The objective is not to unnecessarily collect private conversations.

The focus should be on security-relevant metadata and locally observable security events.

13. Phase 7 — Incident Reporting and Investigation

Once events and detections are available, KhemStrix will generate incident timelines.

Example:

09:10 — Phishing email received
09:12 — Attachment downloaded
09:13 — File executed
09:13 — PowerShell started
09:14 — External connection detected
09:15 — Alert generated

The dashboard can provide:

Incident timeline
Affected endpoint
Related events
Severity
MITRE ATT&CK techniques
Recommended mitigation

The system may also generate security reports for organizations.

14. Phase 8 — Active Response and Containment

This phase will be developed only after detection accuracy and reliability have been validated.

Possible response actions:

Terminate suspicious processes
Quarantine suspicious files
Isolate an endpoint
Block suspicious network connections

Example:

Threat Detected
       │
       ▼
Security Alert
       │
       ▼
Analyst / Administrator Review
       │
       ├── Approve ──► Containment
       │
       └── Reject ───► Continue Monitoring

Human confirmation should be prioritized before high-impact automated actions.
