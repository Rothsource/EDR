# KhemStrix EDR — Real-World Attack Validation

**Goal:** deploy a fresh Ubuntu agent, attack it (and your existing Kali/Parrot/Windows boxes) from Kali using real attack tools, and confirm every technique lands in Postgres as a correctly decoded event — surviving outages, reconnects, and config changes along the way.

**Duration:** 7 days, ~5 hrs/day.
**Scope:** your own lab machines only. Every IP below should be on your private test LAN. Do not point any of this at anything you don't own.

---

## 0. Before You Start (30 min, do this first on Day 1)

- [ ] Spin up a fresh Ubuntu VM (22.04 or 24.04), on the same LAN as Kali/Parrot.
- [ ] Install the agent, register it, confirm it shows up in the `agents` table with a fresh `agent_id`.
- [ ] Confirm SSH is running (`systemctl status ssh`) and enable password auth temporarily if it's key-only by default (`/etc/ssh/sshd_config`: `PasswordAuthentication yes`, then `systemctl restart ssh`) — you need failed-password attempts to actually hit journald.
- [ ] Create 2-3 real, throwaway local users on Ubuntu (`sudo adduser testuser1`, etc.) with weak known passwords, purely so failed-login attacks have valid usernames to fail against as well as invalid ones.
- [ ] On Kali, install the tools you don't have yet:
  ```bash
  sudo apt update
  sudo apt install -y hydra medusa crackmapexec nmap seclists john hashcat
  ```
- [ ] Grab a small password wordlist for brute-forcing (don't use rockyou.txt's full 14M lines — it'll run for days). Make a 20-line list of "plausible bad passwords" plus the real password near the bottom, so hydra actually succeeds once, which you'll also want to see logged.
  ```bash
  cat > /tmp/passlist.txt << 'EOF'
  123456
  password
  admin123
  letme in
  qwerty
  welcome1
  <the real throwaway password you set>
  EOF
  ```
- [ ] Open a `psql` session or a saved query you can re-run fast:
  ```sql
  SELECT time, hostname, username, data->>'event' AS event,
         data->>'reason' AS reason, data->>'status' AS status,
         data->>'src_ip' AS src_ip
  FROM events
  WHERE agent_id = '<ubuntu-agent-id>'
  ORDER BY time DESC LIMIT 20;
  ```
- [ ] Have `agent.log` (or journalctl for the agent service) tailing on the target in a second terminal the whole week: `journalctl -u khemstrix-agent -f` — you want to see it react in real time, not just check Postgres after the fact.

---

## Day 1 (5 hrs) — SSH auth, the core case

This is your bread-and-butter detection: `sshd` failures, successes, invalid users, and PAM summaries. You already validated this synthetically on Kali/Parrot — today do it as an actual attack, against a third, fresh box, and watch it end to end.

### 1.1 Baseline manual checks (30 min)
Before automating anything, do these by hand from Kali so you can eyeball exactly what each one produces, one at a time, with 30s gaps so you can match timestamps easily:

```bash
# Valid user, wrong password
ssh testuser1@<ubuntu-ip>   # type wrong password 3x, let it lock you out of the attempt

# Invalid/nonexistent user
ssh nosuchuser@<ubuntu-ip>

# Valid user, correct password (success case)
ssh testuser1@<ubuntu-ip>   # type the real password
```

After each, check the query above. Confirm:
- Wrong password on a real user → `reason: bad_password`, `status: failure`
- Nonexistent user → `reason: unknown_user`
- Correct password → `status: success`
- `src_ip` matches Kali's IP, `src_port` is populated
- A PAM "N more authentication failures" summary line shows up after 3 failed attempts in one SSH session (this is the multi-failure PAM message, distinct from the per-attempt `Failed password` lines)

### 1.2 Automated brute force with Hydra (1 hr)
```bash
hydra -l testuser1 -P /tmp/passlist.txt ssh://<ubuntu-ip> -t 4 -f
```
`-t 4` throttles concurrency (don't hammer it with 16+ threads — you want to see individual events land cleanly, not just verify the outbox doesn't fall over under load; that's a separate test, see 1.4). `-f` stops on first success.

Watch `agent.log` in real time while this runs. Confirm every attempt shows up as a separate event — no silent drops.

### 1.3 Username enumeration (30 min)
```bash
hydra -L /usr/share/seclists/Usernames/top-usernames-shortlist.txt \
      -p wrongpassword123 ssh://<ubuntu-ip> -t 4
```
This exercises the `unknown_user` path at volume — good stress test for whether your username-doesn't-exist classification stays accurate when the usernames are realistic-looking rather than hand-picked.

### 1.4 Higher-concurrency burst, deliberately (1 hr)
Now do one run specifically to test volume/ordering, not just correctness:
```bash
hydra -l testuser1 -P /usr/share/seclists/Passwords/Common-Credentials/10-million-password-list-top-1000.txt \
      ssh://<ubuntu-ip> -t 16 -f
```
This is Section 6 item 6/7's territory (jitter, outbox size) sneaking into today's test even though the formal jitter test (Step 7) is later — useful early signal on whether a single busy agent can keep up. Note anything that looks like dropped or delayed events, but don't block on fixing it today; just log it as a finding.

### 1.5 `su` and local privilege attempts (1 hr)
```bash
# From an SSH session as testuser1 on Ubuntu itself, or scripted via ssh:
ssh testuser1@<ubuntu-ip> "su - root"    # will prompt, fail on wrong password
```
Also test `su` failures directly at the console/VM if you have console access — some `su` logging differs slightly (PAM stack) between an interactive TTY session and one driven over SSH. Confirm `service: su` (or however your source map labels it) shows up distinctly from `sshd` and `sudo`.

### 1.6 End of day: write down gaps
For each technique above, note: did it log? Correct `reason`? Correct `status`? Any duplicate `event_id`s? Anything that silently didn't appear at all? This becomes your Day 7 bug list.

---

## Day 2 (5 hrs) — `sudo` abuse + Linux durability under real attack

### 2.1 Sudo failures (1 hr)
```bash
ssh testuser1@<ubuntu-ip>
sudo -l          # trigger a sudo password prompt, get it wrong 2-3 times
```
Also try sudo for a command the user isn't authorized for at all (if you've configured limited sudoers), to see whether "wrong password" and "not permitted" produce distinguishable events or the same `sudo_bad_password` reason — worth knowing which, since a SOC analyst will care about the difference eventually even if your current decoder doesn't split it yet. Note it as a possible decoder enhancement, don't fix it today.

### 2.2 Outage-during-attack test (2 hrs) — the actual point of today
This is Test 2/3 from your original report, but for the first time under a *real, messy, uneven* attack pattern instead of a clean synthetic push loop.

1. Start a moderate hydra run against Ubuntu (`-t 4`, the medium wordlist).
2. Mid-run, kill the KhemStrix server process.
3. Let hydra keep running for 2-3 minutes while the server is down.
4. Bring the server back up.
5. Confirm: every attempt made *during* the outage eventually lands in Postgres with zero duplicates and zero loss, once reconciliation completes.
6. Repeat once more, but this time kill the *agent* (not the server) mid-attack, then restart it. Confirm the SQLite outbox on disk had queued the in-flight events and they still land after agent restart. This specifically exercises Section 6 item 7's "no outbox size limit" concern — watch outbox file size before/after.

### 2.3 Clock skew sanity check (30 min)
Compare `date` output on Kali, Ubuntu, and your server VM. If they're not already NTP-synced, this is a good moment to either fix it or deliberately leave a few seconds of skew and confirm `created_at` vs `time` in Postgres stays sane (Section 6 item 17) — note which you did.

### 2.4 Two-agents-attacking-in-parallel test (1.5 hrs)
Run a Kali attack against Ubuntu **and** a separate attack against Parrot at the same time (two terminals, two hydra runs). Confirm `agent_id`/`tenant_id` isolation holds under real concurrent load, not just your earlier two-synthetic-agent test.

---

## Day 3 (5 hrs) — Windows auth, real attack surface, all protocols

Your Windows collector reads all 25 `windows-*` sources, not just 4625/RDP — so today's job is to actually generate traffic across as many distinct **logon types and protocols** as your standalone (non-domain) box can produce, not just RDP. A single-machine Windows 10 box can't exercise Kerberos (needs a domain controller — see the stretch goal at the end), but it can exercise almost everything else.

### 3.0 Coverage matrix — what to attack to hit each LogonType/protocol

Work through this table across the day. Each row is a distinct code path in the Windows auth stack, so each one is worth confirming separately rather than assuming "RDP worked, so it all works":

| LogonType | What it represents | How to trigger it from Kali (attack) or locally | Expect |
|---|---|---|---|
| 2 — Interactive | Console/keyboard logon | Only possible with physical/console access — do this manually at the VM console, wrong password 3x, then correct | `4625` then `4624`, `LogonType: 2` |
| 3 — Network | SMB/file-share auth, most credential-spray tools | `crackmapexec smb`, or `smbclient //<ip>/C$ -U testuser3` with wrong/right passwords | `4625`/`4776`, `LogonType: 3` |
| 3 — Network (WinRM) | PowerShell Remoting / WinRM | See 3.3 below — `evil-winrm` | `4625`/`4624`, `LogonType: 3`, often via NTLM not Kerberos on a workgroup box |
| 4 — Batch | Scheduled task running as a user | Create a scheduled task with bad stored creds (`schtasks /create ... /ru testuser3 /rp wrongpass`), let it fire | `4625`, `LogonType: 4` |
| 5 — Service | A Windows service starting under a specific account | Configure a service to log on as `testuser3` with the wrong password via `services.msc`, try to start it | `4625`, `LogonType: 5` |
| 7 — Unlock | Unlocking a locked workstation | At the console: lock the screen (`Win+L`), enter wrong password 2-3x, then correct | `4625`/`4624`, `LogonType: 7` |
| 8 — NetworkCleartext | Basic auth over HTTP (cleartext creds sent) | If IIS is installed with Basic Auth enabled on a test site, hit it with `curl -u testuser3:wrongpass` | `4625`, `LogonType: 8` (optional — skip if IIS isn't already set up, not worth installing just for this) |
| 9 — NewCredentials | `runas /netonly` — used by attackers to stage credentials without full logon | Locally: `runas /netonly /user:testuser3 cmd` with the wrong password | May not always show as a failure the same way — note what you actually see, this is a known attacker technique (credential staging) worth understanding regardless |
| 10 — RemoteInteractive | RDP | Day 3.2 below (hydra) | `4625`/`4624`, `LogonType: 10` |
| 11 — CachedInteractive | Logon using cached domain creds, no DC reachable | Only relevant if the box was ever domain-joined and is now offline from the DC — likely N/A on a workgroup box, skip unless you set up the AD stretch goal |

Also distinct from LogonType but worth hitting separately:
- **NTLM explicit credential use (4648)** — happens automatically when you `runas /user:otheruser` or connect to a share with explicit alternate creds (`net use \\<ip>\C$ /user:testuser3 wrongpass`)
- **Account lockout (4740)** — only fires if Account Lockout Policy is enabled (Day 3.4) — confirm this ID is actually in your source list, since your last status report only explicitly mentioned 25 IDs without listing them; this is a good day to `cat` the actual list and check
- **PsExec-style remote execution (SMB + Service, chained)** — see 3.5 below, a realistic attacker chain that touches LogonType 3 *and* 5 in one technique

### 3.1 Enable RDP on the Windows VM if not already (15 min)
Settings → System → Remote Desktop → on. Confirm Windows Firewall allows it from Kali's subnet.

### 3.2 RDP brute force with Hydra (1.5 hrs)
```bash
hydra -l testuser3 -P /tmp/passlist.txt rdp://<windows-ip> -t 2
```
RDP is slower/more fragile to brute-force than SSH — keep `-t` low (1-2) or it'll just start refusing connections rather than actually attempting logon. Confirm each attempt produces a `4625` event with a sensible `SubStatus`.

### 3.3 SMB/NTLM attacks with CrowdMapExec-style tooling (1.5 hrs)
```bash
crackmapexec smb <windows-ip> -u testuser3 -p /tmp/passlist.txt
```
This is a genuinely realistic attacker technique (SMB credential spraying) and should generate `4625`/`4776`/`4648`-style events depending on auth path — this is good coverage since your Day-1-equivalent Windows data so far came from manual `net user` commands rather than a real network-based credential attack tool.

### 3.4 WinRM / PowerShell Remoting attack (45 min)
WinRM is a realistic lateral-movement path (attackers love it because it's "living off the land," not a dropped tool) and it's a different code path from SMB even though it also reports as `LogonType 3`.

```bash
# On Kali, if not already installed:
sudo gem install evil-winrm   # or: pip install pywinrm for a scripted version

# Enable WinRM on the Windows box first (as admin, locally):
#   winrm quickconfig -q

evil-winrm -i <windows-ip> -u testuser3 -p wrongpassword
evil-winrm -i <windows-ip> -u testuser3 -p <realpassword>   # confirm success case too
```
Confirm this produces its own distinct sequence of events rather than silently reusing whatever SMB already logged — WinRM auth happens before any shell session starts, so a failed WinRM logon should log even if you never get a working shell.

### 3.5 PsExec-style remote execution chain (45 min)
This is one of the single most common real-world lateral-movement techniques, and it's a good multi-protocol test because it touches SMB (LogonType 3) to copy a service binary, then Service logon (LogonType 5) to execute it.

```bash
# From Kali, using impacket (install if needed: pip install impacket)
psexec.py testuster3:wrongpassword@<windows-ip>
psexec.py testuser3:<realpassword>@<windows-ip>     # confirm success case
```
Confirm you get a coherent, ordered event pair — SMB auth first, then a service-related logon — rather than just one flattened event. This is a good one to specifically check against your Day 4 "does the EDR tell a coherent story" goal, a day early.

### 3.6 Local lockout test (1 hr)
If you have account lockout policy enabled (or enable it: `secpol.msc` → Account Lockout Policy), deliberately trigger a lockout on `testuser3` via repeated RDP or SMB failures. Confirm:
- The failure events leading up to lockout all log correctly
- Windows generates a distinct lockout event (Event ID 4740) — check whether your current 25-ID source list even includes 4740; if not, that's a real, concrete gap to add before you call Windows coverage "done" (compare against your ID list from the last update).

### 3.7 End of day gap list
Same as Day 1 — write down anything Windows-specific that didn't behave as expected, especially around 4740 coverage and the LogonType breakdown from the matrix above. You now have a per-protocol pass/fail table, not just "Windows works" — that's the real deliverable from today.

### Stretch goal (optional, only if time allows later in the week): Kerberos
Everything above is NTLM because the box is standalone/workgroup — Kerberos only exists once there's a domain. If you want real Kerberos coverage (4768 TGT request, 4769 service ticket, 4771 pre-auth failure — genuinely different event IDs from anything above), you'd need to stand up a cheap AD domain controller VM and join the Windows box to it. That's a meaningful chunk of extra setup, so treat it as a "Day 8 if you have one" item rather than squeezing it into this week — flag it in your Day 7 report as explicitly out of scope for now rather than silently untested.

---

## Day 4 (5 hrs) — Account manipulation + config-change-under-attack

### 4.1 Real account manipulation attack chain (2 hrs)
Simulate what a real intrusion looks like *after* a successful compromise, not just failed logins:
```bash
# From an already-compromised session (simulate via RDP or PsExec-equivalent with valid creds)
net user attacker_backdoor Passw0rd123! /add
net localgroup administrators attacker_backdoor /add
net user testuser3 /active:no          # disable a real account (denial)
net user attacker_backdoor /delete     # cover tracks
```
On Linux equivalent:
```bash
sudo useradd -m backdoor
echo "backdoor:Passw0rd123!" | sudo chpasswd
sudo usermod -aG sudo backdoor
sudo userdel -r backdoor
```
Confirm the full chain (create → privilege-escalate → delete) lands as a sequence of correctly ordered, correctly typed events. This is the first time you're testing whether your EDR tells a coherent *story*, not just individual facts.

### 4.2 Config change mid-attack (2 hrs) — combining Test 6 with live attack traffic
This is the test your architecture was specifically built to survive cleanly:

1. Start a sustained hydra run against Ubuntu.
2. While it's running, push a remote config change narrowing Ubuntu's sources (e.g. drop `su` temporarily).
3. Confirm: within one heartbeat, `su`-sourced events stop while `sshd` events from the ongoing attack keep flowing uninterrupted — same PID, same WebSocket connection, only the `journalctl` subprocess restarts.
4. Revert the config, confirm `su` events resume.
5. Do the same test on the Windows agent if its config-apply path is finished by now (per your Sept 24 update, item A.6 — "Test 6 on Windows" was still pending; this is a natural place to close that out).

### 4.3 Wrap-up (1 hr)
Log everything from 4.1/4.2. Specifically flag anything where a config change appeared to drop an event that was in-flight at the exact moment of the subprocess swap — that's the one edge case your design doc flags as needing the saved-cursor resume to work correctly (Section 7B.1 step 5).

---

## Day 5 (5 hrs) — Multi-stage realistic attack scenario

Today, stop testing individual techniques and run one continuous, realistic attack narrative end to end, timed, as if you were demonstrating this to a bank auditor or customer.

### 5.1 Recon phase (30 min)
```bash
nmap -sV -p 22,445,3389 <ubuntu-ip> <windows-ip>
```
(Not itself something your auth-focused EDR will catch yet — network monitoring is still Step 6 in your roadmap — but useful to note as a labeled "not detected, by design, until network module ships" line in your final report.)

### 5.2 Credential spraying phase (1.5 hrs)
Spray a small, realistic username list against both Ubuntu and Windows with a common weak-password list, simulating an opportunistic attacker rather than a targeted brute force:
```bash
hydra -L /tmp/userlist.txt -P /tmp/passlist.txt ssh://<ubuntu-ip> -t 4
crackmapexec smb <windows-ip> -u /tmp/userlist.txt -p /tmp/passlist.txt
```

### 5.3 Successful compromise + persistence (1.5 hrs)
Once one credential succeeds (engineer the wordlist so one does), log in for real and:
- Create a new local admin/sudo account (persistence)
- Add it to relevant privileged groups
- Attempt lateral movement: from the now-compromised Ubuntu box, SSH *into* Parrot or Windows using found/guessed credentials

### 5.4 Cleanup attempt (30 min)
As the "attacker," try to delete the account you created and clear local history — this won't clear your EDR's copy of events (durable outbox already shipped them), which is exactly the point to demonstrate: **the attacker deleting local evidence doesn't delete what the EDR already captured.**

### 5.5 Reconstruct the timeline (1 hr)
Query Postgres for the full sequence across all agents, ordered by `time`, and confirm you can reconstruct the entire attack narrative — recon → spray → compromise → persistence → lateral move → cleanup attempt — purely from what landed in `events`. This is your strongest piece of evidence for both the compliance story (Section 4C from your Sept 24 report) and for your own confidence in the product.

---

## Day 6 (5 hrs) — Scale, jitter, and edge cases

Today pulls forward pieces of your existing backlog (Steps 7-9) that real attack testing naturally stresses.

### 6.1 Jitter/reconnect storm, for real (2 hrs)
If you can spin up several more lightweight Ubuntu agents (containers or fast VMs are fine for this — they don't need to be attack targets, just running agents), get to 10+ concurrent agents. Kill the server, bring it back, confirm reconnects spread out rather than clustering (Section 6 item 6, Step 7). Even 8-10 is far more signal than your current 2.

### 6.2 Duplicate-connection edge case (1 hr)
Deliberately run two instances of the agent binary on the same Ubuntu box pointed at the same `identity.json` (copy it to a second location or just run it twice). Confirm what actually happens — reject, replace, or both connected — and record it as the real answer to the previously-undecided Section 6 item 18.

### 6.3 Outbox size limit stress test (1 hr)
Take Ubuntu offline from the network entirely (not just kill the server — actually disconnect it, e.g. `sudo ip link set eth0 down` if you're on a VM you can recover via console) while running a sustained attack against it from Kali (attacks against an unreachable box won't succeed, but you can still generate local auth activity — e.g. failed local console logins, or just `su`/`sudo` locally). Let it queue for 10-15 minutes, then bring networking back. Confirm outbox behavior at a size you haven't tested before, and note roughly how large the SQLite file got.

### 6.4 Clean up your gap list (1 hr)
By now you likely have 10-20 findings across Days 1-6. Spend this hour just consolidating them into one prioritized list: broken/missing (fix before calling this "tested"), works-but-imprecise (note for later), and working-as-intended.

---

## Day 7 (5 hrs) — Fix, retest, and write it up

### 7.1 Fix the highest-priority findings (2.5 hrs)
Whatever came out of Days 1-6 as an actual bug (something silently dropped, misclassified, or duplicated) — fix those first. Don't try to close every minor gap today.

### 7.2 Retest exactly what you fixed (1 hr)
Re-run the specific technique that exposed each bug, narrowly — you don't need to redo the whole week, just confirm each fix actually holds.

### 7.3 Write the report (1.5 hrs)
Use the same format as your existing status reports (Summary → What we verified → Bugs found and fixed → Still to do → Risks). This one has a genuinely different character from your earlier reports: it's the first one built entirely from real, adversarial traffic rather than synthetic or manually-triggered test events, across three live OSes (Ubuntu added, Windows and Linux both exercised for real) — that's worth stating plainly, since it's a meaningfully stronger claim to make to a customer or auditor than "we tested it with `testpush.exe`."

---

## Running Checklist (copy into your tracker)

- [ ] Ubuntu agent deployed and registered
- [ ] Day 1: SSH manual + hydra brute force + enumeration + burst + su
- [ ] Day 2: sudo failures + outage-during-attack (server kill + agent kill) + clock skew + parallel agents
- [ ] Day 3: RDP brute force + SMB/NTLM spray + lockout + 4740 coverage check
- [ ] Day 4: account manipulation chain + config-change-mid-attack (Linux + Windows)
- [ ] Day 5: full recon→compromise→persist→lateral→cleanup narrative, reconstructed from Postgres
- [ ] Day 6: jitter at 10+ agents + duplicate-connection behavior + outbox stress
- [ ] Day 7: fixes, retest, written report

## Notes / Safety Reminders
- Everything above targets machines you own on your own private lab network.
- Password lists are small and throwaway-focused — don't run multi-day exhaustive brute forces, you're validating detection, not actually cracking anything.
- Keep a rotation habit: any credentials/API keys visible in terminal output or logs during this week should be rotated afterward, same as you've been doing with agent keys.