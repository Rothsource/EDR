# KhemStrix EDR — WebSocket Durable Delivery: Full Status Report

Originally September 15, 2026 — updated September 17 with multi-agent and larger-backlog testing — updated September 18 to confirm real-agent integration, record the new event fields added to Postgres, and lay out the collector roadmap (auth → file → network) — updated September 21 to record the code that carries those new fields through the whole stack — updated again September 21 (later) to record that this code has now been VERIFIED against the live server and database with two agents (Windows + Linux), including an outage and reconnect — updated September 22 to record a real (but not yet complete) Linux auth collector, an architecture decision to move log parsing off the agent and onto the server, and a design for remote, server-side agent configuration — updated September 22 (later) to lock in the auth-first rollout scope, settle the Windows auth collection method (native Security Event Log, not Sysmon, for auth specifically), decide that remote auth config controls source selection only — not outcome/content filtering — and fully specify the agent-side mechanism that applies a pushed config change without ever restarting the agent process — **updated September 24 with execution status: Steps 1–3 done and verified (Linux Test 6 passed on Kali + Parrot); Step 4 built but currently broken (EvtSubscribe bug, fix written not deployed) and scope expanded from D1's 4625-only to all 25 windows-* IDs; two new workstreams added outside the original plan (NBC compliance research, real-world attack testing week); Steps 5, 7, 8, 9, 10 not yet started**

---

## 1. What Is This Project, Briefly

KhemStrix EDR is an Endpoint Detection & Response (EDR) system for small-to-medium businesses in Cambodia. An "agent" (a small program) runs on each employee's computer, watches for suspicious activity (phishing emails, malware, brute-force login attempts), and reports what it sees back to a central server, where it's stored and analyzed.

This report covers one specific piece of that system: how events get from the agent to the server reliably, even when the network connection is unstable — plus the auth-monitoring rollout built on top of it.

## 2–4. WebSockets, the Problem, the Solution

*(Unchanged — see prior revisions. Summary: WebSocket replaces slow REST batching with an instant, persistent connection; a local SQLite outbox on the agent guarantees no event is lost on disconnect; reconciliation on reconnect, idempotent server-side inserts, heartbeat-based drop detection, and jittered reconnects round out the durability guarantee.)*

## 5. What We Have Actually Done and Verified

*(Unchanged through 5.8 — see prior revisions. Server and agent transport fully built and verified; five new event fields wired end-to-end; real Linux auth collector built on the since-superseded agent-parses design.)*

## 6. What Has NOT Been Done Yet

*(Unchanged from Sept 22 (later) — items 1–24 as previously listed. Status as of Sept 24: items 1–3 and 20 resolved as noted; item 23's cursor-advance bug fixed as part of the Step 3 rework; all other items still open.)*

## 7. What We Need to Do Next — In Order

Steps 1–3: ✅ done and verified (Step 3 passed Test 6 on Kali + Parrot Linux).

**Step 4 — status as of Sept 24: built, currently broken.** Collector implemented, but subscription setup fails (`EvtSubscribe: query is invalid`); a fix has been written but not yet deployed. Scope also expanded during implementation from D1's original "4625 only" to all 25 `windows-*` event IDs — this is a real decision reversal, not just added coverage, and D1 needs to be formally revised in Section 7A once the collector is confirmed working. Decoder registration name vs. the agent's actual `data.source` value is still unverified (currently assumed, not confirmed).

Steps 5–10: not started (Step 5/TLS, Step 7/jitter-at-scale, Step 8/multi-tenant, Step 9/hardening, Step 10/shutdown race).

Two workstreams outside the original 10 steps have also started: NBC (National Bank of Cambodia) EDR compliance research (TLS, retention, tamper-evidence, alerting, customer interviews), and a real-world attack testing week (Ubuntu deployment + Kali-driven attacks), layered on top once Step 3 was verified.

### Immediate next steps, in order

1. Fix and deploy the Windows `EvtSubscribe` bug; confirm `auth_bookmark.xml` is being written.
2. Confirm the `windows-security` decoder's registered name actually matches the agent's `data.source` (currently unverified).
3. Run Test 5 and Test 6 on Windows — this is what actually closes out Step 4.
4. TLS (Step 5) — no real endpoint data should cross the wire in plaintext before this lands.
5. Run the Ubuntu + real-attack test week — treat this as the practical, adversarial-traffic version of "prove it end to end" that Step 6 originally called for with manual triggers.
6. Steps 7–10 (jitter at scale, multi-tenant isolation, durability hardening, shutdown race) — still last, still untouched.
7. NBC compliance workstream can run in parallel once TLS is done — it doesn't block engineering work.

## 7A. Architecture Decision: Parsing Moves From Agent to Server

*(Unchanged — agent ships raw records, server decodes. D1–D8 as previously locked in, with one correction:)*

**D1 — revise, in progress.** Originally locked as "Event ID 4625 only." Actual Windows implementation covers all 25 `windows-*` IDs, discovered once the collector was actually run. This table entry needs to be formally updated once Step 4 is unblocked and confirmed working end to end — carrying it forward as "4625 only" in the decision log would now be inaccurate.

## 7B. Remote Agent Configuration

*(Unchanged — design fully specified, not yet built; see prior revision for full detail.)*

## 8. How To Test This Yourself

*(Unchanged — Tests 1–4 previously verified on the transport layer; Test 6 now also passed on Linux (Kali + Parrot) as part of Step 3's close-out; Test 5/6 on Windows still pending Step 4's fix.)*

## 9. One-Sentence Summary (through Sept 22)

*(Unchanged — see prior revision.)*

## 10. Sept 24 Update — Plan vs. Actual, and What Bit Us

| Step | Planned | Actual, as of Sept 24 |
|---|---|---|
| 1 (5 fields) | Build | ✅ confirmed |
| 2 (decoder framework) | Build | ✅ live; found and fixed a `decode_event` bug (result never awaited) |
| 3 (Linux: raw-ship + cursor fix) | Build | ✅ done; Test 6 passed on Kali + Parrot |
| 4 (Windows: 4625-only) | Build | ⚠️ built, scope grew to all 25 `windows-*` IDs, currently broken (`EvtSubscribe: query is invalid`), fix written not deployed |
| 5 (TLS) | After/alongside 3–4 | ❌ not started |
| 6 (file/network) | After auth proven | correctly not started |
| 7 (jitter at scale) | 10–50+ agents | ❌ still 2 agents only |
| 8 (multi-tenant) | Test isolation | ❌ not started |
| 9 (hardening) | Outbox limits, dead-letter, etc. | partial — `agent_id` NOT NULL done, rest not started |
| 10 (shutdown race) | Low priority | ❌ untouched |

**Real scope/decision changes, not just progress:**
- **D1 reversed.** Locked as 4625-only in Section 7A; shipped as all 25 `windows-*` IDs. Flagged for a formal doc revision, not silently carried forward.
- **New workstream: NBC compliance research.** Found the regulatory hook and added a compliance section (TLS, retention, tamper-evidence, alerting, customer interviews) with no equivalent in the original plan.
- **New workstream: real-world attack testing week.** Ubuntu deployment + Kali-driven attacks, layered on after Step 3's verification — functionally a superset of Step 6's original "prove it end to end" goal, done adversarially rather than manually.
- **Bugs the plan couldn't have anticipated**, since they only surfaced once code actually ran: a JSON vs. JSONB type mismatch, a circular import in decoder registration, an empty Windows `AuthConfigPath`, and `%ProgramData%` never being expanded.

**Bottom line:** Step 4 is the one thing blocking forward progress; everything from Step 5 on is genuinely untouched; two workstreams not in the original 10 steps are now running alongside it.