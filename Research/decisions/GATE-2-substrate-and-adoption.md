# GATE-2 — Substrate + adoption

**Opened:** 2026-07-17 · **Wave covered:** B (B1 + B2) · **Status:** OPEN
**Reports in scope:** Research/08 (T07 durable state), 09 (T08 metering/quota/scheduling), 10 (T09 sandboxing), 11 (T10 memory), 12 (T11 evals/observability), 13 (T12 deliverables/review/git), 14 (T16 OSS harvest validation)

## Findings digest

### T07 — Durable state, checkpointing & recovery (`Research/08-durable-state-checkpointing-recovery.md`)
- **Recommendation:** one platform-owned SQLite-WAL `platform.db` (`synchronous=FULL`, `BEGIN IMMEDIATE`, control plane sole writer), ~12 owner-attributed tables; checkpoint = one row per paid call (+ Claude-lane transcript copy-aside); startup/wake recovery ladder (ALIVE / WEDGED / FINISHED-DURING-OUTAGE→harvest / DEAD→fork-from-checkpoint) with leases, generation fencing, suspend-aware grace; two-phase effect journal with payload-hash-pinned approvals + per-provider idempotency registry; kill-9 crash harness as a standing conformance-suite entry.
- **Key evidence:** the field converged on "durable execution = persistent memoization" and on SQLite being enough at this scale (DBOS now ships SQLite as its default); OpenClaw independently ships the same restart-recovery ladder; harvest-first beats blind requeue (Solid Queue's rationale).
- **Changes to prior assumptions:** two explicit R05 corrections recorded (DBOS-requires-Postgres ground stale — rejection re-argued and stands; pre-sleep budget is ~5 s → recovery is wake-side, pre-sleep is an O(1) flush). P-T07-1..5 filed.

### T08 — Metering, quota & scheduling (`Research/09-metering-quota-scheduling.md`)
- **Recommendation:** ledger row per paid call written at the D7 checkpoint (no row silently prices to $0); two currencies — pressure gauge per (person, lane) against operator-declared budgets; dollars only from an effective-dated price table; five-class limit-event taxonomy driving retry/park/freeze; effort modes as *disclosed* depletion ladders (visible mode indicator + receipt lines); one SQLite scheduler (priority ladder, cache-window-aware resumes, Quartz-style missed-slot policies); local arbitration via systemd slices + a VRAM-ledger GPU admission check.
- **Key evidence:** exact metering is achievable on every v0 lane (5-tier approximation hierarchy); budget/policy/auth events masquerade as each other on the wire → the classifier is a tested component; the brief's novelty conjecture was REFUTED (OmniRoute/teamclaude do consumption-pressure routing but are D2/D4-incompatible, study-only) — Sinet's residual novelty = interactive headroom + disclosed background demotion.
- **Changes:** P-T08-1..5 filed (engine usage *values* drift, not just schemas; prices carry effective dates; pause semantics can destroy parked work — named anti-behavior with a test).

### T09 — Sandboxing & confinement (`Research/10-sandboxing-confinement.md`)
- **Recommendation:** composed kernel-primitive per-run sandbox — systemd transient unit → bubblewrap → seccomp → Landlock — with per-run netns + nftables default-drop as the real egress boundary (allowlisting proxy = convenience), Anthropic's Apache-2.0 `sandbox-runtime` adopted for the bwrap+seccomp+proxy core, credentials only in an ssh-agent-shaped host-side broker (never inside sandboxes), confinement classes C0–C3 carried declaratively per worker with a Rule-of-Two admission check. v0 ships C0–C2; C3 (hostile web-reading) at v0.1 with gVisor step-up.
- **Key evidence:** both shipping first-party harnesses independently converged on exactly this stack; the 2025–26 proxy-bypass CVE run proves firewall-not-proxy; "blast radius, not immunity" is the 2026 consensus posture.
- **Changes:** two new problems filed (agent-writable config is an escape surface — CVE-backed; allowlisted egress is an exfil channel). Three host facts on the actual laptop are unverified (userns sysctl, Landlock ABI, XFS/btrfs volume) → Decision 2.3.

### T10 — Memory & knowledge (`Research/11-memory-and-knowledge-architecture.md`)
- **Recommendation:** deterministic-first, platform-owned memory: three write-permissioned layers (L0 task scratch / L1 descriptive observations TTL 90 d / L2 human-gated knowledge) × four scopes (user / worker-overlay / project / house), stored as git-versioned markdown + platform.db rows with FTS5 search; learning admitted only through a proposal→approval pipeline with per-item provenance and injection-manifest influence tracing; vector retrieval held behind a pre-registered miss-rate gate; no agent-supplied identifier ever selects memory.
- **Key evidence:** the memory-sidecar benchmark basis collapsed under audit (LoCoMo ~6.4% corrupted keys; Zep 58–84% depending on runner); field pioneers converged on git-versioned files; Letta's own contra-interest benchmark has plain filesystem beating Mem0.
- **Changes:** N17's rolling-summary `lts` layer DROPPED (indicted self-reinforcement class); the descriptive-vs-prescriptive boundary resolves the S4.3-vs-8.1 tension. Three new problems filed (engine-memory drift = 8.1 bypass; proposal-noise economics unmeasured; knowledge-injection budget pressure).

### T11 — Evals, observability & benchmark (`Research/12-evals-observability-benchmark.md`)
- **Recommendation:** the D7 event log IS the observability stack — no external trace platform; T11 completes the event-type contract (verdicts, decisions, routing, knowledge-injection) + one SSE endpoint for live inspection (resume = one indexed SELECT); watchdog = deterministic counters at published thresholds + local-model disambiguator (annotates, never kills) + pause-and-flag; watchlist executor = pinned changedetection.io + Sinet-native feed poller + Sinet-built API canary layer (auth / conformance / behavioral / logprob); benchmark = blind pairs with *measured* blindness, epoch stamping, anytime-valid Beta-Binomial gate; regression evals on pinned promptfoo (DeepEval swap pre-registered), all scores in Sinet's DB.
- **Key evidence:** trace platforms are 3–5 orders oversized for household volume and churning through M&A (Helicone → maintenance mode, promptfoo → OpenAI); the benchmark's direct arm is un-pinnable (consumer surfaces auto-upgrade) → epoch stamping; household blinding fails unless measured (expert users detect style near-perfectly).
- **Changes:** P-T11-1..5 filed (tools are runners, never stores; small-n statistical honesty is a product surface, not a footnote).

### T12 — Deliverables, review & git (`Research/13-deliverables-review-git.md`)
- **Recommendation:** Gerrit/Phorge deliverable schema (immutable numbered revisions; comments with redundant quote+position anchors; re-anchoring degradation ladder ending in an explicit, visible **orphan** state); accept = host-side attributed commit (author = committer = accepting user, engine Co-Authored-By trailers) pushed via broker-held SSH keys with explicit CAS; branch-per-pipeline + platform snapshot commits (no jj, no stash, no engine checkpoints as the net); previews = host-toolchain runner inside the settled sandbox stack (only novel code: ~200-line port-pool daemon + dual-iframe before/after UI); 11.3 = `VACUUM INTO` → dump → tar|zstd|age → private-repo keep-N with SHA-256 ledger + escrow-identity restore drill.
- **Key evidence:** fine-grained PATs cannot push to collaborator repos (forces SSH) and Free private repos have ZERO enforced protection → **the broker IS the guardrail** (P-T12-1); Hypothesis's ~22% orphan rate proves the explicit orphan state is mandatory; age has no sender authentication → the hash ledger is load-bearing.
- **Changes:** P-T12-1..4 filed; closes R08-OQ5 (snapshot mechanics settled).

### T16 — OSS harvest validation (`Research/14-oss-harvest-validation.md`)
- **Recommendation:** the consolidated G2 adoption package (§4.1): Layer A running deps re-verified healthy; Layer B new organ-grade ADOPTs — genai-prices (MIT price-table defaults, closes the one donor-less area), systemd-creds + sops/age (broker mechanics), frontend shortlist (non-binding); Layer C patterns only (LangGraph interrupt/approval schema for the empty approval-inbox niche, gh-aw safe-outputs, AG-UI vocabulary). Report 14 §6.1 becomes the citable harvest reference, superseding `Docs/component-harvest-map-proposal-v1.md`.
- **Key evidence:** zero capability failures across the whole map; the 2025 control-plane cohort is dead (~14-month half-life: Bloop/VibeKanban, Omnara, Terragon, HumanLayer, Crystal) → build-the-control-plane / adopt-the-organs is what revealed mortality teaches; the approval-inbox niche is EMPTY (build it); the "Agent SDK banned" press claim is refuted by Anthropic's primary legal text (P2 gray-zone posture SUPPORTED).
- **Changes:** Crush anti-harvest *rationale* corrected — FSL legally permits private code use; patterns-only survives as policy (ratified in Decision 2.2); ClawHavoc poisoned ~12% of a public skill registry → no-public-registry-imports rule (P-T16-4). P-T16-1..4 filed.

## Decisions required

### Pre-registered / coordinator-closed during Wave B (recorded, NOT re-asked)

| # | Item | Resolution | Where |
|---|---|---|---|
| P1 | R14-OQ3 N15/N19 vs report 13 | Report 13 confirmed report 14's verdicts — no reconciliation needed | STATE log |
| P2 | R14-OQ8 "SDK banned" press claim | Refuted by primary Anthropic legal text; P2 posture SUPPORTED; exact quotes → spec compliance note | report 14 §7 |
| P3 | R14-OQ6 Agent Vault | STUDY input to the broker spec workshop (single-host voids its premise) — no decision | report 14 §7 |
| P4 | R14-OQ7 OpenHands fallback spike | Target moved to `OpenHands/software-agent-sdk`; runs only if triggered, post-G2 | report 14 §7 |
| P5 | R14-OQ4 frontend picks | Shortlist only; binding choice at the spec-phase frontend workshop | report 14 §3.3 |
| P6 | R06-OQ2 native micro-fanout | Still DEFERRED with standing operator reminder at adapter-spec drafting (G1 rider 2) — carried, not reopened | GATE-1 |

### DECISION 2.1 — Ratify the substrate package (state + metering + memory + observability + deliverables)
- **Context:** five of the six G2 subjects have exactly one evidenced recommendation each and zero surviving competitors: (a) **state layer** = report 08 §4 (platform.db, checkpoint-per-paid-call, recovery ladder, effect journal, kill-9 harness); (b) **metering/scheduling** = report 09 §4 (ledger, pressure gauge, five-class limit policy, effort-as-disclosed-depletion, one scheduler, local arbitration); (c) **memory** = report 11 §4 (L0/L1/L2 × 4 scopes, human-gated L2, FTS5, vector post-gate); (d) **observability** = report 12 §4 (event log as the stack, watchdog suite, watchlist executor, benchmark protocol, regression evals); (e) **deliverables/review/git** = report 13 §4 (revision schema, anchor ladder, broker-mediated accept flow, preview stack, snapshot pipeline). All compose on the G1-ratified direction; every ⚙ number ships as an operator-editable setting (G1 rider 1).
- **Options:** A) Ratify all five. B) Ratify with named exceptions. C) Hold for discussion.
- **Recommendation:** A — each is the convergent-evidence winner; Wave C and the P2 spec consume them as settled inputs.
- **Forecloses:** A → nothing irreversibly (all Sinet-owned designs behind contracts; numbers are settings) · C → Wave C can still launch, but the P2 spec waits.

### DECISION 2.2 — Ratify the adoption list (harvest verdict list, report 14 §4.1 + §6.1)
- **Context:** the sixth G2 subject. **Layer A** (running deps, re-verified): pinned opencode v1.x serve · wrapped `claude` CLI · SQLite-WAL · systemd/bwrap/seccomp/Landlock/netns+nftables · `sandbox-runtime` · AGENTS.md+CLAUDE.md shim · promptfoo · changedetection.io. **Layer B** (new, organ-grade): genai-prices price defaults · systemd-creds + sops/age broker mechanics · frontend shortlist (non-binding). **Layer C:** patterns only (LangGraph interrupt schema, gh-aw safe-outputs incl. threat-detection pass, AG-UI vocabulary, Archon A2 dialect vocabulary — own versioned dialect, never their parser). Anti-harvest rows all CONFIRM; one rationale corrected: Crush's FSL *legally* permits private code use — patterns-only survives as **policy** (R14-OQ1). Report 14 §6.1 supersedes the Docs harvest proposal as the citable reference; P-T16-1 makes every ADOPT carry pin + replacement path + abandonment criteria (component-onboarding checklist, the report-02 §5 twin — spec requirement).
- **Options:** A) Ratify the package incl. Crush patterns-only-as-policy. B) Ratify but permit Crush code harvest (legally allowed). C) Named exceptions.
- **Recommendation:** A — every add is organ-grade (data files, OS mechanisms, single-purpose components), no new spine; Crush code offers nothing the K1/K2 patterns don't (Go/TUI stack mismatch) and patterns-only keeps adopt-don't-fork clean.
- **Forecloses:** B → nothing hard (revisitable per-version as FSL→MIT conversions land from ~mid-2027).

### DECISION 2.3 — Sandbox stack ratification mode (R10-OQ3 — completes the sixth subject)
- **Context:** report 10's stack is the unanimous field answer, but three facts about the actual laptop are unverified: `kernel.apparmor_restrict_unprivileged_userns` posture on Ubuntu 26.04 (bwrap under unprivileged userns), the Landlock ABI level (TSYNC multithreaded enforcement; UDP scoping), and whether the project-store volume is/can be XFS or btrfs (ext4 kills reflink CoW workspaces). Each has a documented fallback (per-binary AppArmor profile; nftables-only egress — which is the boundary anyway; plain-copy workspaces).
- **Options:** A) Ratify now, **contingent** on a pre-P2 host-verify afternoon (probe list in report 10 §7.3); fallbacks auto-apply, any surprise returns as a card. B) Operator runs the probes first; ratify afterwards.
- **Recommendation:** A — no probe outcome changes the stack's shape, only named sub-choices; B blocks the gate on operator hands with no decision-relevant upside.
- **Forecloses:** nothing either way; A keeps the campaign moving.

### DECISION 2.4 — Outward-effect channel policy (R08-OQ3, P-T07-3)
- **Context:** plain-SMTP-class channels cannot be made exactly-once by any journal — a crash in the send window leaves "unknown: did the email go out?" cards for a human to resolve. Providers with idempotency keys (Resend-class) close this window.
- **Options:** A) Require idempotent-capable providers for email-class outward effects; plain SMTP only as an explicit per-channel exception. B) Allow plain SMTP generally; accept routine unknown-outcome cards.
- **Recommendation:** A — channel choice is a durability design input; class-D unknowns should be exceptional.
- **Forecloses:** A → nothing (exception path exists) · B → recurring manual did-it-send checks forever.

### DECISION 2.5 — Backup posture (R08-OQ5 + R13-OQ3)
- **Context:** the load-bearing 11.3 mechanism is settled inside 2.1(e): scheduled dump → age-encrypt → private-repo snapshot, keep-N, SHA-256 ledger, scheduled escrow-identity restore drill. Two riders remain: (i) also run pinned Litestream v0.5.x as *continuous* local/off-host replication (its bug #1083 — silent replication failure — is exactly the class Sinet bans); (ii) GitHub retains rewritten-away blobs indefinitely → an old age key decrypts every snapshot ever pushed (P-T12-3); annual snapshot-repo rotation bounds that exposure.
- **Options:** A) Snapshots daily, keep-30 ⚙, annual repo rotation; Litestream deferred to implementation once #1083 is triaged. B) Add Litestream now. C) Snapshots only, no rotation (accept indefinite encrypted remnants).
- **Recommendation:** A — the drill-tested dump lane is the guarantee; Litestream is additive; rotation is one cheap operator task per year.
- **Forecloses:** nothing — all additive/reversible.

### DECISION 2.6 — Git identity hardening (R13-OQ1 + R13-OQ2 — spends money)
- **Context:** (i) the broker can SSH-sign every accept-commit per user (green "Verified" badge; one extra pubkey upload per member; all-or-nothing per user — unsigned users must never enable GitHub vigilant mode). (ii) On Free private repos the broker is the ONLY force-push protection (P-T12-1); GitHub Pro (~$4/mo per repo-owning user) buys *enforced* rulesets as a server-side second layer.
- **Options:** A) Signing: operator from day one, members opt-in at enrollment; Pro: NO at v0, revisit when a second member joins. B) Signing + Pro now. C) Neither at v0.
- **Recommendation:** A — signing is free hardening where it matters most (operator repos); Pro's marginal layer matters once non-operator credentials exist, which operator-only v0 doesn't have.
- **Forecloses:** nothing — both reversible anytime.

### DECISION 2.7 — v0 memory surface (R11-OQ4)
- **Context:** 15.6 requires the full schema day one. What *activates* at v0 is open: report 11 recommends v0 = knowledge injection (house/project/user) + L0 scratch + manual L2 entries (operator-written), with the auto-proposal pipeline (evidence → local-model drafting → approval queue) activating at v1 per 15.4's learning sequencing.
- **Options:** A) As recommended — proposal pipeline at v1. B) Pull the proposal pipeline into v0.
- **Recommendation:** A — proposal-noise economics are unmeasured field-wide; v0 first accumulates the evidence stream the drafter needs, and manual L2 exercises the full injection path anyway.
- **Forecloses:** A → v0 learns only what humans write in (fine for one operator) · B → v0 ships an untuned proposal queue competing for operator attention.

### DECISION 2.8 — Done-directly figure + benchmark pre-registration (closes R09-OQ3 = R12-OQ2; sets up R12-OQ1)
- **Context:** the receipts' "what this would have cost done directly" figure is the platform's honesty keystone; its formula must not float. Report 12 refined report 09's proposal into a two-stage form: per-run heuristic (final accepted attempt's execution usage at list price, labeled "direct-use estimate (heuristic)") → per-domain measured median once ≥10 benchmark pairs exist (labeled "measured, n=…"). The full benchmark package (gate ⚙20 non-tied pairs at P≥0.90; alarm P>0.95; measured blindness; epoch stamping; no cross-epoch pooling) freezes in a signed pre-registration commit — a dedicated session at P2 spec time, before v0 ships.
- **Options:** A) Ratify the two-stage formula now + commit to the P2 pre-registration session with report 12 §4.6 as the draft. B) Ship receipts without the figure until measured pairs exist.
- **Recommendation:** A — the heuristic is honest when labeled; receipts silent on the one number the operator most wants would defeat 3.6.
- **Forecloses:** A → later formula changes require a dated re-registration (deliberate friction) · B → the honesty keystone missing at launch.

### DECISION 2.9 — Next spike battery: scope + timing
- **Context:** accumulated probe-class work, none of it consumed by Wave C research: engine credential-injection probe per lane (R10-OQ1 — decides whether engine creds stay fully outside sandboxes); host-verify afternoon (Decision 2.3); systemd harvest matrix on this host (R08-OQ8); serialize-by-deny fallback probe + optional ~$2 live parallel-rate battery (S2 carry-over); S3 live-auth park re-run + Z.AI prompt-unit calibration (R09-OQ4 — operator provisions the Z.AI key, literal steps in the S3 report); `anthropic-ratelimit-unified-*` header probe (R09-OQ6); logprob availability probe per lane (R12-OQ7). All ≤$0.50 except the ~$2 battery; no app code.
- **Options:** A) One consolidated battery at P2 entry (post-Wave-C, pre-spec); operator provisions the Z.AI key whenever convenient before then. B) Run the battery now, parallel to Wave C. C) Defer everything to P3 implementation.
- **Recommendation:** A — P2 is when the answers bind spec text, and running them fresh there avoids staleness (engines version-bump monthly).
- **Forecloses:** C → spec sections resting on unprobed assumptions — the exact failure class this campaign exists to prevent.

### DECISION 2.10 — Close mechanics: launch Wave C
- **Context:** closing G2 unblocks Wave C per CAMPAIGN §4: T14 (worker ontology & domain agents, FULL) + T15 (local-models layer, FULL) + T13 (platform stack, LIGHT). Two concurrent per standing rule; the coordinator appends a G2 addendum to the three briefs before launching (routing in Follow-ups).
- **Options:** A) Launch T14 + T15 now; T13 (LIGHT) into the first freed slot. B) A different order. C) Hold Wave C.
- **Recommendation:** A — the two FULL topics are the long poles; T13 consumes the most G2-settled material and slots naturally later.
- **Forecloses:** nothing; C stalls the last research wave.

### Defaults unless objected (adopted silently at gate close if unflagged; every number is an operator-editable ⚙ setting per G1 rider 1)

1. **SQLite durability (R08-OQ1):** `synchronous=FULL` — power-cut-proof; unmeasurable cost at Sinet's write rate.
2. **Lease/liveness set (R08-OQ2):** heartbeat 60 s; dead-after 5 min; wake grace 120 s; recovery attempts ≤3; interrupted >24 h → finalize-with-card; ask-approval expiry 7 d.
3. **Artifact-claim conservatism (R08-OQ4):** whole-project write claim when a plan can't bound its write-set (serializes more, never overwrites).
4. **Pressure/headroom (R09-OQ1):** background admission stops at 0.7 pressure; background budgets default ≤50% of advertised window capacity, labeled "assumed".
5. **Budget denominator (R09-OQ2):** seed from plan marketing shape (GLM Max ~1,600 prompts/5 h) at the conservative fraction — fires sooner than a calibration week; label honest.
6. **GPU operator-wins at v0 (R09-OQ5):** manual eager-unload switch + GameMode start/end hook; idle-detection auto-pause = post-v0 novel work.
7. **Memory lifecycle set (R11-OQ2):** L1 TTL 90 d; re-verify lessons 90 d / house playbooks 6 mo; ≤2 proposals/task; weekly digest; distillation at ≥3 lessons/topic; per-scope injection budgets enforced.
8. **Vector post-gate (R11-OQ3):** evaluate embeddings only when lexical misses affect ≥5% of tasks or corpus >5,000 entries; embeddings only ever *rank candidates* for trace-manifested selection.
9. **True deletion (R11-OQ5):** content purged + tombstone row (id, dates, "removed at owner request") for audit continuity.
10. **Watchdog/inbox numbers (R12-OQ4):** loop 5×; ping-pong 6 cycles; error-loop 3×; per-run-type silence budgets; daily spend >3× trailing-14-day median (armed after 2 weeks of history); ≤2 flag-now/day target; suppress-twice-proposes-retune.
11. **Retention (R12-OQ5):** run summary at run end (local tier); 6-month compaction horizon; keep-forever = summaries, verdicts, decisions, receipts, routing, drift, benchmark records.
12. **Watchlist executor (R12-OQ3):** Sinet-native feed poller on the ratified scheduler (no new DB); Miniflux pre-registered as fallback; changedetection.io + canary layer regardless.
13. **PDF deliverables (R13-OQ4):** v0 = extracted-text diff only; pixel-overlay lane post-v0.
14. **Binary deliverables (R13-OQ5):** local object dir + hash refs from committed text; LFS only if a binary must be reviewable on GitHub itself.
15. **awesome-harness-engineering (R14-OQ2):** watch both leading referents (`walkinglabs/…`, `ai-boost/…`); drop the row if both stall.
16. **genai-prices integration (R14-OQ5):** vendor the pinned `data.json`; scheduled refresh lands as a *proposal* (price-table drift already triggers freshness re-validation per G1 Def.5); user edits overlay, never overwritten.

## Decisions taken (filled at close)

| # | Decision | Chosen | By | Date | Notes |
|---|---|---|---|---|---|

## Follow-ups spawned

- **Platform problems → spec Known-problems list:** P-T07-1..5; P-T08-1..5; T09's two (config-poisoning-as-escape-surface, allowlisted-egress-exfil); T10's three (engine-memory drift = 8.1 bypass, proposal-noise economics, knowledge-injection budget pressure); P-T11-1..5; P-T12-1..4; P-T16-1..4.
- **Wave C brief addenda (coordinator, before launch):** **T13** ← P-T11-4 adoption criteria (permissive license, pinned, record-in-Sinet's-DB — tools are runners never stores), report 14 §3.3 frontend shortlist + §6.4 license-audit rule + P-T16-1 component-onboarding checklist; **T14** ← config-poisoning problem (permission schema owner), P-T16-4 no-public-registry-imports, SAW S1 palette steer (tool+context bundles first, never a standing formation), N22 sanity level; **T15** ← GPU-broker interface (R10-OQ4), VRAM-ledger calibration (R09-OQ8), three new local-model duties with acceptance bars (R12-OQ6: watchdog disambiguation, watchlist triage, canned-query intent-filling; Layer-2 SQL at 14–32B as stretch), contradiction-screen quality (R11-OQ7), local entailment (G1 Def.2 carry-over).
- **P2-entry spike battery** per Decision 2.9; operator prerequisite: provision the Z.AI key into opencode (S3 report §Blocked steps).
- **11.2 benchmark question pre-registered:** anchored `[F#]` findings vs file-level notes on retry quality (R13-OQ6).
- **Adapter-time spikes carried:** Claude-lane auto-memory containment (R11-OQ6); engine memory features join the S2.8 canary suite; S5 PreCompact spike at P3 (G1 D1.6).
- **Standing reminder (operator-requested):** R06-OQ2 native micro-fanout resurfaces when the adapter spawning section is drafted at P2.
- **Watchlist adds (T11 cadence):** fine-grained-PAT collaborator gap; Free-plan ruleset availability; diffity maturity; jj compat matrix; X-Wing age-CLI plugin status; DBOS TS/SQLite parity; MCP 2026-07-28 final (Tasks/Extensions); Litestream #1083; Python-Redlines + pandiff license/maintenance verify at the spec-time dependency audit (R13-OQ7).
