# P3 handoff — written 2026-07-28 at the close of the B5 build

**Read this first, then `P3/STATE.md`.** This file is a *snapshot* meant to get a fresh session oriented in two minutes. It is deliberately short and it goes stale; **`P3/STATE.md` is the single source of truth and outranks anything here.** If the two disagree, STATE wins and this file should be corrected.

Authority order for everything: **`Spec/core-architecture-v1.md` (frozen v1, tag `spec-v1`; drafts in `Spec/drafts/` are canonical text)** → `Spec/benchmark-preregistration-v1.md` (BENCH-REG, signed — its registered numbers change only via its own §17) and `Spec/frontend-components-v1.md` → `P3/CONVENTIONS.md` → `P3/STATE.md` → this file. `Research/` is a **closed archive**; `Docs/` is **read-only**.

---

## 1. Where the build is

**Phase B5 (observability & evals, S14) — BUILD COMPLETE. All ten packets done and validated. The B5 gate is OPEN and unpresented.**

Phases closed so far: **B0** spine · **B1** execution substrate · **B2** pipeline (walking skeleton) · **B3** workforce & memory · **B4** deliverables & local tier. **B5's build is finished but its gate has not been presented**, so B5 is not yet closed. **B6** (React SPA over every API — S15 + `Spec/frontend-components-v1.md`) is the last phase and has not started.

The ten B5 packets, in order: event-type contract · live SSE + serve-side redaction · watchdog suite · conformance registry · regression evals + promptfoo · watchlist executor · API canary layer · benchmark practice · retention substrate · queryable history.

**Counters at the close (assert these still hold before you change anything):**

| Thing | Value |
|---|---|
| Event inventory | **88 minted / 7 declare-only = 95 registered** |
| ⚙ settings | **118 keys / 33 domains** — `internal/settings/index.go` was byte-unchanged for the *entire* phase |
| `components.lock` | **21 entries** (lockgate green) |
| Migrations | **0001–0016** (`user_version` 16) |
| Test packages green | **42** |
| CONVENTIONS | §1–§37 (§29–§37 are B5's; earlier sections never rewritten) |
| Paid spend across all of B5 | **$0** |

Git: `main` == `origin/main` at **`7495c5d`**, everything pushed. The working tree carries one intentional non-packet modification — `Research/02-provider-watchlist-and-onboarding-criteria.md`, a **pre-existing operator-instructed addendum**. Every stage all session deliberately left it unstaged. Do not stage, revert, or "clean" it. Untracked `Presentation/`, `Research/Presentation/`, `tools/dbpeek/` are likewise pre-existing operator items.

---

## 2. The next session's first act — the B5 gate

Open the session with **"continue implementation"** (or `/p3-implementation`). Then, in order:

1. **Start the session on Opus**, not Fable — see §5. The gate presentation summarizes B5-8B's SQL-guardrail work, which is exactly the content that tripped the safeguard last session.
2. Run the session-entry checks from the runbook: read `P3/STATE.md`, confirm `git status` is clean apart from the known operator files above, and re-run the landing battery so the gate report's evidence is measured, not remembered:
   `gofmt -l internal/ cmd/` · `go vet ./...` · `go build ./...` · `go test ./... -count=1` · `go run ./tools/lockgate`
3. **Write `P3/gates/B5-report.md`**, following the shape of `P3/gates/B4-report.md`: what shipped (packet → commit → one line), evidence (battery, conformance highlights, spend), measurements taken and due, deviations and readings, then a numbered decision list. Commit it.
4. **Present the gate to the operator in plain language** — walkthrough first, then the decisions. Operator free-text answers are authoritative and must not be re-asked via a form (this is a standing preference).
5. Record the answers in the gate file **and** STATE, execute what they authorize, close B5, and open B6 by cutting its packet queue from S19.5 + S15 + `Spec/frontend-components-v1.md`.

**Do not start B6 packets before the gate closes.**

---

## 3. What the operator is being asked at the B5 gate

This batch accumulated across the phase. Everything here is an operator act; none of it was done unilaterally.

**Host installs** (these touch the machine, so they are gate decisions):
- promptfoo — `npm install -g promptfoo@0.121.19`
- changedetection.io — pipx/venv from PyPI **0.55.8**, plus the decision on installing the Sinet-**generated** `sinet-watchlist.service`
- Note: **per the operator's standing hand-steps rule, these should be delivered as a single guided, self-verifying script** — not as loose commands to paste. Three-plus commands with interactive prompts is exactly the threshold that rule names.

**Lock ratifications** (S16.4 check #10): promptfoo · changedetection.io · the genai-prices vendored data.

**Arming decisions** — the canary layer ships deliberately disarmed:
- The **auth** and **model-list** canaries cannot be composed at all today: no per-lane HTTP endpoint or broker credential accessor exists in the tree. Arming them is gate-time composition work, honestly recorded rather than faked.
- The **behavioral** canary's arming carries a precondition: promptfoo issues its provider calls itself, so Sinet sees no per-call usage stream; the itemization question must be answered at arming.
- Its pre-registered projection: **real dollars must be exactly $0.00** on the flat-rate lanes (any non-zero probe-tagged total is itself the alarm — disarm and flag), with ≈$0.11/wk ≈ **$5.70/yr** API-equivalent for the behavioral leg.
- Credential custody: `SINET_CANARY_ARM`, `SINET_CDIO_API_KEY`, and the authenticated Reddit feed URLs (the unauthenticated seed URL live-rate-limits at HTTP 429).

**Ratifications**: the B5-5 eval floor (TPR 0.84) · the B5-5 paid golden-sweep say-so ($2.10 projected against a $5.00 stop line — **pre-registered but never executed**).

**Named follow-on scope** (recorded so it cannot fall between chairs):
- B5-7's **dispatch→render driver** — single-shot direct-arm output capture plus the B6 verdict form. Until both exist no benchmark pair can complete; this was a deliberate honest absence, not an oversight. BENCH-REG accrual is a post-B6 bring-up act and the opt-in defaults OFF, so nothing samples until operators opt in.
- The B5-8B **family-12 `RequiredFields`** question — an S00.9 amendment *if* the operator wants the frozen event-family row to name the new type. The packet correctly refused to author spec text inside a packet.
- Known bounds to acknowledge: redaction covers only its enumerated secret classes; the Layer-2 guardrail refuses honest *unfenced* reasoning that opens with a SQL keyword (fenced output is immune) — this must ride the bring-up acceptance measurement as an expected refusal source so guardrail conservatism is never misread as model weakness.
- Carried suites and rows: B5-4's compaction-canary standing suite · B5-3's silence-budget TBD-BRINGUP row · the structural cadence constants awaiting the settings-tab amendment.

**Carried from earlier gates, still open:** Z.AI prompt-unit calibration (parked — no Z.AI lane) · the probe batch's remaining legs (catch-up suspend, paired with the battery-drain hour; `user.slice` freeze/thaw, paired with the hardening session) · week-one push drill on household phones (first deploy) · optional GitHub Verified badge · `/tmp/llamaswap-test/` awaiting an operator `rm`.

---

## 4. How the machinery works (do not reinvent it)

Every packet runs a **four-stage pipeline**, all stages launched by the coordinator as fresh-context background agents, strictly sequential within a packet:

**grounding** → writes `P3/briefs/P3-<phase>-<n>.md` (the handoff artifact *and* the evaluation rubric; numbered requirements with spec refs, seams, ⚙ by name, acceptance checklist, and open questions for coordinator disposition) → **executor** (always Opus) → **evaluation** (fresh agent, judge ≠ executor, told to find what is wrong) → **drain** (coordinator triages findings, executor fixes, evaluator re-checks; **hard cap of two rounds**, after which the coordinator implements the remainder inline).

What repeatedly earned its keep this phase:

- **Open questions get coordinator dispositions before the executor launches.** Grounding surfaces genuine choices with evidence and a recommendation; the coordinator rules and logs it. Nothing is resolved silently.
- **Evaluations are adversarial and reproduce their own claims.** The strongest catches came from evaluators planting canaries, reverting fixes to prove tests bite, and inventing attack routes beyond the shipped battery. B5-8B's critical finding was proven end-to-end before it was reported, and the fix was then attacked from 28 fresh angles.
- **A declared deviation is a referral, never an acceptance.** Every stage that hit judgment called it out; the coordinator ruled. Several referrals turned out to be real defects, and several were sound readings worth recording.
- **Split a packet before an executor rushes it.** B5-3, B5-6 and B5-8 were split on the grounding's scope verdict, and B5-8B additionally stopped clean at a pre-authorized seam mid-build. That is a success mode, not a failure.
- **Never commit while a stage agent is live in the tree** (a real index race happened in B5-3). Use `git commit --only P3/STATE.md` for coordinator bookkeeping while an agent runs.
- **Migration immutability has one narrow exception, now precedent:** a migration may be edited during its *own* packet's drain, because §6 immutability protects migrations already applied to real databases and none exists yet. Immutability re-attaches at landing. This was ruled and logged for 0015.
- Update STATE **before and after every step**. Push after milestone commits.

---

## 5. Model routing — read before you start

Executors and finalizers are **always Opus** (judge independence plus classifier immunity mid-run). Grounding and evaluation inherit the session model, with a **lossless Opus fallback** on any safeguard trip. B5-8 ran all-Opus because the Layer-2 open-SQL work is an injection surface.

**The lesson from the last session:** the *coordinator itself* tripped Fable's dual-use safeguard while relaying B5-8B's security evaluation to the operator — the first coordinator-side trip; every earlier one hit a stage agent. The operator switched to Opus 5 with `/model` and work continued losslessly mid-drain. So: **on any session that will summarize security-dense work, start on Opus.** The B5 gate presentation qualifies. This is a documented false positive on the operator's own authorized infrastructure work — never reword to evade the classifier; report via `/feedback` if it recurs.

---

## 6. Invariants that must never be broken

- **The spec wins** — over model memory, over the research reports, over existing code. A discovered conflict, gap, or impossibility is *never* resolved silently: stop the packet, write an S00.9 amendment proposal, present it. Any amendment touching a ⚙ setting re-runs the S18 sweep.
- **D1–D10 are fixed.** Never re-derive or relitigate them.
- **Adopt, don't fork.** Adopted engines are never modified; pins are exact; `components.lock` plus the CI lock-gate cover every dependency.
- **Every ⚙ number ships through the settings registry** with clamps and audit — never a constant in code. Structural constants are permitted but each carries a named reason and is interim under the standing settings-tab directive.
- **No load-bearing metered paths**; subscription-coverage rule holds.
- **Real-world facts are live-verified at time of use** — library versions, provider behavior, licenses — never answered from memory.
- **BENCH-REG registered numbers are read-only data.** Silent drift between that text and the running platform is a platform defect by its own clause.
- **No auto-kill anywhere** (S14.4 / G1-D1.3). Watchdogs and canaries pause and flag; they never terminate.
- Secrets are never committed. Host-level changes (systemd units, sysctls, packages) are proposed and approved, never applied unilaterally — and units are **generated, not installed**.

---

## 7. Host and environment state you inherit

- **Local inference tier is live-capable, user-level:** llama-swap v241 installed; llama.cpp b10085 CUDA-built for sm_120 via a user-level CUDA 12.9 toolkit and micromamba gcc-14 — **no driver, kernel, or system-CUDA change was ever made, and none may be.** Weights and scratch live in `~/.sinet-b45` (~38 GB, outside the repo). Tier-L GPU smoke proved green twice.
- **Production front chain is LIVE on this machine:** `tailscale serve` → Caddy on 127.0.0.1:8481 → the production unit on 8482, with `sinet-control` and `sinet-broker` active. **Packet tests must stay hermetic and never touch the live Caddy admin API or config** — this hazard is real and has its own tripwire.
- Three model seats remain unpulled and none is on the v0 default path (an HF rate limit, a license gate needing an operator account act, and one architecture with no GGUF).
- Service-context confined runs stay blocked by the AppArmor userns finding until the dedicated hardening session (needs sudo, operator scheduling).
- The B5 organs — changedetection.io and promptfoo — are **not installed**; that is exactly what the gate decides.
