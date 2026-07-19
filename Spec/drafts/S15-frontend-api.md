## S15 — Frontend & API surface

**Scope:** The web workspace (all S3 surfaces plus PWA/push), the four binding frontend component picks, and the HTTP-API/SSE consumption contract under which every surface — SPA, chat, any future CLI or ingress channel — is an equal client of the control plane.
**Binding inputs:** G3 D3.3 + discipline riders · `Spec/frontend-components-v1.md` (cited **FC-v1**) · R17 §2.4–2.6, §4.3, §4.6, §4.10 · G2 D2.2 (Layer C patterns), D2.8 (receipt labels), Def.2, Def.13, Def.14 · G1 Def.3 (richer close set) · G3 D3.5, Def.1, Def.6 · feature list §6, §9, §13, S1–S3 · siblings: S01 (front chain, auth, settings registry), S13 (deliverable/comment contracts), S14 (event semantics, query layers), S10 (meter/receipt data), S06 (intake, stale-replan threshold).

### S15.1 Architecture: a disciplined React SPA

The frontend is a **plain React 19 + Vite SPA in TypeScript — no meta-framework, no RSC, no SSR layer — installed as a PWA** [G3 D3.3; R17 §4.3]. The RSC/SSR exclusion is load-bearing, not stylistic: the Dec-2025 critical-RCE class lived in RSC, which a plain SPA does not ship, and a tailnet-only app has no SEO or first-paint case for server rendering [R17 §2.4, §4.3]. The built assets are embedded in the control-plane binary and served through the S01.4 front chain; the web surface is not a process [XREF:S01].

**Discipline riders (bind)** [G3 D3.3; R17 §4.3]:

- Lockfile-pinned minimal dependency tree; every frontend component carries a `components.lock` entry — pin, license, funeral plan [XREF:S16].
- Scheduled dependency pass every ⚙ `frontend.dependency_pass_interval` = 90 d (quarterly); SHOULD be co-scheduled with S16's manifest review [XREF:S16].
- Vite majors on a lag: adopted only at a scheduled pass, never day-one.
- The Nexus anti-pattern (11.7k-line untyped hand-rolled SPA) is structurally excluded: typed framework + adopted organ-grade components only [R17 §4.3].

**HTMX re-entry — the named conditions [R17 §4.10], the only sanctioned path back:** (a) the S15.4 picks prove replaceable by framework-agnostic widgets at acceptable cost → HTMX 2.x re-opens as the architecture candidate (it holds the field's strongest written longevity commitment); (b) Datastar earns a re-look only at the first major frontend overhaul, and only if its 1.x line has proven stable through ~2027.

### S15.2 The API surface: one seam, equal clients

Every surface is a client of the same HTTP API and the one SSE endpoint — the SPA, the conversational assistant, any future CLI, and any 15.5 ingress channel; nothing renders from private access [XREF:S01 — the API seam]. The browser is a display, never an authority: authorization and visibility are enforced server-side through the S01.9 owner-scoped accessors and session identity [XREF:S01]; client-side filtering is presentation only.

**REST resource families (contract level; full endpoint schemas are P3 work within these bounds):**

| Family | Root | Exposes | Mutating verbs (all owner-scoped) | Data owner |
|---|---|---|---|---|
| tasks | `/api/tasks` | task objects: spec + numbered AC, plan, stage progress, lineage (project, follow-ups), receipt view | create (opens intake [XREF:S06]); follow-up spawn (S1.2); cancel (4.5) | S06/S02; receipt figures S10 |
| runs | `/api/runs` | run FSM state, live-activity refs, spawn records, routing records (S2.6) | cancel (4.5) | S02/S03/S10 |
| approvals | `/api/approvals` | inbox items — proposals, questions, sign-offs, escalations — with risk tier, expiry, and 13.5 help fields | answer (approve / deny / answer / re-plan); Low-tier batch answer | S02 effect journal; D7/D10 gates |
| deliverables | `/api/deliverables` | immutable numbered revisions, diffs, anchored comments, preview sessions | comment CRUD (own comments); request bounded revision; accept (6.3 → S13 flow) | S13 |
| settings | `/api/settings` | registry schema + values, price table, per-setting audit history | validated set/reset (clamped [XREF:S01]); price-table edits | S01.10; price table S10 |
| meters | `/api/meters` | consumption, pressure, budgets, burn rates, limit-event status per (person, lane, period) | budget edits (own); pause-my-automation switch (3.3) | S10 |
| memory [coordinator-draft] | `/api/memory` | scoped memory/knowledge entries (person / project / house) with provenance and gate status | manual entry create/edit — S09's write gate applies (own-store writes tier Medium; house promotion operator-only, D10) | S09 |
| events | `/events` (SSE) + `/api/events` (history) | the live stream; filterable history (S2.10) through the S14 query layers | — (append is control-plane-internal) | S14 |

Login/session endpoints are S01.9's and are not restated here [XREF:S01]. Contract rules binding every family:

- **No direct outward-effect verb exists anywhere on the API.** Anything non-idempotent and outward enters as a proposal and exits only through `approvals` (D7) [XREF:S02].
- **Writes are retry-safe.** Approval answers and accepts are pinned to the payload hash they were shown for: a stale-payload answer is rejected, a repeated answer returns the already-resolved state — a phone retry can never double-fire [G2 D2.1; XREF:S02].
- **Every mutation lands on the event log** and reaches clients over SSE; the REST response confirms, the event feed is the truth surfaces render from [XREF:S14].
- **Versioning is lockstep and unversioned at v0** [coordinator-draft]: SPA and API ship in one binary (S01.5), so no `/v1` prefix exists; evolution is additive-first, and a future non-bundled client pins to platform releases.

### S15.3 Live data: SSE consumption

Client-side consumption discipline for the one SSE endpoint (event semantics, event types, and the query layers are S14's [XREF:S14]):

- **Snapshot-then-tail:** a view loads its REST snapshot, then tails `/events` from the snapshot's `event_seq` cursor; disconnects resume via `Last-Event-ID` / `?after_seq` [G2 D2.1 (R12 §4.3)].
- A tab SHOULD hold one EventSource and fan events out in-app; multi-tab rides HTTP/2 terminated at the front door, so the 6-connection limit never binds [R17 §4.6; XREF:S01].
- On cursor gap or eviction the client re-snapshots — it never patches blind. The polling fallback is S14's transport concern [XREF:S14].
- Connection state is always user-visible (S15.12): a disconnected or catching-up surface says so rather than posing as live.

**AG-UI is vocabulary, not runtime** [G2 D2.2 Layer C]: the event-type names in S14's contract mirror AG-UI's vocabulary pattern (run lifecycle, streaming deltas, tool events, state sync) so chat, board, and activity feeds consume one recognizable dialect; no AG-UI runtime, SDK, or wire protocol is adopted, ever [FC-v1].

### S15.4 Binding component picks

The four picks are **binding** [FC-v1; G3 D3.3]. Versions are as recorded at binding (2026-07-18); exact pins live in `components.lock` [XREF:S16]. Adopt-don't-fork applies: the carried Nexus behaviors below are specs *over* these widgets, never modifications *of* them.

| Surface | Pick (binding) | Version at binding | License | Pre-registered fallback |
|---|---|---|---|---|
| Board drag-drop (S1.3) | **@hello-pangea/dnd** | 18.0.1 (peers `^18\|\|^19`) | Apache-2.0 | pinned @dnd-kit/core 6.3.1 (frozen-but-working) |
| Diff review (S3.5) | **react-diff-view** + gitdiff-parser, rendering the ported N15 behavior spec | 3.3.3 | MIT | monaco diff via @monaco-editor/react 4.7.0 — pre-registered upgrade path |
| Chat (S3.6) | **@assistant-ui/react** via LocalRuntime/ExternalStoreRuntime — assistant-cloud is never used | 0.14.27, pinned | MIT | owned chat primitives on the same SSE cursor |
| Settings forms (13.4/13.5) | **JSON Forms** (@jsonforms/core + react) with a Sinet-owned custom renderer set | 3.8.0 (react peer admits `^19`) | MIT | RJSF v6 (documented runner-up) |

Pick-level riders [FC-v1 §1–4]:

- The @dnd-kit 0.x rewrite family is excluded outright; re-evaluate only at its 1.0 with a dated note.
- react-diff-view's single-maintainer risk is bounded by the pin, MIT forkability, and containment to one view; syntax highlighting is client-side via its tokenizer — no server-side highlighter exists in the platform.
- @assistant-ui/react is 0.x: the pinned lockfile + quarterly pass absorb its churn; the ExternalStoreRuntime boundary keeps message/session state platform-owned, so a widget swap loses no data.
- JSON Forms renderers are Sinet-owned (no theme-package dependency); RJSF v6 re-enters only if JSON Forms stalls >18 months without a maintenance release or a registry need exceeds its rule/layout vocabulary.

### S15.5 Oversight surfaces: mission control, board, task detail, fleet, filters

**Mission control (S3.1)** is the home surface: one live screen of everything running, queued, parked (with "parked until…" times), blocked-on-a-human, and recently finished; consumption and automation-budget meters with burn rates per person and model [data XREF:S10]; who owns what; filterable history by project, person, status, and date [query layers XREF:S14]. Any item drills into its task detail and full trace.

**Live board (9.1, S1.3).** Tasks are cards moving through their stages in real time from the SSE feed; cards group by project with follow-up lineage visible (S1.1–S1.2 semantics [XREF:S06]). A card face shows exactly the S1.3 set: what it is, whose it is, current stage, effort mode — with any disclosed downgrade note (3.5) [G2 D2.1; XREF:S10] — cost so far, and waiting-on-human. Drag-drop uses @hello-pangea/dnd; **stage columns are never writable by drag** — stage is FSM state owned by the control plane [XREF:S02]. The sanctioned v0 drag interaction is reordering one's *own queued* tasks as a scheduler priority hint [coordinator-draft; XREF:S10].

**Task detail (9.2, S1.7)** shows the confirmed specification with its numbered acceptance criteria, the approved plan, per-stage progress with the live activity feed (S2.2), every human decision along the way (S2.4), every deliverable revision [XREF:S13], and the receipt: ceremony-vs-execution itemization and the done-directly figure under its ratified labels — "direct-use estimate (heuristic)" / "measured, n=…" [G2 D2.8; data XREF:S10].

**Fleet overview (9.3)** answers what is running on whose account at what burn rate: per-person/per-model consumption meters and automation-budget remainders (S2.5), limit-event status and history ("parked until…"), all filterable [XREF:S10; XREF:S14]. Accounts are always distinguished (S3.10; D2).

**Personal filters (S1.4)** are first-class saved views, identity-scoped: **what-needs-me** (approvals, questions, review-ready deliverables), **mine**, **running**, **finished-today**. All four are phone-complete (S1.10).

### S15.6 Approval inbox

The single queue where "the platform needs you" lives (S3.2); the deep-link target of every push (S15.11). Cards arrive ranked by risk — proposals, questions (incl. the S14 blind-pair verdict forms with their mandatory arm-guess field [XREF:S14; coordinator-draft]), sign-offs, escalations — and carry the 13.5 help fields — what this decision does, what could go wrong, what the platform recommends and why; for settings-class approvals that text is registry-sourced [XREF:S01].

**Risk tiers and batching (ratified rules, S3.2):**

- **Low** — read-only or reversible-inside-the-workspace → batchable: one action answers a selected set; each item stays individually listed and individually logged.
- **Medium** — writes to the person's own stores → owner approves individually; never batched.
- **High** — outward or irreversible (send, publish, push to shared, metered spend, permission changes) → **never batchable**; owner approves; **operator co-approves when platform-level** (D10), both approver states visible on the card; the PIN is re-prompted — approval identity is never inherited from an idle session [G3 Def.1; XREF:S01].

**Plan cards always carry both Approve and Re-plan** (S3.2). A card pending past the S06.9 staleness threshold auto-flags "assumptions may be stale" with one-click re-plan [XREF:S06]. Cards display their expiry countdown (ask-approval expiry [G2 Def.2; XREF:S02]). **Answering resumes the paused work in place** from its last checkpoint (4.3) [XREF:S02]. Re-nags follow the ratified SLA set — approval remind 4 h, re-push 24 h; safety escalations push immediately and re-ping hourly — with cadence ⚙ and delivery owned by the notifier [G1 Def.3 (richer close set); XREF:S14]. The approval-card payload contract follows the LangGraph interrupt/approval schema as pattern only — no runtime [G2 D2.2 Layer C].

### S15.7 Conversational assistant

A first-class surface inside the dashboard and on mobile (S3.6, 9.5): ask what's running, what's blocked on you, what something consumed — or start a task by chatting, which hands the conversation to the S06 intake pipeline in place [XREF:S06]. Platform-state answers ride the S14 query layers: deterministic views → canned queries (intent-filling is a local duty alias [XREF:S12]) → Layer-2 open SQL, whose answers MUST visibly carry their lower-confidence flag [G3 D3.5; XREF:S14].

The widget is @assistant-ui/react (S15.4) on LocalRuntime/ExternalStoreRuntime with a Sinet adapter speaking to the platform API; message and session state are platform-owned. **Carried Jarvis-tab behavior specs (bind) [FC-v1 §3]:**

- **Stream-survives-navigation inventory:** an explicit inventory of what a view switch may shed versus what MUST survive — the in-flight turn, its stream, and its produced-files accumulation survive navigation; hard-stop is a separate explicit user action, never a navigation side effect.
- **Error humanization:** the SSE alias/error-humanization table — provider/transport error classes mapped to plain language; empty-stream vs transport-error distinguished; 429/overload humanized; model fallback announced in-feed.
- **File exchange (v0 carry — explicitly not parked):** drag-drop + click-browse upload sidebar; uploaded-file list with delete; post-turn **produced-files chips** computed as an exchange-folder diff around the turn. Exchanged files are owner-attributed artifacts (15.6), fail-closed on ownership; files handed to a launched task enter as task inputs through intake [XREF:S06]; confinement rules govern anything a run touches [XREF:S11].
- **Sessions:** server-side per-user session registry, fail-closed; **first-message auto-titling** runs as a local-tier duty [Operating reality; XREF:S12].

### S15.8 Review surfaces (per deliverable type)

Everything arrives as a reviewable change (6.1). The behavior contract — the anchor schema (`file_path + side + line_no + line_text`), server-side re-validation with ±2-line drift tolerance, degrade-to-file-anchor and the explicit orphan state, the comment lifecycle `open → consumed` with batch `consumed_at`, the single findings→retry drain point — is S13's [XREF:S13; FC-v1 §2]. S15 renders it:

- **Code/text:** react-diff-view side-by-side and inline views over unified git diffs, gutter/widget hooks carrying anchored comments at exact places (6.2); client-side highlighting. **Round-over-round diff is the default rework view**, with revision-over-revision navigation across rounds (S3.5).
- **Unanchorable findings** render on the **synthetic render surface** — a finding is never invisible [FC-v1 §2].
- **Images:** visual compare (6.1). **PDFs:** extracted-text diff at v0 [G2 Def.13]. **Binaries:** hash refs + download from the local object dir [G2 Def.14].
- **Try-it (S1.9, 6.4):** one-click preview renders as the dual-iframe before/after surface; ports, disposable environments, and preview lifecycle are S13's stack [XREF:S13].
- **Accept (6.3, S1.8)** is one action on the reviewed revision; the attributed-commit flow is S13's [XREF:S13].

monaco diff is the pre-registered upgrade if editor-grade needs emerge [FC-v1 §2].

### S15.9 Settings UI

Generated, never hand-built: JSON Forms consumes the registry-emitted JSON Schema + UISchema (S01.10) with Group/Categorization tabs and rule-based SHOW/HIDE visibility — core, theme-independent vocabulary rendered by the Sinet-owned renderer set [FC-v1 §4; XREF:S01]. **Every setting** is present and editable here (13.4): the per-user flat-rate/metered flags, the API-equivalent price table [XREF:S10], automation budgets, freshness thresholds, depth cap, retention, missed-slot defaults, and every ⚙ in this spec [index XREF:S18]. The UI shows each setting's clamp bounds and enforces the split — automation moves *value* only within bounds, only the operator edits bounds; restart-required settings are badged; per-setting audit history renders from `settings_events` [XREF:S01]. 13.5 help text renders inline from the registry help field.

### S15.10 Workforce map (view-only at v0)

A readable graphical view of the machinery (9.4, S3.3): every worker, what each is equipped with (tools, knowledge, permissions, helpers), and how multi-stage procedures connect, rendered from the S08 worker registry [XREF:S08]. The map also surfaces S08's per-version quality/cost views (the version→outcome joins) alongside each worker [XREF:S08; coordinator-draft]. Its purpose is audit and understanding; identity rules apply (personal workers to their owner, the shared roster to all). **Editing through the map is parked to 15.5** — the v0 surface has no mutation affordances. Rendering uses owned components at v0; a dedicated graph-layout dependency may enter only through the manifest: TBD-P3(workforce-map rendering approach — owned rendering vs manifest-admitted layout library).

### S15.11 Notifications: Declarative Web Push

Decisions reach the right person on a phone (S3.7, 9.6). The channel is **Declarative Web Push**: JSON payload, no service-worker requirement, the required `navigate` field deep-links straight to the answerable card, and `app_badge` carries the identity's pending-approvals count as the home-screen badge [R17 §2.5, §4.6]. iOS requires Home-Screen PWA install and ≥18.4 for the declarative form; Android is standard push [R17 §2.5]. Every approval card and task has a stable URL, so deep links and bookmarks always land. The loop target is normative: **notified → glance → decide < 10 s on a phone, while the host is up** (S3.7; Operating reality).

Accepted facts, tied to the observables register [XREF:S01 — S01.8; P-T13-3]: no self-hostable standard Web Push service can exist — the user agent picks its push service (RFC 8030) [R17 §2.5]; payloads are encrypted to subscription keys, so vendor relays see **timing, volume, and endpoint metadata — never content** (register row 2). The frontend MUST NOT add any further external observable without a prior register amendment [XREF:S01].

**Week-one push drill:** TBD-OPERATOR(week-one real-device drill on household phones — iOS Home-Screen install + declarative push + `navigate` deep-link; Android push; the tap-with-tailnet-down edge; acceptance = notified → glance → decide < 10 s) [G3 Def.6]. **Flip condition [R17 §4.10]:** drill failure on household iPhones → notifications move to ntfy-with-relay; Web Push stays for badges only.

### S15.12 Cross-cutting duties

- **Live by default (S3.9).** Every surface updates from the SSE feed; manual refresh is never required for currency. A surface whose feed is disconnected or catching up MUST show that state visibly — stale data never poses as live; with the host asleep or the tailnet down, the PWA renders an honest unreachable state (the push-arrives-but-link-fails edge belongs to the S15.11 drill) [Operating reality].
- **Multi-user aware (S3.10).** Identity comes from the S01.9 session; every view is owner-scoped server-side; people see their own work and what is shared with them; approvals route to the decision owner (D10); the fleet always shows whose account burns (D2, 3.4). Shared-device PIN policy and High-tier re-prompts render as specified [G3 Def.1; XREF:S01].
- **Mobile/desktop (S1.10).** One responsive workspace, not two products. Phone-complete: the inbox and every decision, personal filters, board and fleet glancing, task status, chat, push handling. Desktop-comfortable: diff review, the workforce map, settings bulk edits. Both speak the identical API (S15.2).
- **Escape-by-default.** Model- and web-derived content renders escaped everywhere; the only sanctioned raw-HTML channel is S15.8's preview surface [FC-v1 §2].

### S15.13 Parked v1+ (behind the 15.3 benchmark gate) and open sockets

Per 14.5, no extra surfaces ship before the benchmark gate passes; the operator re-affirmed valuing this set on 2026-07-18 [FC-v1 §3]:

- **Avatar hologram + memory-galaxy backdrop** — the most portable pieces of the old SPA; plan: host in a React wrapper at v1, re-verify then [FC-v1 §3].
- **Conversation mode + STT/TTS** (incl. the clause-latency progressive-speech pattern) — re-enter as **satellite tools** under the 12.4 framing, never core platform features [FC-v1 §3].
- **JARVIS file exchange is NOT in this set** — it ships at v0 (S15.7).

Sockets the architecture leaves open: **multi-channel ingress (S3.8, 10.3)** at 15.5 — a bot channel is just another API client at the S15.2 seam, subject to the same intake, attribution, and gates; **workforce-map editing** at 15.5 (S15.10).

---

**Settings introduced (⚙):**

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `frontend.dependency_pass_interval` | 90 d (quarterly) | (30 d, 365 d) [coordinator-draft] | G3 D3.3 rider; R17 §4.3 |

**Known problems owned here:**

- **P-T13-3** — accepted-external-observables: register owned at S01.8; S15 binds the frontend contribution (push metadata = register row 2) and the no-new-observables-without-amendment duty (S15.11).
- **P-T16-1** — component onboarding: owner S16; enforced for the frontend tree here — the four picks and any future frontend dependency enter `components.lock` (S15.1, S15.4).

**Deferred / parked:**

- Avatar hologram + memory galaxy → 15.3 benchmark gate passes; host-in-React-wrapper plan, re-verify at re-entry [FC-v1 §3].
- Conversation mode + STT/TTS → 15.3 gate; re-enters as 12.4 satellite tools, never core [FC-v1 §3].
- Workforce-map editing → 15.5.
- Multi-channel ingress (S3.8) → 15.5; the socket is the API seam (S15.2).
- HTMX 2.x → re-entry only via the S15.1 named conditions [R17 §4.10].
- Datastar → re-look at the first major frontend overhaul if its 1.x line proves stable through ~2027 [R17 §4.10].
- monaco diff → pre-registered upgrade on editor-grade needs [FC-v1 §2].
- @dnd-kit/react → re-evaluate at its 1.0 with a dated note [FC-v1 §1].
- RJSF v6 → re-entry if JSON Forms stalls (>18 months) or a registry need exceeds its rule/layout vocabulary [FC-v1 §4].
- ntfy-with-relay → flips in only on push-drill failure; Web Push then keeps badges only [G3 Def.6; R17 §4.10].

**Coverage:**

| Feature-list item | Where |
|---|---|
| S3.1 mission control | S15.5 |
| 9.1 / S1.3 live board (card fields, real-time movement) | S15.5 |
| 9.2 / S1.7 task detail (incl. S2.2 live inspection, S2.4 decisions, 3.6 receipt display) | S15.5 |
| 9.3 fleet overview (+ S2.5 cost-altitude display) | S15.5 |
| S1.4 personal filters | S15.5 |
| 9.6 / S3.2 approval inbox (tiers, batching, Approve+Re-plan, resume-in-place) | S15.6 |
| 13.5 approvals explain themselves (display; field source S01.10) | S15.6 |
| 9.5 / S3.6 conversational assistant (+ S2.10 questions over history) | S15.7 |
| 6.1/6.2 / S3.5 review surfaces; S1.8 native review | S15.8 |
| 6.4 / S1.9 try-it preview (UI; machinery S13) | S15.8 |
| 6.3 accept as one action (UI; flow S13) | S15.8 |
| 13.4 settings UI (every setting; registry S01.10) | S15.9 |
| 13.2/13.3 see-everything, configure-everything frontend | S15.5–S15.10 |
| 9.4 / S3.3 workforce map view-only | S15.10 |
| S3.7 / 9.6 notifications carrying the decision | S15.11 |
| S3.9 live by default | S15.3, S15.12 |
| S3.10 multi-user aware throughout | S15.12 |
| S1.10 same workspace everywhere | S15.12 |
| S3.8 multi-channel ingress (parked 15.5; socket noted) | S15.13 |
| 12.4 / 14.5 parked satellite set | S15.13 |

**Open items for G4:** none. Six drafting-time sub-choices are flagged inline as [coordinator-draft] for G4 attention — three from the original draft: the unversioned-lockstep API posture (S15.2), the sanctioned board-drag interaction — reordering one's own queued tasks only (S15.5), and the `frontend.dependency_pass_interval` clamp range (⚙ table); three interlock refinements added at coordinator review (2026-07-19), each closing a committed sibling's expectation into this section: the `/api/memory` resource family (S15.2 ↔ S09's manual-L2 workspace-UI seam), the S14 blind-pair verdict forms as inbox question cards (S15.6 ↔ S14), and the per-version quality/cost views on the workforce map (S15.10 ↔ S08).
