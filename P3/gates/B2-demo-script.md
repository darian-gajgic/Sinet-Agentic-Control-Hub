# B2 gate — walking-skeleton live demo script

**This script IS the B2 gate** (P3/STATE: "its gate is the live demo on this machine"): intake → plan → execute → verify → checkpoint → receipt, live, dev-mode, on this machine. Follow it top to bottom with the coordinator session open; every step states its expected result. Paid steps are marked **[PAID]** — total expected spend is roughly **$0.05–$0.30** on the dev default model (haiku class), all on the subscription lane.

**Honest posture (what this demo is and is not):**
- Dev-mode process, loopback only. **No host installs** — those are gate decisions, not prerequisites.
- Engine sessions spawn **unconfined** (the sanctioned B1-1 dev posture; the control log warns loudly). Confined+authenticated spawns need the S11.5 credential-injection proxy — part of the host batch this gate decides.
- The demo task is **generic-family** on purpose: software-domain verification requires the per-project check pack (S13, B4) and fails loudly without it; generic runs the ratified S07.8 degraded mode (V1 empty, judge verdict authoritative-for-demo, marked).
- No research tools exist yet (egress substrate is a gate decision), so keep the request free of P47 trigger words (prices, "latest", product names, dates) — otherwise the plan must carry research nodes that can only end in an UNVERIFIABLE-HERE / RESEARCH-NOT-RUN card. The suggested haiku task is trigger-clean.

**Prerequisites:** repo `main` at `e578f83` or later; Go on PATH (`go version` → 1.26.5); `claude` CLI installed and logged in (`claude --version` → 2.1.215 per the pinned engine); ~5 min.

---

## Part A — boot

1. **Build the binary:**
   ```
   cd ~/Sinet-Agentic-Control-Hub && go build -o ~/sinet-demo/sinet ./cmd/sinet
   ```
   Expected: silent success; `~/sinet-demo/sinet` exists.

2. **Terminal A — start the control plane (leave it running, watch its log):**
   ```
   ~/sinet-demo/sinet control --state-dir ~/sinet-demo/state --http-addr 127.0.0.1:8420
   ```
   Expected log lines, in order: `state: platform.db open` → `sandbox: host probe` (available=true on this machine) → **`stage: dev posture — engine spawns UNCONFINED …`** (expected, see posture above) → listener bound on `127.0.0.1:8420` → `platform.started`.

3. **Terminal B — health:**
   ```
   curl -s http://127.0.0.1:8420/api/health
   ```
   Expected: `{"ready":true,"mode":"running",…}`.

4. **(Optional) Terminal C — live event feed, the platform's spine visible:**
   ```
   curl -N -s -b ~/sinet-demo/cookies http://127.0.0.1:8420/events
   ```
   (Start it after step 6 so the cookie exists; every event below — `run.created`, `run.state`, `ledger_update`, `context.manifest`, `engine.usage`, `run.checkpoint`, `intake.state`, `verify.round` — streams here with `id:` = event_seq.)

## Part B — identity (the D10 requester)

5. **Bootstrap the operator user** (allowed exactly once while `users` is empty; pick a real PIN — you will type it again at approval):
   ```
   curl -s -X POST http://127.0.0.1:8420/api/auth/users \
     -H 'Content-Type: application/json' \
     -d '{"user_id":"sinep","display_name":"Sinep","role":"operator","pin":"<YOUR-PIN>"}'
   ```
   Expected: HTTP 201, `{"user_id":"sinep"}`. The `role` field is mandatory and must be `operator` in the bootstrap window — the platform never defaults a role silently (S01.9; B0-5 contract). *(Script fix 2026-07-20: the original step omitted `role` and mis-stated the response shape — caught live at the gate demo; code behavior was correct.)*

6. **Log in (session cookie into a jar):**
   ```
   curl -s -c ~/sinet-demo/cookies -X POST http://127.0.0.1:8420/api/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"user_id":"sinep","pin":"<YOUR-PIN>"}'
   ```
   Expected: `{"user_id":"sinep","expires":…}`; jar file written. All following curls carry `-b ~/sinet-demo/cookies`.

## Part C — intake (interview → spec → plan → approval)

7. **Submit the request:**
   ```
   curl -s -b ~/sinet-demo/cookies -X POST http://127.0.0.1:8420/api/intake/requests \
     -H 'Content-Type: application/json' \
     -d '{"title":"Skeleton haiku","text":"Write a haiku about a walking skeleton platform coming alive. Deliver it as a short text file."}'
   ```
   Expected: task view JSON — note the `task_id` (call it `$T`); `runs` shows `$T.intake` queued/running; `tier` is **high** (no classifier seam yet → fail-closed, S06.2 — correct behavior, and it forces the full-ceremony path this demo wants to show).

8. **Fetch the interview card** (the claim loop dispatches within ~1 s):
   ```
   curl -s -b ~/sinet-demo/cookies http://127.0.0.1:8420/api/tasks/$T
   ```
   Expected: `open_ask_id` set; `open_card` is a `kind:"interview"` card with up to 4 highest-weight questions from the generic taxonomy, plus live `clearance`.

9. **Answer one or two questions for real** (watch Clearance rise), using the slot ids from the card:
   ```
   curl -s -b ~/sinet-demo/cookies -X POST http://127.0.0.1:8420/api/asks/<open_ask_id>/answer \
     -H 'Content-Type: application/json' \
     -d '{"answer":{"answers":[{"id":"<slot-id>","value":"<your answer>"}]}}'
   ```
   Expected: task view again — clearance higher; a fresh interview card if still below the high floor (90).

10. **Force-proceed past the rest** (S06.5: vagueness becomes VISIBLE assumptions, never silent):
    ```
    …/answer -d '{"answer":{"force_proceed":true}}'
    ```
    Expected: **[PAID ×2]** the planner session drafts SPEC+PLAN and (high tier) the critique session attacks it — watch Terminal A/C: `context.manifest` (the injected brief, manifested), `engine.usage` + `run.checkpoint` rows (D7 per paid call, each carrying the live ledger-revision block), `ledger_update` events as artifacts/decisions land. Then the task view shows the **approval card**: Layer 1 with restatement, numbered plain steps, what it will NOT do, **the assumptions list as the centerpiece** (your force-proceed conversions are here), risks, `UNPRICED` cost line (empty price table — honest posture), Clearance; Layer 2 with numbered ACs in dual phrasing, the plan + AC coverage map, and the critique verdict.

11. **Approve — WITH your PIN in the same request** (High tier → S01.9 verify-at-act step-up; without `pin` this returns 401 `pin_required`, which is itself worth showing):
    ```
    curl -s -b ~/sinet-demo/cookies -X POST http://127.0.0.1:8420/api/asks/<ask-id>/answer \
      -H 'Content-Type: application/json' \
      -d '{"pin":"<YOUR-PIN>","answer":{"action":"approve"}}'
    ```
    Expected: ACs freeze + pin into the ledger (`ledger_update` events: objective_ac, constraints, plan_version); the S02.8 write-set claim row lands; `*.approved.md` artifacts appear under `~/sinet-demo/state/artifacts/$T/`; the intake run **completes and its ceremony receipt materializes**; `$T.execute` is created and enqueued automatically.

## Part D — execute → verify (the skeleton walks by itself)

12. **Watch execution** — no input needed. **[PAID ×N]** one fresh engine session per plan step (Read/Write/Edit tools only — no shell, no network), each checkpointed. Then: deliverable filed as `artifacts/$T/deliverable-rev1.md`, ledger work items flip to `done_unverified`, stage-close gate passes, execute run completes, `$T.verify` launches itself.

13. **Watch verification** — **[PAID ×2 (+rework if judged REVISE)]** V0 pre-gates (free) → V1 empty (generic family, degraded mode — S07.8) → both judge axes on the clean-context slice. Then:
    ```
    curl -s -b ~/sinet-demo/cookies http://127.0.0.1:8420/api/tasks/$T
    ```
    Expected: `kanban_status` **"done"** on SHIP/SHIP-with-notes — or **"attention"** with a durable card if the judge escalated (also a pass for the demo: that is the tested escalation ladder doing its job; the card text explains itself).

14. **Read the receipts — the D-line's last stop:**
    ```
    curl -s -b ~/sinet-demo/cookies http://127.0.0.1:8420/api/runs/$T.intake/receipt
    curl -s -b ~/sinet-demo/cookies http://127.0.0.1:8420/api/runs/$T.execute/receipt
    curl -s -b ~/sinet-demo/cookies http://127.0.0.1:8420/api/runs/$T.verify/receipt
    ```
    Expected: three receipts, purpose-tagged **ceremony / execution / verification** respectively (S06.10/S07.11 itemization), full measured token breakdowns, priced **UNPRICED tier-5** (empty price table — the S10.1 honest posture; the genai-prices seed is a flagged follow-up).

15. **Durability leg:** Ctrl+C in Terminal A (clean shutdown log, exit 0) → start it again (same command). Expected: `integrity_check ok`, recovery-ladder pass classifies nothing (all runs terminal), the task view and receipts read back identically — the whole walk lives in `platform.db`, not in any process memory.

**Gate criterion:** every expected result above observed live ⇒ the walking skeleton stands (S19.5 B2 exit). Anything that deviates: stop, keep the state dir, tell the coordinator — the event log carries the full trace either way.

**Cleanup (optional, after the gate closes):** `rm -rf ~/sinet-demo` — nothing outside it was touched.
