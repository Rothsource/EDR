# 🛡️ KhemStrix EDR

## Affordable, Localized Cybersecurity Detection and Response for Cambodian Organizations

---

# 1. Executive Summary

**KhemStrix EDR** is an endpoint security and threat detection platform designed to provide affordable cybersecurity monitoring for small and medium-sized organizations, particularly in Cambodia.

Many organizations cannot afford enterprise security platforms such as CrowdStrike or SentinelOne, and they may not have dedicated Security Operations Center (SOC) teams or cybersecurity specialists. As a result, important systems and employee devices may operate with limited security visibility.

KhemStrix aims to provide a practical alternative by combining:

* A lightweight endpoint agent
* Centralized security event collection
* Endpoint behavior monitoring
* Rule-based threat detection
* AI and machine learning analysis
* MITRE ATT&CK mapping
* Incident reporting and remediation guidance

The platform will initially focus on monitoring endpoints and detecting suspicious activity such as malicious process execution, suspicious parent-child process relationships, repeated failed login attempts, unusual network connections, and unauthorized file activity.

KhemStrix is not intended to immediately compete directly with large enterprise EDR platforms. Instead, the goal is to provide an accessible cybersecurity monitoring platform that can be deployed and managed by organizations with limited cybersecurity resources.

The long-term vision is to support both:

1. **Private On-Premise Deployment** for organizations that want to keep security telemetry inside their own network.
2. **Centralized Managed Deployment** for IT service providers or cybersecurity providers that want to manage multiple organizations from a central platform.

The core product value is simple:

> **KhemStrix helps organizations understand what is happening on their endpoints, detect suspicious activity, and respond before a security incident becomes more serious.**

---

# 2. Problem Statement

## 2.1 Limited Cybersecurity Visibility

Many small and medium-sized organizations do not have continuous visibility into what happens on their computers and servers.

They may not know:

* Which processes are running
* Which applications are connecting to external networks
* Whether suspicious PowerShell commands are being executed
* Whether repeated login attempts are occurring
* Whether important files are being modified
* Whether a phishing attack has led to malicious activity on an endpoint

Traditional antivirus software may detect known malware, but organizations may still lack visibility into suspicious behavior and attack chains.

For example:

```text
Employee receives phishing email
        ↓
Employee opens malicious document
        ↓
Document launches PowerShell
        ↓
PowerShell downloads additional payload
        ↓
Suspicious network connection
```

Without endpoint monitoring and event correlation, these activities may appear as separate and unrelated events.

---

## 2.2 Cost and Expertise Barrier

Enterprise security products can be expensive and often require:

* Dedicated security personnel
* Security Operations Center capabilities
* Complex deployment
* Continuous monitoring
* Security expertise

Many smaller organizations cannot justify these costs.

KhemStrix aims to reduce this barrier by providing a lightweight and practical security monitoring platform.

---

## 2.3 Limited Localized Security Solutions

Organizations may prefer security solutions that:

* Support local deployment
* Minimize unnecessary data exposure
* Are understandable for non-specialist IT administrators
* Can be managed by local IT or cybersecurity providers
* Provide actionable alerts rather than large volumes of raw logs

KhemStrix aims to focus on these requirements.

---

# 3. Solution

KhemStrix consists of four main layers:

```text
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
```

The development approach will start with reliable telemetry and simple detection rules before introducing more advanced AI models.

The principle is:

> **First collect good security data. Then detect suspicious behavior. Then improve detection using AI and machine learning.**

---

# 4. Deployment Models

## Model 1 — Private On-Premise Deployment

KhemStrix can be deployed inside an organization's own network.

```text
Endpoint Agent
      │
      ▼
Local KhemStrix Server
      │
      ├── FastAPI Backend
      ├── PostgreSQL
      └── Security Dashboard
```

Security events remain within the organization's infrastructure.

This model may be suitable for:

* Clinics
* Accounting firms
* Educational institutions
* Organizations handling sensitive information
* Businesses with strict privacy requirements

---

## Model 2 — Centralized Managed Platform

A central KhemStrix platform can manage multiple organizations.

```text
Organization A Agents ──┐
Organization B Agents ──┼──► KhemStrix Platform
Organization C Agents ──┘
                              │
                              ▼
                       Multi-Tenant System
                              │
                              ▼
                     Security Management
```

This model could eventually allow:

* IT service providers
* Managed Security Service Providers
* Cybersecurity companies

to provide managed cybersecurity monitoring to multiple customers.

---

# 5. Development Roadmap

The project will follow a revised development roadmap.

Instead of immediately building email, Telegram, and AI features, KhemStrix will first establish a strong endpoint monitoring foundation.

```text
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
```

---

# 6. Phase 1 — Agent Registration and Management

## Status: ✅ Completed / Current Foundation

The first phase establishes communication between the KhemStrix endpoint agent and the backend platform.

### Features

* Endpoint agent registration
* Agent authentication
* Agent heartbeat
* IP address synchronization
* MAC address synchronization
* Agent status monitoring
* Agent activation and revocation
* Endpoint management
* Dashboard visibility

### Architecture

```text
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
```

This phase provides the foundation for future security telemetry.

---

# 7. Phase 2 — Endpoint Core Telemetry

## Status: 🚧 Next Development Phase

This is the current priority.

The KhemStrix agent will begin collecting security-relevant events from the endpoint.

---

## 7.1 Process Monitoring

The agent will monitor process creation and execution.

Collected information may include:

* Process name
* Process ID
* Parent Process ID
* Parent process
* Executable path
* Command line
* Username
* Timestamp

Example:

```json
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
```

Example suspicious behavior:

```text
winword.exe
      ↓
powershell.exe
      ↓
external network connection
```

This event chain may indicate suspicious activity and can later be analyzed by both rule-based and AI detection.

---

## 7.2 File Activity Monitoring

The agent will monitor important directories for file activity.

Events may include:

* File created
* File modified
* File deleted
* File renamed
* File extension changed

Initial focus may include:

### Windows

```text
Downloads
Desktop
Important system directories
```

### Linux

```text
/etc
/tmp
Selected user directories
```

The goal is initially to collect reliable telemetry rather than immediately classify files as malicious.

---

## 7.3 Authentication Monitoring

The system will collect authentication-related events such as:

* Successful login
* Failed login
* Logout
* Remote login
* SSH sessions
* RDP sessions
* Privilege-related events

Example:

```text
10 failed logins
        ↓
Successful login
        ↓
Suspicious process execution
```

Later phases can correlate these events into an attack timeline.

---

## 7.4 Network Monitoring

The agent will collect network connection information.

Example fields:

```text
Process
Local IP
Local Port
Remote IP
Remote Port
Protocol
Timestamp
```

Example:

```text
powershell.exe
      ↓
192.168.1.20:49152
      ↓
Suspicious-Remote-IP:443
```

---

# 8. Event Pipeline

Security collectors should not directly communicate with the database individually.

Instead:

```text
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
```

The agent should also support temporary buffering if the server is unavailable.

```text
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
```

---

# 9. Phase 3 — Rule-Based Detection

Before building complex AI models, KhemStrix will implement understandable security rules.

Examples:

### Rule 1 — Suspicious Parent-Child Process

```text
WINWORD.EXE
       ↓
POWERSHELL.EXE
```

Result:

```text
Alert: Suspicious Process Execution
Severity: High
```

---

### Rule 2 — Brute Force Attempt

```text
Multiple failed logins
within a defined time period
```

Result:

```text
Alert: Possible Brute Force Attempt
Severity: Medium / High
```

---

### Rule 3 — Suspicious File Activity

```text
Executable created in Downloads
        ↓
Immediately executed
```

Result:

```text
Alert: Suspicious File Execution
```

---

### Rule 4 — Suspicious Network Behavior

```text
Unusual process
        ↓
External connection
```

Result:

```text
Alert: Suspicious Network Connection
```

---

# 10. Phase 4 — Email and Phishing Telemetry

Once endpoint telemetry is working, KhemStrix can begin monitoring phishing-related events.

Possible information:

* Sender domain
* SPF status
* DKIM status
* DMARC status
* Suspicious URLs
* Domain lookalikes
* Attachment hashes
* Email risk indicators

Example attack chain:

```text
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
```

The objective is not only to detect individual events, but eventually to reconstruct the entire attack chain.

---

# 11. Phase 5 — AI, Machine Learning and MITRE ATT&CK

AI and ML will be introduced after sufficient telemetry and datasets are available.

The AI/ML system may include:

## Phishing Classification

Example:

```text
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
```

Output:

```text
Classification: Suspicious
Risk Score: 94%

Indicators:
- Urgency manipulation
- Brand impersonation
- Suspicious domain
- Credential request
```

---

## Endpoint Behavioral Analysis

The system can analyze events such as:

* Parent-child process relationships
* Command-line behavior
* Process frequency
* Login patterns
* Network connections
* File activity

Example:

```text
Normal Behavior
       │
       ▼
Behavioral Analysis
       │
       ▼
Suspicious / High Risk
```

---

## Risk Scoring

KhemStrix may combine multiple detection signals.

```text
Rule Detection ───┐
                  │
AI Detection ─────┼──► Risk Scoring
                  │
Threat Intelligence┘
                        │
                        ▼
                      Alert
```

Example:

```text
Rule Score:        70
AI Score:          85
Network Risk:      60

Final Risk Score:  High
```

---

## MITRE ATT&CK Mapping

Detected activity can be mapped to relevant MITRE ATT&CK techniques.

Example:

```text
Suspicious PowerShell Execution
        ↓
MITRE ATT&CK
        ↓
T1059
Command and Scripting Interpreter
```

This helps cybersecurity analysts understand the behavior in a standardized format.

---

# 12. Phase 6 — Telegram and Additional Security Telemetry

Telegram may later become a Cambodia-specific research and detection feature.

Possible research areas include:

* Monitoring downloaded files
* Hashing suspicious downloaded files
* Detecting suspicious links
* Detecting lookalike domains
* Monitoring potentially dangerous file types

The objective is not to unnecessarily collect private conversations.

The focus should be on security-relevant metadata and locally observable security events.

---

# 13. Phase 7 — Incident Reporting and Investigation

Once events and detections are available, KhemStrix will generate incident timelines.

Example:

```text
09:10 — Phishing email received
09:12 — Attachment downloaded
09:13 — File executed
09:13 — PowerShell started
09:14 — External connection detected
09:15 — Alert generated
```

The dashboard can provide:

* Incident timeline
* Affected endpoint
* Related events
* Severity
* MITRE ATT&CK techniques
* Recommended mitigation

The system may also generate security reports for organizations.

---

# 14. Phase 8 — Active Response and Containment

This phase will be developed only after detection accuracy and reliability have been validated.

Possible response actions:

* Terminate suspicious processes
* Quarantine suspicious files
* Isolate an endpoint
* Block suspicious network connections

Example:

```text
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
```

Human confirmation should be prioritized before high-impact automated actions.

---

# 15. Team Structure

## 15.1 Rong Sovannorth

### Role: Lead Developer and Cybersecurity Engineer

### Main Responsibilities

* Overall system architecture
* Go endpoint agent development
* Backend development
* API development
* Database architecture
* Agent communication
* Event pipeline
* Infrastructure
* Dashboard integration
* Security feature implementation

### Current Main Focus

```text
Phase 2
Endpoint Core Telemetry
```

Primary work:

```text
Go Agent
    ↓
Process Monitoring
    ↓
Event Schema
    ↓
Event Queue / Buffer
    ↓
FastAPI
    ↓
PostgreSQL
    ↓
Dashboard
```

---

# 15.2 AI and ML Member

### Recommended Role:

# AI/ML Engineer and Detection Intelligence Researcher

This role should not simply be:

> "Train an AI model."

The responsibility should be to build the intelligence layer that helps KhemStrix identify suspicious behavior.

### Main Responsibilities

#### 1. Dataset Research and Preparation

Research and prepare datasets for:

* Phishing detection
* Malicious URLs
* Suspicious command lines
* Process behavior
* Authentication anomalies
* Network behavior

---

#### 2. Feature Engineering

Define what information from KhemStrix events can be used for machine learning.

Example:

```text
Process Name
Parent Process
Command Line
Execution Frequency
Network Destination
Port
Time
User
```

The AI/ML member will help determine which features are useful for identifying suspicious behavior.

---

#### 3. Phishing Detection Model

Develop an initial phishing classification proof of concept.

Possible starting approaches:

```text
TF-IDF
+
Logistic Regression
```

or other lightweight models.

The initial goal is to build a working and explainable model before experimenting with more complex deep learning approaches.

---

#### 4. Endpoint Behavioral Detection

Once the agent produces telemetry, research models for detecting anomalous behavior.

Possible models:

```text
Random Forest
LightGBM
Isolation Forest
Other suitable anomaly detection methods
```

The selected approach should be evaluated based on:

* Accuracy
* False positives
* Performance
* Explainability
* Ability to integrate with KhemStrix

---

#### 5. Detection API Integration

The AI model should not remain as a separate notebook or demonstration.

It should eventually integrate with:

```text
KhemStrix Events
       │
       ▼
AI / ML Detection Service
       │
       ▼
Risk Score
       │
       ▼
KhemStrix Alert
```

---

#### 6. MITRE ATT&CK Intelligence

Research how detected behavior can be mapped to relevant MITRE ATT&CK techniques.

---

# 15.3 Cybersecurity Research Member

### Recommended Role:

# Cybersecurity Research and Detection Engineer

This member's role is extremely important.

The AI/ML member determines:

> **Can machine learning identify suspicious behavior?**

The cybersecurity research member determines:

> **What behavior should actually be considered suspicious or malicious?**

### Main Responsibilities

#### 1. Threat Research

Research real-world attack techniques relevant to KhemStrix.

Examples:

* Phishing
* PowerShell abuse
* Living-off-the-Land Binaries
* Credential attacks
* Brute force
* Malicious file execution
* Persistence techniques
* Command and Control behavior

---

#### 2. Detection Rule Development

Create and document security detection rules.

Example:

```text
Rule Name:
Office Application Spawning PowerShell

Condition:
winword.exe → powershell.exe

Risk:
High

MITRE:
Relevant MITRE ATT&CK technique

Reason:
Office applications spawning scripting engines may indicate malicious document execution.
```

These rules can later be implemented in the KhemStrix detection engine.

---

#### 3. Attack Simulation and Testing

Test whether KhemStrix can actually detect suspicious behavior.

For example:

```text
Attack Simulation
        ↓
Generate Security Event
        ↓
Agent Collects Event
        ↓
Backend Receives Event
        ↓
Detection Rule
        ↓
Alert
```

The research member should help verify:

* Did the agent collect the correct data?
* Did the detection rule trigger?
* Was the alert useful?
* Were there false positives?

---

#### 4. MITRE ATT&CK Research

Research the relationship between:

```text
Observed Behavior
        ↓
Attack Technique
        ↓
MITRE ATT&CK Technique
```

This research can support both rule-based and AI-based detection.

---

#### 5. Threat Scenario Development

Create realistic scenarios for testing KhemStrix.

Example:

```text
Scenario 1:
Phishing Email
        ↓
Malicious File
        ↓
PowerShell
        ↓
External Connection
```

Another example:

```text
Scenario 2:
Repeated Failed Login
        ↓
Successful Login
        ↓
Privilege Escalation
```

These scenarios become the test cases for the entire platform.

---

# 16. Team Collaboration

The three roles should work together like this:

```text
CYBERSECURITY RESEARCH
        │
        │ Defines threats, attack behavior,
        │ detection logic and test scenarios
        ▼
┌───────────────────────────────┐
│       KhemStrix Platform      │
│                               │
│ Go Agent                      │
│ Backend                       │
│ Event Pipeline                │
│ Database                      │
└───────────────┬───────────────┘
                │
                ▼
         Security Events
                │
        ┌───────┴────────┐
        ▼                ▼
Rule-Based Engine     AI / ML Engine
        │                │
        └───────┬────────┘
                ▼
             Risk Score
                │
                ▼
              Alert
                │
                ▼
            Dashboard
```

---

# 17. Current Work Assignment

## Lead Developer / Cybersecurity Engineer

### Current Task

Build:

```text
Windows Process Monitoring
        ↓
Standard Event Schema
        ↓
Event Queue
        ↓
FastAPI Event API
        ↓
PostgreSQL Storage
        ↓
Dashboard Event View
```

---

## Cybersecurity Research and Detection Engineer

### Current Task

Research and document the first detection rules.

Initial priority:

1. Office application spawning PowerShell
2. Encoded PowerShell execution
3. Suspicious command-line execution
4. Repeated failed login attempts
5. Suspicious process-network behavior

Each rule should include:

```text
Rule Name
Description
Detection Condition
Required Telemetry
Severity
Possible False Positives
MITRE ATT&CK Mapping
Test Scenario
```

---

## AI/ML Engineer and Detection Intelligence Researcher

### Current Task

Research and design the KhemStrix Detection Intelligence module.

Initial deliverables:

1. Identify available public datasets.
2. Define potential ML features.
3. Research phishing classification.
4. Research endpoint anomaly detection.
5. Compare suitable models.
6. Build a small proof of concept.
7. Define the model input/output format.
8. Prepare an integration design for the FastAPI backend.

The AI/ML member should start with research and a small proof of concept while the core platform generates telemetry.

---

# 18. Immediate Goal

The immediate objective is not to build every feature.

The first major milestone should be:

> **KhemStrix can collect real endpoint events and successfully detect suspicious behavior.**

The first complete demonstration should look like:

```text
Suspicious Activity Happens
        ↓
KhemStrix Agent Detects It
        ↓
Event Sent to Backend
        ↓
Event Stored
        ↓
Detection Rule Evaluates Event
        ↓
Threat Alert Generated
        ↓
Alert Appears on Dashboard
```

Once this works reliably, KhemStrix will have its first real core EDR capability.

After that, the team can progressively add:

```text
More Telemetry
        ↓
More Detection Rules
        ↓
Email Correlation
        ↓
AI / ML Detection
        ↓
MITRE ATT&CK
        ↓
Incident Investigation
        ↓
Response and Containment
```

---

# 19. Long-Term Vision

The long-term goal of KhemStrix is to become a practical cybersecurity detection platform that can support organizations with limited cybersecurity resources.

The strategy is:

```text
Start Small
    ↓
Build Reliable Endpoint Monitoring
    ↓
Validate Detection
    ↓
Test With Real Users / Organizations
    ↓
Improve Based on Real Telemetry
    ↓
Add AI and Advanced Detection
    ↓
Develop Managed Security Capabilities
```

The project should prioritize **real detection capability and customer value** over adding AI features only for marketing purposes.

The long-term principle of KhemStrix is:

> **Collect meaningful security telemetry. Detect real threats. Explain what happened. Help organizations respond.**
