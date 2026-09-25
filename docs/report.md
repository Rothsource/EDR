# KhemStrix EDR — Roadmap Addendum, September 24, 2026

This extends the original report's **Section 6 (What Has NOT Been Done Yet)** and **Section 7 (What We Need to Do Next — In Order)**. Numbering continues from the original document rather than restarting, so items can be cross-referenced. Sections 1–9 of the original document are unchanged except where noted.

---

## Section 6 — What Has NOT Been Done Yet (continued)

25. ~~No decision on Windows auth collection scope beyond 4625.~~ **Resolved this update — see new Section 7A.1.** Windows auth is now all auth-relevant Security events (25 Event IDs), each independently toggleable, not just 4625.

26. ~~Windows auth collector not wired into the agent process.~~ **Resolved this update.** `StartAuthCollector` on Windows was a no-op stub; `authcollector_windows.go` now starts `ReadSecurityLog` for real. This was the actual state of the project at the start of this session — the "Step 4: Build the Windows auth collector" line item in the original Section 7 had not actually been done, despite `window.go` and `windows_wevtapi.go` existing as files.

27. **New — Windows: successful logons (4624) and privileged-use events (4672) are being silently dropped.** Verified from production `unmatched_raw` data: all 22 dropped Windows records are SYSTEM service-account noise (`LogonType=5`), which is correctly-intentioned filtering, but it's unconfirmed whether a *real user's* 4624/4672 would survive the same decoder rule. This is not a "nice to have" — the NBC guideline (Section 8 below) explicitly requires logon and privilege-use logging, not just failures.

28. **New — Windows: group-membership-change events report a SID instead of a username.** `MemberName` is read instead of `SubjectUserName` (the person who made the change), so `4728`/`4732`/`4756` rows show something like `S-1-5-21-...-1005` where a human-readable actor should be.

29. **New — Windows: some 4625 failures decode with `reason: other`** where a specific substatus should have matched. Suspected cause: hex-case mismatch between what Windows sends (`0xc000006a`) and the reason map's keys (`0xC000006A`). Unconfirmed without seeing the deployed decoder.

30. **New — Windows: 4776 (NTLM validation) `status` field is inconsistent** across otherwise-identical rows (blank in most, `failure` in one). Suggests the deployed decoder's 4776 handling differs from what's been reviewed.

31. **New — `unmatched_raw` has no `agent_id` or hostname column and no retention policy.** It currently holds 194 Linux `sudo` lines, 172 `sshd-session` lines, and 22 Windows records. Some of the dropped `sudo` lines are very likely successful privileged commands (`sudo ... COMMAND=...`), which the NBC guideline also wants logged — not just failures. Right now there is no way to trace a dropped record back to its source machine, and the table has no cap or cleanup job.

32. **New — Windows agent-side error handling was silently swallowing the real failure reason.** `EvtSubscribe`/`EvtCreateBookmark`/`EvtRender` all used `syscall.GetLastError()` after a `.Call()` invocation, which returns `nil` in that context — every failure printed as `%!w(<nil>)`. This is fixed (the real Win32 error is now captured from `.Call()`'s third return value), but it's listed here because it was the single biggest reason Windows collection was undiagnosable for most of this session, and the same pattern should be checked anywhere else `.Call()` is used in the Windows-specific code.

33. **New — Windows Event Log has a 20-expression-per-XPath-query limit**, undocumented anywhere in the original design. `buildXPath` originally emitted one flat expression with all enabled Event IDs OR'd together; with 25 IDs this exceeded the limit and `EvtSubscribe` failed outright with "the specified query is invalid." Fixed by splitting into a structured `<QueryList>` with one `<Select>` per group of ≤20 IDs. This limit will resurface if the Event ID list grows further (e.g. Phase 2/3 IDs, Section 7A below) — the grouping logic already handles that, but it's worth flagging as a real platform constraint, not a one-off bug.

34. **New — Windows had the same class of cursor/push-ordering bug as Linux (item 23), independently.** A failed `Push()` in `handleEvent` did not stop the subscription's bookmark from advancing on the *next* successful event, which would silently and permanently drop the failed one. Fixed by marking the subscription "failed" on a push error, causing it to ignore further callbacks and force a full resubscribe from the last bookmark actually persisted to disk. Worth noting as a pattern: any per-record durable-delivery loop (present or future, any OS) needs this same guard, and it's easy to add without it the first time, as both this and item 23 show.

35. **New — the Windows agent's `EvtSubscribe`/`EvtCreateBookmark`/`EvtRender` errors were invisible in production** because a Windows service has no console and nothing captured `os.Stderr`/`log` output anywhere. Added basic file logging (`%ProgramData%\khemstrix-agent\agent.log`) this session, which is what made items 32–34 diagnosable at all. This log file currently has no rotation or size cap.

36. **New — clock skew between at least one Linux VM (Parrot) and the server is measurably nonzero** (`created_at` observed up to ~0.7s before `time` on some rows — i.e. the agent's clock is ahead of the server's). This was called out as unaddressed in the original report (item 17) but is now confirmed present with real numbers, not just a theoretical risk. The NBC guideline (Section 8) explicitly requires synchronized clocks via a central NTP source, which makes this a compliance item as well as a correctness one.

37. **New — the deprecated query-param fallback on `GET /agent/config` is still live.** Headers now work and are deployed on Kali and Parrot; Windows needs re-confirmation after its recent redeploys. The fallback should not be removed until all three agents are confirmed clean in the server log (no "deprecated" warning).

38. **New — the Windows agent's API key has not been confirmed rotated.** It was pasted in plaintext into chat/logs during earlier debugging in the same way the two `testpush`/`testreconnect` keys were (item 15); it should be treated the same way.

39. **New — a stray agent ID (`b276b301-...`) is generating repeated 401 heartbeats** against the server and does not exist in the `agents` table. Source unidentified — could be a leftover test agent pointed at the wrong server, or a misconfigured `config.json` somewhere. Not urgent (it's just noise in the log) but unexplained.

---

## Section 7 — What We Need to Do Next — In Order (continued)

The original Step 0–10 sequence (auth-first rollout, decoder framework, Linux collector rework, Windows collector, TLS, file/network monitoring, jitter/scale testing, multi-tenancy, hardening, shutdown race) still holds as the overall shape. This session sits inside **Step 4 (Windows collector)**, which turned out to require substantially more work than originally scoped — the original Step 4 description assumed 4625-only via `wevtapi.dll`, correctly identified the mechanism, but underestimated that the collector had never actually been wired into the running agent at all.

### Step 4 (revised): Finish Windows auth collection

4.1. **Diff the deployed `windows_security.py` against the reviewed corrected version.** This is the immediate blocker — items 27–30 above cannot be fixed with confidence until it's clear what's actually running on the server right now.

4.2. **Fix decoder issues found from production data:**
   - Read `SubjectUserName` (not `MemberName`) for the "who" on 4728/4732/4756.
   - Normalize substatus hex-code casing before the `SUBSTATUS_REASONS` lookup (item 29).
   - Confirm and fix 4776's `status` handling (item 30).
   - Confirm real (non-SYSTEM, non-service-account) 4624 and 4672 events are not being caught by whatever filter is dropping the SYSTEM ones (item 27) — this is the one that matters most for the compliance angle.

4.3. **Re-run Test 5's full event set** (successful login, failed login, workstation lock/unlock, account create/enable/disable/delete, group membership change) with a fresh eye on *every* event type reaching `events`, not just some of them.

4.4. **Run Test 6 on Windows — not yet done at all.** Needs the VM running. Narrow the source list via a config push, confirm excluded Event IDs stop arriving, confirm the agent process PID never changes throughout (the whole point of Section 7B.1's design). This is the one piece of Step 4 that hasn't been attempted yet in any form.

4.5. **Decide and implement server-side volume filtering for 4624/4634** (drop `LogonType=5` service logons and `$`-suffixed machine accounts at the decoder/policy layer, per D8 — never in the agent). Not urgent for correctness, but will matter quickly once this runs on a real multi-user machine instead of a lab VM.

### Step 4.5 (new): Linux privileged-activity coverage

Not in the original roadmap at all, but surfaced by production data (item 31): confirm whether successful `sudo` command lines (not just auth failures) are currently reaching `events` or being silently filtered as "routine noise" by `looksAuthRelevant`. The NBC guideline (Section 8) wants "use of privileges" logged generally, not just failed privilege escalation attempts. This may just be a decoder gap (no journald decoder rule currently matches a successful `sudo ... COMMAND=...` line) rather than an agent-side filtering issue — needs the same "what's actually in `unmatched_raw`" investigation as 4.1 above, applied to the `sudo` identifier.

### Step 4.6 (new): `unmatched_raw` cleanup

Before this table grows further unattended:
- Add an `agent_id` column so a dropped record can be traced to its source machine.
- Add a `reason` column distinguishing intentional noise-filtering from genuinely unrecognized records — these need different retention policies and different follow-up actions.
- Add a retention/cleanup job. It currently has no cap and no scheduled deletion.

### Between Step 4 and Step 5 (rollout hygiene, should happen regardless of Windows status)

- Confirm all three agents (Kali, Parrot, Windows) are using the header-based `/agent/config` fetch with no "deprecated" warning in the server log, **then** delete the query-param fallback entirely (item 37).
- Rotate the Windows agent's API key (item 38) and confirm Parrot's rotation actually landed (it was ambiguous mid-session whether the rotation or just a routine reconnect was being observed).
- Add basic size-based rotation to `agent.log` on Windows (item 35) before it's left running unattended for any length of time.
- Investigate the stray `b276b301-...` agent (item 39) — low priority, just needs an explanation.

### Step 5 (TLS) — unchanged from original, but now more urgent

Real Windows Security-log data (usernames, source IPs, domain names) is now flowing in plaintext over `ws://`. This was already flagged as a blocker before "real endpoint data" flows; that data is now flowing. TLS should move up ahead of any further Windows scope work (Phase 2/3 Event IDs, Section 7A.1 below) rather than after it.

### New: Step 4A — Compliance-driven roadmap branch (Cambodia NBC guideline)

Not part of the original technical roadmap, but discovered this session and directly relevant to prioritization: Cambodia's National Bank Technology and Cyber Risk Management Guidelines (effective January 2026) require, among other things, EDR deployment, centralized logon/logoff and privilege-use logging, ≥3-year retention, tamper protection, and synchronized clocks. Concretely, this reorders some backlog priority:

- **Pulled forward:** items 27 (real-user logon/privilege coverage), 36 (clock sync), and Step 5 (TLS) all map directly to explicit guideline requirements.
- **New backlog items, not previously anywhere in the roadmap:**
  - Log retention ≥ 3 years with no user-facing delete path on `events`.
  - Tamper-evidence on `events` (append-only enforcement, or a hash chain) — currently nothing prevents modification of a stored row.
  - A short report template mapping specific collected fields to specific guideline sections, for use with prospective customers.
- **Deferred, not urgent for compliance:** file/network monitoring (original Step 6) — nothing found in the guideline research specifically requires it ahead of what's already planned.
- **Suggested validation step, not code:** talk to 3–5 people at banks or microfinance institutions — confirm the guideline's actual scope (does it reach small MFIs, or only larger banks?) and ask directly how they currently produce access-log evidence for an auditor. This determines whether Step 4A is worth continued investment or was a one-off research tangent.

---

## Section 7A.1 (new) — Revised Windows Event ID scope, for reference

Supersedes the original Step 4's "Event ID 4625 only" framing. Current full list, implemented in `windowsEventIDs`:

| Category | Event IDs |
|---|---|
| Logon / logoff | 4624, 4625, 4634, 4647, 4648, 4672, 4778, 4779, 4800, 4801 |
| Credential validation / Kerberos | 4776, 4768, 4769, 4771 |
| Account lifecycle | 4720, 4722, 4723, 4724, 4725, 4726, 4740, 4767 |
| Privileged group changes | 4728, 4732, 4756 |

Kerberos IDs (4768/4769/4771) will only ever fire on a domain controller — silence for these on a workstation is expected, not a bug. Each ID is independently toggleable per D8 (source selection only, never outcome filtering) — the same mechanism already used for `sshd`/`sudo`/`su` on Linux, just with more entries.