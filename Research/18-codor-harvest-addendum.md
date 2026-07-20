# 18 — Codor harvest addendum: what to copy, what not to copy

> **Post-campaign addendum (2026-07-20), operator-requested.** Not a campaign topic and not a gate input — the campaign closed at G4 and `Spec/core-architecture-v1.md` is frozen. This file analyzes an external project against the frozen spec and P3 build state. Nothing in it changes the spec by itself; every row states whether acting on it is (a) already inside the spec, (b) an implementation-detail pattern for a pending B-phase, or (c) an S00.9 amendment the operator would have to approve.

## Scope

Analyzed: **Codor** (`https://github.com/rjx18/codor`) — a self-hosted "switchboard" daemon that puts multiple coding-agent harnesses (Claude Code, Codex, Gemini, Copilot, OpenCode) into shared chat-style channels as persistent, addressable members. Touches Sinet territory in S02 (durability), S03 (engine adapters), S04 (orchestration), S05 (context/recitation), S09 (memory), S10 (budgets), S14/S15 (observability, frontend, push), S16 (adoption discipline).

Method: full clone at commit `305670fb554` (head, 2026-07-20); line-level reads of the switchboard daemon/store/router, the claude-code and codex adapters, CLI, hooks, and all design docs; compared against the canonical spec drafts S00–S19 and `P3/STATE.md`. Codor file references below are `path:line` at that commit.

## 1. What Codor is

Codor's own framing: *"the chat channel where harness sessions are first-class members."* One Node/TypeScript daemon per machine owns channels (SQLite + JSONL run blobs), routes `@mention` messages into harness sessions as turns, journals normalized run events, renders everything in a switchboard-served PWA (plus CLI), and pushes sealed notifications through a self-hostable relay. Agents address each other; chains run until done; opt-in "brakes" can hold runaway chains for a human. Custody of a session moves both ways between the daemon and a human terminal (`codor join`/`adopt`/`attach`). Per-channel shared memory is an Obsidian-compatible markdown vault cited via `[[wikilinks]]`. It is explicitly **not** an agent framework: *"it never prompts, plans, or holds model context… Codor is the wire between them"* (`docs/VISION.md`).

**Provenance and maturity (verified 2026-07-20):** MIT, single author (Richard Xiong), public on GitHub only since **2026-07-19** (139★/21 forks in the first day); 0.1.0 changelog dated 2026-07-11; README carries an alpha warning. Despite the age, the engineering discipline is exceptional: ~1:1 test-to-source ratio in the core daemon, adversarial crash-matrix tests (kill-point boot-reconcile matrix, FIFO order proven against adversarial UUID ordering, byte-exact payload goldens), zero TODO/FIXME anywhere in `packages/`, recorded-stream fixtures with a "never hand-edit a fixture — re-probe" rule, and doc↔code traceability tags (`harn:assume`) throughout. Honest gaps it documents itself: Copilot/Gemini adapters are docs-derived (no live captures), the Codex live smoke wasn't run, remote interrupt/ask is unimplemented, and there is **no retention story** for messages/blobs/change-log. Verdict on trust: a serious, dogfooded system — but one day public, one maintainer, alpha. **Study source only; never a dependency.**

## 2. The frame: why Sinet cannot adopt Codor — and why mechanisms still transfer

Two structural blocks, before any row-by-row:

1. **Opposite topology by ratified decision.** Codor's product *is* agent↔agent lateral messaging (mention routing, chains, group rounds). Sinet's **D6** mandates a single-coordinator tree with **no lateral messaging at any depth** — a conformance test literally attempts a lateral send and asserts refusal (S04.1) — backed by the S16.6 anti-harvest evidence row (17.2× vs 4.4× error amplification). Codor's own operating history argues *for* D6: two of its live agents entered an endless empty-acknowledgement loop during M0 acceptance (`docs/PROTOCOL.md` §3), and it had to invent an `<ACK_OK>` marker protocol, empty-run no-route rules, hop-count brakes, and group-round barriers to keep chains governable. Sinet deleted that whole hazard class by decision.
2. **Organ-grade rule.** S16: *"Nothing spine-shaped (session-owning, board-owning, orchestration-owning) is ever adopted."* Codor is precisely a spine — session-owning and routing-owning. So "adopt Codor" is structurally excluded regardless of quality; only patterns, probed engine facts, and test shapes transfer.

The products are complementary, not competing: Codor is deliberately *only the wire* between interactive sessions; Sinet is the factory (intake→spec→plan→execute→verify→receipt) that Codor explicitly declines to be. That is why the copy list below is mostly mechanisms at the engine seam, and the don't-copy list is mostly Codor's core product.

## 3. COPY — take these, and why

| # | What | Mode | Lands | Spec impact |
|---|---|---|---|---|
| C1 | PostToolUse "quiet inbox" hook = the missing per-turn channel (recitation, cancel-notice, operator notes) | PATTERN | B3 (S04/S05 seam), pre-spiked | none — mechanism for an already-ratified ⚙ |
| C2 | Serve-side secret redaction + redact-before-match search + bounded evidence excerpts | PATTERN | B5/B6 (S14 query layer, S15 API) | none (spec-silent, additive) |
| C3 | Probed Claude-engine facts + crash-safe interaction test shapes | STUDY/evidence | S03 conformance suite rows | none |
| C4 | Group/round barrier shape for parallel fan-out | STUDY | post-v0 (S04 deferred mass fan-out) | amendment, later, if ever |
| C5 | Pre-resume context peek from transcript tails | PATTERN (minor) | B3/B4 (S02.5 recovery, stage-fit cross-check) | none |

### C1 — The per-turn channel: PostToolUse hook that injects only when there is something to say

**The gap it fills:** S05.43 ratifies recitation — *"the coordinator re-reads the ledger's `state`/`next_actions` every ⚙ `context.recitation_interval_turns`"* — but B2-4 shipped with that ⚙ **deliberately unconsumed: "no per-turn channel"** exists (P3/STATE.md, 2026-07-20). An agent cannot count its own turns; only the platform can, and until now Sinet had no way to speak into a running session at a safe boundary.

**Codor's proven mechanism:** after **every tool call**, a PostToolUse hook runs a tiny CLI (`codor inbox --new --consume --format hook`) with member-scoped env credentials. If the inbox is empty it prints **nothing**, the hook returns `{}`, and zero context is injected. If something is queued, it prints `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"…"}}` and the content lands mid-turn at the next tool boundary, with the queued items atomically consumed (CAS — single winner vs. the turn pump) so they can never double-deliver. Evidence: `packages/adapters/claude-code/src/adapter.ts:327-353`, `packages/cli/src/program.ts:293-308`, contract in `docs/PROTOCOL.md` ("a quiet inbox emits zero stdout and therefore injects no context"). Running live in their dogfood.

**Why copy:** it is exactly the shape Sinet needs, D6-clean (strictly control-plane→worker, no lateral edge), token-frugal (quiet path costs zero), and it composes with what B1-4 already proved (SessionStart/`--settings` hook injection on the pinned CLI). Uses beyond recitation, all control-plane-authored: soft **cancel-notice at a safe boundary** (a gentler first rung before S03's interrupt ladder), **budget warnings** before a `blocked_budget` park, and the S15 "adjust-with-note" operator note reaching a *running* stage instead of waiting for the next one.

**Sinet-shaping (for the B3 packet cut):**
- Keep it off PreToolUse — that hook is Sinet's **gating/defer-park primitive** (S03.4) and must stay single-purpose. PostToolUse is the natural boundary, same as Codor.
- The hook calls the `sinet` binary (already a single multi-call binary, S01.5) — cheap exec, unlike Codor's Node CLI.
- Companion security pattern (also Codor's): **per-run scoped token**, minted at spawn, hash-only storage, verb allowlist of one or two read/consume verbs, injected via the broker env path; Codor additionally masks the inherited privileged token by aliasing (`daemon.ts:389-411`, `authorization.ts:98-106`). Fits Sinet's broker posture (S11) unchanged.
- One design decision to resolve in the packet: reach path from inside the sandbox (loopback HTTP vs. a mounted unix socket). C2-class network-off stages simply get no channel — acceptable; verification stages don't recite.
- **Pre-registered spike first** (one cheap fixture): confirm on the pinned CLI 2.1.215 that a `--settings` PostToolUse command hook honors `additionalContext` mid-turn the way Codor's SDK-side hook does. Codor's stdout shape *is* the settings-hook JSON contract, so this should pass — but Sinet's rule is probe, don't assume.

### C2 — Projection hygiene: redact at the serving edge, search over redacted text, bounded excerpts

**The gap:** grep of S02/S13/S14/S15 drafts finds **no serve-side redaction** anywhere. Sinet keeps secrets out of run *env* (broker, scrubbed env, CI key-hygiene greps) — but tool output can still capture a secret (an agent `cat`s a key file; a provider echoes a header), and from there it flows into `run_events` → SSE → the B6 UI → household-visible screens.

**Codor's mechanism:** raw content stays in the store/journals; **every** WS frame and REST payload passes through one projection function that deep-walks values and redacts pattern classes (bearer tokens, `sk-…`/`gh?_` keys, AWS key ids, PEM blocks, `SOME_KEY=<value>` pairs, its own pairing codes) — and, critically, **search redacts before matching, so the search endpoint cannot be used as an oracle for redacted text**; run-evidence search returns bounded 240-char excerpts with hard result caps (`packages/switchboard/src/redact.ts:9-65`, `daemon.ts:548-584`, `store.ts:2033-2056`, `daemon.ts:4073-4110`). Raw `raw` payloads are journal-only and stripped from live frames unconditionally.

**Why copy:** cheap, additive, zero behavior change for honest content, and it converts "the broker keeps secrets out" from a single line of defense into two. Lands naturally in the B5 query layer and the B6 API projection. Recommend constant-on (no ⚙, so no amendment); if the operator later wants a toggle under the settings-tab directive, that's one clamped S18 row in the already-planned amendment.

### C3 — Probed engine facts and test shapes for the S03 conformance suite

Codor paid for live probes against Claude Code's control surface that Sinet can take as free, dated evidence (verify at Sinet's own pin before relying):

1. **Re-raised interactions carry fresh native ids.** After a crash-resume, a re-raised ask/approval arrives with new `request_id`/`tool_use_id` and only after a new turn is nudged — correlation must use a semantic key (member, tool, prompt content), never native ids (`docs/PROTOCOL.md` §2 "Probed reality", implemented at `daemon.ts:3376-3406`). Sinet's durable ask-record (P-T02-1) is already authoritative, so this mostly confirms the posture — but the *test shape* (restart-with-pending, restart-answered-but-not-acked) is worth cloning into the S03 conformance suite if Sinet ever holds a native pending interaction across a restart.
2. **Answered approvals are never auto-resent; answered asks replay idempotently.** Same distinction Sinet draws (approvals are effects-adjacent). Their boot-reconcile kill-point matrix (`daemon.spec.ts:1737-2113`) is a ready-made template for extending Sinet's kill-9 harness to interaction boundaries.
3. **Empty/error output must never speak.** Codor: an empty-bodied finalized run never routes (it live-looped two agents); a failed run's diagnostic is stored as `error` evidence and its reply/fanout stay empty — *"provider or process errors [never] speak as the agent"*. Sinet equivalents (report-as-data P-T03-4, S07 quarantine) already exist; keep this as a named negative test in the conformance suite: error text ≠ stage output, ever.
4. **The SDK-flip case study (watch informally).** Codor started on `claude -p` CLI-wrap and **migrated to the Agent SDK** (`query()` streaming, programmatic hooks) — their NOTES record why: the generated `--settings` + loopback-HTTP hook bridge was the pain point (`packages/adapters/claude-code/NOTES.md:128-143`). Sinet decided the opposite for v0 (CLI-wrap; SDK = documented in-adapter fallback with pre-registered flip conditions, S03). Codor is now a live specimen of the fallback path: when the S03 flip decision ever comes up, read their adapter first (`adapter.ts:355-763` — long-lived query, identity-keyed retire/rebuild, pump/turn routing, `canUseTool`→durable cards). No formal S16 watch row — nothing is adopted; just re-visit the repo at that decision point and at the B3 cut.

### C4 — Barrier rounds for parallel fan-out (post-v0 shelf)

S04 defers script-driven mass fan-out (≥50 items) to post-benchmark-gate. When that day comes, Codor's collaboration-group machinery is a clean, DB-backed map-reduce barrier over *unreliable* workers: groups/rounds/participants with terminal statuses `completed/failed/interrupted/skipped`, round N+1 releases only when every participant is terminal, results bundled in stable ordinal order independent of finish order, failed/skipped visible as statuses rather than vanishing (`store.ts:130-163`, `daemon.ts:3247-3319`). Take the *schema and settlement rules*, not the mention-routing around them. Requires an amendment if it ever lands; shelf until then.

### C5 — Pre-resume context peek (minor)

Before resuming a session, Codor tail-scans the harness's own transcript for the last usage record to estimate context occupancy **without spending a token**, marks it `estimated:true`, and never lets an estimate outrank engine-reported numbers (`packages/adapters/claude-code/src/peek.ts:8-133`). Sinet analog: at S02.5 recovery, a peek at Sinet's own transcript **copy-aside** (P-T01-1 — don't depend on the engine store) as a cheap fork-vs-resume input and a cross-check on the stage-fit budget watcher. Small, optional, no spec impact.

## 4. DO NOT COPY — and why

| # | Codor feature | Why not |
|---|---|---|
| N1 | **Chat-channel paradigm**: mention routing, agent↔agent chains, default recipients, fan-out | Contradicts **D6** (no lateral messaging; conformance-tested refusal) and S16.6's error-amplification evidence. Codor's own ack-loop incident and brake inventory are the cautionary tale. Sinet's interaction model is pipeline + gates + cards by frozen decision (S04, S06, S15.6). |
| N2 | **Codor as a component/dependency** | Spine-shaped (session-owning, routing-owning) — S16 forbids adopting spines. Also: 1 day public, single author, alpha, no retention story. Study source only. |
| N3 | **Persistent named sessions as the unit of identity** (`session_ref` anchor, members live for weeks) | Sinet's opposite bet is load-bearing: fresh-context-per-stage (S05.3), platform checkpoint rows as the durable truth, engine transcripts explicitly *not* durable (P-T01-1). Long-lived sessions would resurrect the context-rot and custody problems Sinet designed away. |
| N4 | **Two-way custody / TUI attach** (`join`/`adopt`/`attach`, leases, `custody_uncertain`) | Impressive engineering (fail-closed single-writer leases, `daemon.ts:1325-1493`) but S03.4 is explicit: *"never attach a TUI to a Sinet-owned serve"* — and with ephemeral per-stage sessions there is nothing worth jumping into. Steering is via cards/re-plan by design. Would need an amendment and a real operator need first. |
| N5 | **Agent SDK `query()` as the driver now** | S03's spike verdict stands: CLI-wrap for v0, SDK as documented fallback. Codor's SDK adapter is the case study for *later* (C3.4), not a reason to flip early. |
| N6 | **`[[wikilink]]` refs as memory selection** | Directly violates S09's structural rule: *"no agent-supplied identifier ever selects memory"* — a wikilink in agent output is exactly an agent-supplied selector. Sinet's registry-keyed deterministic injection + FTS5 tool with server-side scope predicate stays. |
| N7 | **Obsidian-vault-shaped memory store** | S09 already decided the store: markdown files in git-versioned dirs + registry rows + FTS5, L0/L1/L2 write gates. Codor's vault has no write gating, no provenance rows, no scope precedence — weaker than what B3 will build. (Free consolation: Sinet's knowledge dirs are plain markdown; the operator can open them in Obsidian read-only any day. No spec change involved.) |
| N8 | **Multi-box residency + hyperswarm DHT transport** | D1: single host, LAN/Tailscale only, no runners on member devices. Sinet's multi-host future is pre-absorbed at the seams (S03.1) and explicitly not v0 design work. The DHT layer also drags in a crypto/key-rotation surface Sinet doesn't need. |
| N9 | **Device-keypair identity + QR/8-char pairing** | S01.9 decided auth: tailnet wall → header hint → server-side sessions + per-user PIN (argon2id); passkeys at v1. Codor needs device keys because it promises E2EE through untrusted relays; Sinet's trust boundary is the tailnet. Revisit only if v1 household onboarding shows pairing friction. |
| N10 | **Sealed-payload push + self-hosted push relay** | S15.11 decided: Declarative Web Push with content-free payloads (navigate + badge) — relays see *"timing, volume, and endpoint metadata — never content"* already, without a relay to run or service-worker crypto. ntfy fallback is pre-registered if the iOS drill fails. Codor solves a problem Sinet's design avoided. |
| N11 | **Slack/Telegram bridges** | Out of scope and against posture: bridging knowingly exports channel content to platform servers (their own banner says so). Sinet's D1 exposure rule and the observables register leave no seat for it. |
| N12 | **Four-tier role lattice** (observer/member/admin/owner) | S01.9/D10: one role bit (operator vs member) + owner-scoped records, *"no policy engine at household n."* Codor's lattice exists for multi-human orgs — Sinet's household model already covers its actual users with less machinery. |
| N13 | **Credential-scoping as the *only* containment** (agents run as the OS user; policy chips are honesty labels) | Sinet runs C0–C2 kernel sandboxes + broker (S11) — strictly stronger. Codor's README admits it: *"they are not a process sandbox."* Nothing to take except the honesty-labeling instinct, which S15's policy surfacing already has. |
| N14 | **Stack choices** (Node/TypeScript daemon, better-sqlite3, zod, Fastify/ws, React-in-daemon) | Sinet's stack is decided and building (Go, modernc sqlite, SSE, embedded SPA). No migration value. |
| N15 | **Turn/spend brakes + `<ACK_OK>` protocol + group-routing conventions trailer** | Solutions to chain-runaway problems Sinet structurally lacks (no chains). Sinet's budget parks (two-gated, per-person, CRITICAL_PLUS interactive) and pause-and-flag watchdogs are the stronger, already-specced equivalents. |
| N16 | **`harn:assume` doc-traceability tagging as a process add-on** | Genuinely clever (code/schema/tests share assumption slugs), but Sinet already has provenance tags in the spec, readings in commit bodies, CONVENTIONS, and gate records — and the operator has declined optional extra process. Noted, not adopted. |

## 5. Convergences — independent validation of frozen decisions (no action)

A one-author project, built in the same season against the same engines, independently landed on a striking number of Sinet's answers. That's evidence the frozen spec sits on real constraint topology, worth recording:

- **Exactly-once-or-held delivery** with attempt-WAL bound before spawn, consume only on journaled completion, and boot reconcile classifying *provably completed / provably never started (retry once) / ambiguous → held for the operator* (`daemon.ts:3518-3663`) ≈ S02.5 recovery ladder + S02.7 two-phase effect journal with `unknown` → decision card, dispatch-id idempotency, harvest.
- **Monotonic ids over timestamps** ("timestamps can tie", durable `queue_seq`; per-channel dense message ids) ≈ S02.2 *"`event_seq` … the sole ordering authority — never timestamps"* (P-T07-4).
- **One seq cursor for client sync** (change-log `since_seq`, re-hydrate changed rows, commit frame) ≈ S14.3/S15.2 snapshot-then-tail SSE with `Last-Event-ID` resume and *"re-snapshot on gap — never patch blind."*
- **Software never kills** — stall watchdog flags `stalled_since` and pushes, *"operators kill, software doesn't"* ≈ D1.3 pause-and-flag, *"auto-kill does not exist anywhere in this suite"* (S14.4).
- **Explicit context transfer, never ambient** — *"if an agent needs to know something, someone must have said it to them"*; bounded delivery payloads; refs resolved at delivery ≈ D7/S05 ledger-assembled fresh contexts and deterministic injection.
- **Errors never speak as the agent**; run diagnostics are evidence, not replies ≈ P-T03-4 report-as-data + S07 quarantine.
- **Poison-pill ceilings** (recovery attempt ceiling 2 so a boot-killing job can't ping-pong the daemon) ≈ ⚙ `recovery.max_attempts` + tombstoning repeat offenders.
- **Honest cost accounting** — tokens-only lanes accumulate `uncosted_tokens`, never guessed into dollars ≈ D5's two-currency wall.
- **Declared-vs-actual policy tests** (a harness must declare what each policy tier becomes natively; tests compare against real argv) ≈ S03 conformance-at-the-pin philosophy.
- **Physical-check runbooks** (`MANUAL-VERIFY.md` for credential-gated/device checks) ≈ `P3/measurements/PROBES.md` operator homework.
- **Files-as-memory instinct** (markdown vault, git-friendly, no DB ownership of content) ≈ S09's *"the row is the selector index, the file is the content, git is the history"* — same instinct, Sinet adds the write gates and provenance Codor lacks.

## 6. Recommendation and open questions

**Recommendation:** copy **C1** (per-turn channel pattern + scoped-token companion, with the small pre-registered PostToolUse spike) into the B3 packet cut, and **C2** (projection hygiene) into B5/B6 acceptance criteria. Take C3 as conformance-suite rows/evidence now, shelf C4/C5. Adopt nothing as a dependency. Everything in §4 stays un-copied with the stated decision anchors.

Open questions for the operator (none block the B2 gate):

1. **OQ1 — C1 into B3?** The coordinator should fold C1 into the B3 packet cut (it consumes the already-ratified recitation ⚙ and the B1-4 injection spike result). Approve treating this file as the B3-cut input for that packet?
2. **OQ2 — C2 posture:** constant-on redaction at the serving edge (no ⚙, no amendment) — or operator-toggleable, which folds into the planned settings-tab amendment as one clamped S18 row?
3. **OQ3 — informal watch:** no S16 watch row is created (nothing adopted). Agree to simply re-read Codor at two future decision points — the S03 CLI→SDK flip (their adapter is the fallback-path case study) and the B3 cut (C1 mechanics)?

## Sources

- Repository: `https://github.com/rjx18/codor`, cloned at head `305670fb554` (2026-07-20); GitHub API metadata accessed 2026-07-20 (created 2026-07-19, MIT, 139★, 21 forks, 3 open issues, not archived).
- Codor internals cited by `path:line` at that commit throughout; design docs: `docs/VISION.md`, `docs/ARCHITECTURE.md`, `docs/PROTOCOL.md`, `docs/ROADMAP.md`, `docs/ROLES.md`, `CHANGELOG.md`, adapter `NOTES.md` files.
- Sinet positions from the canonical drafts `Spec/drafts/S00…S19` (frozen v1, tag `spec-v1`), `P3/STATE.md` (B2 complete, gate open, 2026-07-20), and memory/register entries cited inline (D1, D5, D6, D7, D10; P-T01-1, P-T02-1, P-T03-4, P-T07-4).
- Analysis performed with three parallel deep-read passes (switchboard core; adapter/CLI/hook layer; spec-position extraction) plus coordinator line-reads of the protocol docs; agent reports retained in session scratchpad only — this file is the durable record.
