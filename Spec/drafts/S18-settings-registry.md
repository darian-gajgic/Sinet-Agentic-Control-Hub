## S18 — Settings-registry index

**Scope:** The single consolidated index of every operator-editable ⚙ setting introduced in S01–S16 — one row per setting, grouped by domain prefix, deduplicated to its introducing section — plus the reconciliation log of naming/dedup decisions the sweep executed.
**Binding inputs:** G1 rider 1 (settings-not-constants) · G3 Def.2 (`settings.changed` event) · S01.10 (registry architecture — never restated here [XREF:S01]) · the ⚙ tables and section bodies of S01–S16 (the swept corpus) · `Research/decisions/GATE-{1,2,3}` for ratification cross-checks · BENCH-REG §1/§4.1 via [XREF:S14].

### S18.1 What this index is, and how to read it

This section is an **index, not an owner**: every setting below is normatively defined by its introducing section; a row here never overrides its owning row. The registry architecture — declare-once in code, JSON-Schema emission, `/etc/sinet` bootstrap vs DB-row overrides, `settings_events` + `settings.changed` audit on every write — is S01.10's [XREF:S01]; the editing surface is S15.9's [XREF:S15]. Per G1 rider 1, **every ⚙ number ships as an operator-editable setting with audit trail; automation may move *value* only within operator-set `(floor, ceiling)` bounds; auto-raises are visible on receipts** [G1 rider 1; S01.10].

Reading rules:

- **Dedup rule.** A setting introduced in one section and consumed in others appears **once**; owner = the introducing section (group heading). Cross-section consumers are noted in the row. Two source tables self-flagged rows for this sweep (S02's † freshness rows; S05's † `pressure.cache_read_weight`); their dispositions are S18.4 R3–R4.
- **Columns:** `⚙ key` (dotted, per owning domain) · `default` (with unit as ratified) · `clamp / range` (the registry `(floor, ceiling)` or enum; scope qualifiers as stated by the owner) · `R` = restart-required (✓; blank = live-apply — the S01.10 flag, badged in the UI [XREF:S15]) · `auto` = `val` where the owning section states automation moves the value within bounds (G1 rider 1); blank = operator-only writes · `ratified by` (provenance as carried by the owning table).
- **Map-valued keys** (`watchdog.silence_budget.<run_type>`, `local.alias.<duty>`) count as one entry each, as their owners count them.
- **Count:** **118 dotted registry keys** across **33 domains** and 16 owning sections (S18.5 tallies), plus **3 data-valued settings surfaces** with no dotted key (S18.3).

### S18.2 The index, by domain

**S01 — `shell.` (4)** [XREF:S01]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `shell.drain_grace` | 15 min | (1 min, 24 h) [coordinator-draft] | | | G1 Def.7 |
| `shell.watchdog_sec` | 30 s [coordinator-draft] | (10 s, 300 s) [coordinator-draft] | | | R17 §4.1 (⚙ flagged unnumbered) |
| `shell.inhibit_delay_max` | 30 s (host-measured; logind stock 5 s) | (5 s, 60 s) [coordinator-draft] | ✓ | | R17 §4.9; SPIKE P2-S2 |
| `shell.journal_max_use` | 4 GB [coordinator-draft] | (512 MB, 32 GB) [coordinator-draft] | ✓ | | R17 §4.8 (⚙ flagged unnumbered) |

**S02 — `state.` · `recovery.` · `effects.` · `claims.` · `freshness.` (14)** [XREF:S02]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `state.synchronous` | `FULL` | {FULL, NORMAL} | | | G2 Def.1 |
| `state.busy_timeout` | 5 s | ≥ 5 s | | | R08 §4.1 |
| `state.wal_truncate_interval` | 1 h | 5 min – 24 h | | | R08 §4.1 |
| `state.event_payload_cap` | 64 KB | 4 KB – 1 MB | | | R08 §4.1 / P-T07-5 |
| `recovery.heartbeat` | 60 s | 15 s – 5 min | | | G2 Def.2 |
| `recovery.dead_after` | 5 min | ≥ 2× heartbeat | | | G2 Def.2 (seeds `watchdog.silence_budget.*` [XREF:S14]) |
| `recovery.wake_grace` | 120 s | 30 s – 10 min | | | G2 Def.2 |
| `recovery.max_attempts` | 3 | 1 – 5 | | | G2 Def.2 |
| `recovery.stale_finalize` | 24 h | ≥ 1 h | | | G2 Def.2 |
| `recovery.sweep_interval` | 5 min | 1 – 30 min | | | R08 §4.4 |
| `effects.approval_expiry` | 7 d | 1 h – 30 d | | | G2 Def.2 / R08 §4.5 |
| `claims.default_write` | whole-project | {declared-set, whole-project} | | | G2 Def.3 |
| `freshness.max_age` | 24 h | ≥ 1 h | | | G1 Def.5; feature 4.3 — cross-cutting; consumers S03/S06/S10; **alias folded: `intake.approval_stale_hours` (S06 ⚙ table)** — S18.4 R2 [coordinator-draft] |
| `freshness.hold_vs_park` | 10 min | 1 – 60 min | | | G1 Def.4 — cross-cutting; consumer S10 (cache-cliff resume timing) |

**S03 — `adapter.` (3)** [XREF:S03] — engine version pins are deliberately *not* rows: manifest home + audit trail = `components.lock` (S18.4 R9) [XREF:S16]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `adapter.claude.cleanup_period_days` | 365 d | integer ≥ 30; MUST exceed ask-expiry (7 d, G2 Def.2) | | | P-T02-1 mitigation; SPIKE G1-S2 F5 |
| `adapter.engine_ceiling_backstop_mult` | 2 | ≥ 2.0 | | | SPIKE G1-S2 F4 — consumer S10 (ceiling ordering) |
| `adapter.parallel_gate_fallback` | `serialize-by-deny` | {serialize-by-deny, hold-process} | | | SPIKE G1-S2 §Verdict; P2-S4 headline; [coordinator-draft] |

**S04 — `orchestration.` (9)** [XREF:S04] — the registry stores `(value, floor, ceiling)` for every row per S04.4; auto-movement is stated for the three marked rows

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `orchestration.depth_cap` | 2 | 0–2 at v0; >2 = deliberate operator ceiling change | | | D6; G1 Def.8 |
| `orchestration.max_concurrent_helpers` | 4 per task (all depths) | 0–ceiling | | val | G1 Def.8 + rider 1 |
| `orchestration.helper_turns` | 20 per helper | 1–ceiling | | val | G1 Def.8 + rider 1 |
| `orchestration.helper_tokens` | 80,000 per helper | within ceilings | | val | G1 Def.8 + rider 1 |
| `orchestration.spawn_budget` | 8 per task (incl. sub-helpers + retries) | overrun only via operator-visible gate | | | G1 Def.8; R06 §4.3 |
| `orchestration.report_tokens` | 2,000 per report | screen-enforced (S04.5) | | | G1 Def.8 |
| `orchestration.bulk_offload_tokens` | 20,000 | — | | | R06 §4.2; G1 D1.1 |
| `orchestration.helper_retry_limit` | 1 | 0–ceiling | | | R06 §4.4; G1 D1.1 |
| `orchestration.stagger_identical_prefix` | on | {on, off} | | | R06 §4.2; G1 D1.1 |

**S05 — `context.` (4)** [XREF:S05] — S05's fifth table row, `pressure.cache_read_weight` †, is owned by S10 (S18.4 R3)

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `context.stage_fit_target` | 0.50 of lane window | 0.20–0.70; < overflow threshold [coordinator-draft clamp] | | val | G1 Def.11 (per-generation recalibration, S05.2) |
| `context.stage_overflow_threshold` | 0.70 of lane window | (target+0.05)–0.85 [coordinator-draft clamp] | | val | G1 Def.11 (same recalibration) |
| `context.conventions_max_lines` | 150 | 50–400 [coordinator-draft clamp] | | | R07 §4.4 |
| `context.recitation_interval_turns` | 10 | 5–50; 0 = off | | | [coordinator-draft] |

**S06 — `intake.` (7)** [XREF:S06] — an eighth table row, `intake.approval_stale_hours`, is folded into `freshness.max_age` (S18.4 R2)

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `intake.zero_interaction_cost_usd` | $0.50 API-equivalent | ≥ 0; per-user | | | G1 P11 + D1.7 (⚙ per-user at the gate) |
| `intake.clearance_floor.low` | 60 | 0–100 | | | G1 P8 mechanism; default [coordinator-draft] |
| `intake.clearance_floor.standard` | 75 | 0–100 | | | G1 P8 mechanism; default [coordinator-draft] |
| `intake.clearance_floor.high` | 90 | 0–100 | | | G1 P8 mechanism; default [coordinator-draft] |
| `intake.size_recheck_factor` | 2.0× | > 1.0 | | | feature 2.5; R03 §4 Stage 2(c); default [coordinator-draft] |
| `intake.coverage_autofix_rounds` | 1 | 0–2 | | | R03 §4 Stage 2(a) ("bounded") |
| `intake.critique_revise_rounds` | 1 | 0–2 | | | R03 §4 Stage 3 ("fixes once") |

**S07 — `verification.` (11)** [XREF:S07] — the SLA cadence rows are inbox-wide: consumers S15 (inbox display/re-nag) + S14 (notifier delivery); naming resolved at S18.4 R1

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `verification.rework_rounds` | 3 | 0–ceiling | | | R04 §4 via G1 D1.1(d); S07 resolution note stands |
| `verification.convergence_patience_rounds` | 2 | 1–`rework_rounds` | | | R04 §2.6/§3-D via G1 D1.1(d) |
| `verification.sanity_stakes_floor` | standard | tier enum (S06.4) | | | G1 D1.2 mechanism; default [coordinator-draft] |
| `verification.entailment_sample_rate` | TBD-BRINGUP → derived 0.20 (A8, 2026-07-23); live write pending bring-up | 0–1 | | | G1 Def.2; G3 Def.4/Def.8 |
| `verification.research_rerun_limit` | 1 | 0–2 | | | feature 1.9; R03 §4 Stage 2(d) via S06.6 |
| `verification.check_audit_interval_days` | 90 d | > 0 | | | P-T06-1; R04 §4; default [coordinator-draft] |
| `verification.canary_interval_hours` | 24 h | > 0 | | | G1 Def.3 (superseded set: "canary daily") — dead-man escalation canary; see S18.4 R5 |
| `verification.drill_interval_days` | 90 d | > 0 | | | G1 Def.3 (superseded set: "drill quarterly"); see S18.4 R6 |
| `verification.card_remind_hours` | 4 h | > 0 | | | G1 Def.3 (superseded set) — inbox-wide |
| `verification.card_push_hours` | 24 h | > 0 | | | G1 Def.3 (superseded set) — inbox-wide |
| `verification.safety_reping_hours` | 1 h | > 0 | | | G1 Def.3 (superseded set) — inbox-wide |

**S08 — `workers.` (4)** [XREF:S08]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `workers.first_n` | 3 | integer ≥ 1; count-based; resets on body/equipment version change | | | G3 D3.4 |
| `workers.gap_proposal_count` | 2 | integer ≥ 2 | | | G3 D3.4 ("second occurrence") |
| `workers.persona_lines_max` | 2 | integer ≥ 0; station-1 lint warn threshold | | | R15 §4.2 (⚙ there) |
| `workers.dryrun_cost_cap_usd` | $0.50 API-equivalent | > 0 (D5 currency) | | | R15 §4.3 (⚙ there); anchor to G1 D1.7 [coordinator-draft] |

**S09 — `memory.` (12)** [XREF:S09]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `memory.l1_ttl_days` | 90 d | 7–365 | | | G2 Def.7 (L1 scope activates v1) |
| `memory.reverify_lessons_days` | 90 d | 30–365 | | | G2 Def.7 |
| `memory.reverify_house_days` | 180 d (6 mo) | 90–730 | | | G2 Def.7 |
| `memory.proposals_per_task_max` | 2 | 0–5 | | | G2 Def.7 |
| `memory.digest_interval_days` | 7 d | 1–30 | | | G2 Def.7 |
| `memory.distill_threshold_lessons` | 3 | 2–10 | | | G2 Def.7 |
| `memory.injection_budget_tokens.house` | 2,000 tok | 500–10,000 | | | budgets G2 Def.7; numbers [coordinator-draft] |
| `memory.injection_budget_tokens.project` | 3,000 tok | 500–10,000 | | | as above |
| `memory.injection_budget_tokens.user` | 1,500 tok | 500–10,000 | | | as above |
| `memory.injection_budget_tokens.worker_overlay` | 1,500 tok | 500–10,000; scope activates v1 | | | as above |
| `memory.vector_gate.task_miss_rate` | 0.05 | 0.01–0.25; pre-registered trigger, evaluated post-15.3 | | | G2 Def.8 |
| `memory.vector_gate.corpus_entries` | 5,000 | 1,000–50,000; pre-registered trigger | | | G2 Def.8 |

**S10 — `pressure.` · `budget.` · `meter.` · `limit.` · `scheduler.` · `arbitration.` (9)** [XREF:S10]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `pressure.cache_read_weight` | 0.1× | 0–1; labeled "assumed" until provider publishes quota semantics | | | G1 Def.10 — consumer S05 (weighting display); dedup S18.4 R3 |
| `pressure.bg_admit_stop` | 0.7 | 0.1 – 0.95 | | | G2 Def.4 |
| `budget.background_window_fraction` | 0.5 | 0 – 1; labeled "assumed" | | | G2 Def.4/5 |
| `meter.value_divergence_alarm` | 20 % | 5 – 100 % | | | R09 §4.1 / P-T08-1 |
| `limit.retry_cap` | 3 | 1 – 5 | | | G2 D2.1 / R09 §4.3 |
| `limit.retry_budget_ratio` | 0.10 | 0.01 – 0.5 | | | G2 D2.1 / R09 §4.3 |
| `limit.probe_interval_max` | 30 min | 1 – 120 min | | | G2 D2.1 / R09 §4.3 |
| `scheduler.missed_slot_default` | run-once-late | {run-once-late, skip, notify-only}; ships v0, consumer surface v1 | | | R09 §4.7 |
| `arbitration.background_cpuweight` | idle | systemd CPUWeight; slice mechanism [XREF:S01] | | | R09 §4.9 |

**S11 — `sandbox.` (4)** [XREF:S11] — seccomp-BPF profile, Landlock ruleset, per-class profile defaults are structural, not ⚙ (S18.4 R9)

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `sandbox.egress_deny_cidrs` | {169.254.169.254/32, 169.254.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7} | editable list; metadata IP is a non-removable floor [coordinator-draft] | | | R10 §2.3/§3.3 |
| `sandbox.block_outbound_doh` | true | {true, false}; false requires a recorded reason | | | R10 §2.3 |
| `sandbox.c2_registry_allowlist` | curated npm/pypi/crates/apt/go/maven/nuget/rubygems + CDN hosts (Copilot/Codex preset) | editable list (data, like the price table) | | | R10 §3.3 |
| `sandbox.model_egress_tls_terminate` | true (per lane) | {true, false}; false ⇒ pattern-2 scoped-egress fallback for that lane | | | SPIKE P2-S3 / R10 §3.4 |

**S12 — `local.` (11)** [XREF:S12]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `local.ttl.fast_s` | 120 s | 0–3600 [coordinator-draft clamp] | | | R16 §4.1 |
| `local.ttl.workhorse_s` | 300 s | 0–7200 [coordinator-draft clamp] | | | R16 §4.1 |
| `local.ttl.egpu_s` | 1800 s | 0–86400; dormant until pool24 | | | R16 §4.1 |
| `local.vram.guard_band_mb` | 512 MB | ≥ 128 [coordinator-draft clamp] | | | R16 §4.4 step 5 |
| `local.unload.term_grace_s` | 5 s | 1–30 [coordinator-draft clamp] | | | R16 §4.5 |
| `local.gamemode_hook` | on | {on, off} | | | G2 Def.6 |
| `local.battery.gpu_admission` | urgent-only | {never, urgent-only, always}; name [coordinator-draft] | | | R16 §4.6 |
| `local.batch.ac_only` | true | {true, false} | | | R16 §4.6 |
| `local.broker.sandbox_logprobs` | off | {off, on}; per-template | | | R16 §4.7 |
| `local.reeval.cadence_months` | 6 mo | 1–12 | | | R16 §4.10 |
| `local.alias.<duty>` (map) | per S12.4 table; workhorse = Qwen3.5-9B | changes only via the S12.10 swap gate | | | R16 §4.7; G3 Def.8 |

**S13 — `review.` · `backup.` · `preview.` (7)** [XREF:S13]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `review.anchor_drift_lines` | ±2 lines | 0 – 10 [coordinator-draft clamp] | | | FC-v1 §2 (carried N15 behavior) |
| `backup.interval` | 24 h (daily) | 6 h – 7 d [coordinator-draft clamp] | | | G2 D2.5 |
| `backup.keep` | 30 snapshots | 7 – 365 [coordinator-draft clamp] | | | G2 D2.5 |
| `backup.repo_rotation` | 12 mo | 6 – 24 mo [coordinator-draft clamp] | | | G2 D2.5 (annual) |
| `backup.drill_interval` | 3 mo [coordinator-draft] | 1 – 12 mo | | | R13 §4.9 (⚙ flagged unnumbered); see S18.4 R6 |
| `preview.idle_stop` | 15 min [coordinator-draft] | 1 min – 24 h | | | R13 §4.8 (⚙ flagged unnumbered) |
| `preview.max_concurrent` | 3 [coordinator-draft] | 1 – 10 | | | feature 3.11 posture [coordinator-draft] |

**S14 — `obs.` · `watchdog.` · `watchlist.` · `canary.` · `retention.` · `eval.` · `benchmark.` (15)** [XREF:S14]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `obs.sse_keepalive` | 20 s | 15–30 s | | | R12 §4.3 |
| `watchdog.loop_repeat` | 5× | 3–10 | | | G2 Def.10 |
| `watchdog.pingpong_cycles` | 6 cycles | 3–12 | | | G2 Def.10 |
| `watchdog.error_loop` | 3× | 2–10 | | | G2 Def.10 |
| `watchdog.silence_budget.<run_type>` (map) | seed = `recovery.dead_after` (5 min) | ≥ 1 min; per-type; TBD-BRINGUP(per-run-type calibration) | | | G2 Def.10 |
| `watchdog.spend_median_mult` | 3× trailing-14-d median | ≥ 1.5× | | | G2 Def.10 |
| `watchdog.spend_arm_days` | 14 d | ≥ 7 d | | | G2 Def.10 |
| `watchdog.flag_now_target` | 2/day | 1–10 | | | G2 Def.10 |
| `watchdog.suppress_retune_count` | 2 | 1–5; retune lands as a proposal card, never an auto-move | | | G2 Def.10 |
| `watchlist.fetch_fail_streak` | 3 cycles | 2–10 | | | R12 §4.5 / P-T11-3 |
| `canary.auth_interval` | daily | 4/day – weekly | | | G1 Def.3 set; P-T17-1 — auth-revocation canary; see S18.4 R5 |
| `canary.behavioral_interval` | weekly | daily – monthly | | | R12 §4.5 [coordinator-draft] |
| `retention.compaction_horizon` | 6 months | ≥ 1 month; per-user | | | G2 Def.11; feature 13.4 |
| `eval.sweep_interval` | quarterly | monthly – semi-annual | | | R12 §4.7 |
| `benchmark.sampling_rate` | BENCH-REG §4.1 schedule (100% pre-gate / 25% maintenance) | 0–100%; uniformity frozen | | | BENCH-REG §4.1 — registered-number rule applies (S18.4 R9) |

**S15 — `frontend.` (1)** [XREF:S15]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `frontend.dependency_pass_interval` | 90 d (quarterly) | (30 d, 365 d) [coordinator-draft] | | | G3 D3.3 rider; R17 §4.3; see S18.4 R7 |

**S16 — `adoption.` (3)** [XREF:S16]

| ⚙ key | default | clamp / range | R | auto | ratified by |
|---|---|---|---|---|---|
| `adoption.dependency_pass_months` | 3 mo (quarterly) | (1, 6) [coordinator-draft] | | | G3 D3.3 rider; R17 §4.8; see S18.4 R7 |
| `adoption.abandonment_months` | 6 mo [coordinator-draft] | (2, 24) [coordinator-draft]; per-entry overridable | | | R17 §4.8 (⚙ flagged unnumbered); P-T16-1 |
| `adoption.price_data_stale_days` | 60 d | (14, 180) [coordinator-draft] | | | R14 §4.2 |

### S18.3 Data-valued settings surfaces (13.4-editable, no dotted key)

Three operator-editable surfaces are **tables/flags, not single registry keys** — their owners declare them settings ("data, not a single default") and they follow the same audit pattern as registry writes [S01.10; XREF:S10]:

| Surface | Shape | Owner | Ratified by |
|---|---|---|---|
| **Price table** | effective-dated per-model rows; vendored genai-prices defaults; user edits overlay, refresh-as-proposal | S10.3 | D5; G2 Def.16 |
| **Per-person automation budgets** | per (person, lane, period); seeded from plan marketing shape, labeled "assumed" | S10.4 | G2 Def.5 |
| **Per-model attributes** | flat/metered flag (per user, feature 3.1); `overflow_mode` {hard-stop, opt-in-credits, auto-metered}; `region_model_gate` observed-vs-config diff state | S10.2 (flag); S03.6 (attribute pair) | D5; R02 §4; P-T17-3 |

These carry no dotted names by design (row-keyed data); the settings UI surfaces all three [XREF:S15]. No index rows are minted for them.

### S18.4 Reconciliation log (decisions this sweep executed)

- **R1 — Inbox SLA naming seam (the S07-recorded duty): RESOLVED, no rename.** S07 registers the escalation SLA cadences under `verification.*` (card remind 4 h / push 24 h / safety re-ping 1 h, with the daily canary and quarterly drill from the same G1 Def.3 superseded set) and says "the S18 sweep reconciles naming"; S15 re-states the set with "cadence ⚙ and delivery owned by the notifier [XREF:S14]". No competing dotted name exists anywhere in the corpus, and the contract binds a key's prefix to its introducing section's domain — so the canonical names stay `verification.card_remind_hours` / `verification.card_push_hours` / `verification.safety_reping_hours`, owner S07, with **scope: inbox-wide** carried on the rows; S15's phrase is read as *delivery* ownership (the notifier), not registry ownership [coordinator-draft].
- **R2 — `intake.approval_stale_hours` ≡ `freshness.max_age`: FOLDED (one setting, alias recorded).** Same concept (the feature-4.3 "assumptions may be stale" horizon), same default (24 h), same ratification (G1 Def.5) at two enforcement points — S02.6's parked-run freshness pass and S06.9's pending-card flag. S02's own † footnote calls the setting cross-cutting and "enforced via S06", and both sections delegated dedup to this sweep. Canonical key: `freshness.max_age`, owner S02; `intake.approval_stale_hours` (S06 ⚙ table) is recorded as its alias and mints no separate key [coordinator-draft]. If G4 wants the two enforcement points independently tunable, the alias becomes a second key — flagged for that attention.
- **R3 — `pressure.cache_read_weight` dual listing: deduped to S10** as S05's own † directs ("owned by the pressure gauge"). One row, owner S10, consumer S05.
- **R4 — `freshness.*` rows listed in both S02 (†) and S10's consumed-elsewhere note: owner S02** (the freshness-pass and fingerprint mechanics live in S02.6); consumers noted on the rows. Executes the dedup both tables requested.
- **R5 — Two canaries, not one.** `verification.canary_interval_hours` (S07: dead-man escalation-path canary, alert-on-silence) and `canary.auth_interval` (S14: provider auth-revocation canary, P-T17-1) both default to "daily" and both cite G1 Def.3's close set — distinct duties, deliberately separate keys; no merge. `canary.behavioral_interval` (weekly) is a third, unrelated cadence.
- **R6 — Two drills, not one.** `verification.drill_interval_days` (G1 Def.3 quarterly escalation drill) vs `backup.drill_interval` (R13 §4.9 restore drill). Distinct; no merge.
- **R7 — Dependency-pass siblings, not duplicates.** `frontend.dependency_pass_interval` = 90 d (S15) and `adoption.dependency_pass_months` = 3 mo (S16) both cite the G3 D3.3 rider and both mean "quarterly"; S15 itself says the passes SHOULD be co-scheduled with S16's manifest review. Two deliberate keys for two activities (SPA dependency bumps vs `components.lock` review); the day-vs-month unit heterogeneity is noted as cosmetic.
- **R8 — Settings-shaped inline items that are NOT registry keys.** S10's consumed-elsewhere list names per-run `runs.ceilings` [S02.2] and `RuntimeMaxSec` [S01.2]: `runs.ceilings` is a per-run column set (time/steps/cost, feature 3.7) seeded at dispatch, and `RuntimeMaxSec` is the transient-unit directive carrying the run's time ceiling — per-run enforcement data, not operator dials; no rows minted. This disposition completes S10's dedup request.
- **R9 — Deliberate non-rows, confirmed by their owners.** Engine version pins (`claude` 2.1.214, `opencode-ai@1.18.3`) — operator-editable via the S03.3 deliberate-bump procedure, manifest home `components.lock` [XREF:S16]. Sandbox seccomp/Landlock/per-class profiles — structural, versioned, not dials [S11]. Benchmark frozen values (gate limbs, alarm threshold, done-directly minimum, labels) — surface in the registry marked **"registered — changing this value requires a re-registration commit"** [BENCH-REG §1; S14]; only `benchmark.sampling_rate` is a live dial.
- **R10 — Naming observations (cosmetic; no renames proposed).** Unit-suffix style is heterogeneous (`_s`/`_mb`/`_days`/`_hours`/`_months`/`_usd` in `local.*`, `memory.*`, `verification.*`, vs unitless keys elsewhere with units in the default) — the registry's typed declarations [S01.10] make this cosmetic. Multi-level keys (`adapter.claude.*`, `intake.clearance_floor.*`, `memory.injection_budget_tokens.*`, `memory.vector_gate.*`, `local.ttl.*`) and the two map-valued families are consistent with declare-once. Domain→owner is many-to-one for S02, S10, S13, S14; no domain prefix straddles two owners — the CONTRACT prefix rule holds corpus-wide.

### S18.5 Exhaustiveness

Per-owner key counts: S01 4 · S02 14 · S03 3 · S04 9 · S05 4 · S06 7 · S07 11 · S08 4 · S09 12 · S10 9 · S11 4 · S12 11 · S13 7 · S14 15 · S15 1 · S16 3 = **118**.

Per-domain: shell 4 · state 4 · recovery 6 · effects 1 · claims 1 · freshness 2 · adapter 3 · orchestration 9 · context 4 · intake 7 · verification 11 · workers 4 · memory 12 · pressure 2 · budget 1 · meter 1 · limit 3 · scheduler 1 · arbitration 1 · sandbox 4 · local 11 · review 1 · backup 4 · preview 2 · obs 1 · watchdog 8 · watchlist 1 · canary 2 · retention 1 · eval 1 · benchmark 1 · frontend 1 · adoption 3 (33 domains).

**Sections with no ⚙:** none — every committed section S01–S16 introduces at least one; S00 introduces none by design. S17 and S19 are not yet drafted and are expected to introduce none (register and boundary sections); any section amendment before assembly that touches a ⚙ table or adds an inline ⚙ re-runs this sweep as part of the S00.9 assembly step [coordinator-draft].

**Dormant-at-v0 keys ship in the registry with their ratified defaults** (G1 rider 1 applies from day one): `scheduler.missed_slot_default` (consumer surface v1), `local.ttl.egpu_s` (pool24), `memory.injection_budget_tokens.worker_overlay` (L1/overlay scope v1), `memory.vector_gate.*` (trigger evaluated post-15.3).

---

**Settings introduced (⚙):** none — this section is the index; every entry above is owned by its introducing section.

**Known problems owned here:** none — P-* ownership stays with the owning sections (P-* ids appearing in provenance cells are citations, not ownership); S17 consolidates the register.

**Deferred / parked:**

- Dormant-key activations (S18.5 list) → triggers: v1 schedule surface [XREF:S10/S19]; pool24 [XREF:S12]; v1 memory scopes [XREF:S09]; post-15.3 vector gate [XREF:S09].
- Independent tunability of the two R2 enforcement points (parked-run vs pending-card staleness) → re-entry: G4 objection to the fold, or operator demand for split thresholds post-bring-up.
- Re-sweep on amendment → the S00.9 assembly step (S18.5).

**Coverage:**

| Feature-list item | Where |
|---|---|
| 13.4 every number an operator-editable setting — the consolidated index | S18.2 (dotted keys), S18.3 (data surfaces) |
| G1 rider 1 mechanics visible in one place (bounds, auto-movement, audit) | S18.1 legend; `auto` column; [XREF:S01] |
| 4.3 staleness threshold single-sourced | S18.4 R2 |
| Sweep exhaustiveness (provably complete over S01–S16) | S18.5 |

**Open items for G4:** none. Sweep resolutions tagged [coordinator-draft] for G4 attention: R1 (keep `verification.*` names for the inbox-wide SLA set), R2 (fold `intake.approval_stale_hours` into `freshness.max_age`, alias recorded, split path parked), and the assembly re-sweep rule (S18.5).
