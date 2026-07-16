# Self-Hosted AI Agent Platform — Feature List (v2)

**What this document is:** the complete list of what the platform does, described as capabilities and behavior. It exists so that a person or session with no other context can research *how* to build each capability independently, without being anchored by anyone's existing answers — **except** the items under "Decided constraints," which were deliberately settled after discussion, are fixed, and must be treated as given rather than re-derived or contradicted.
**Date:** 2026-07-16. Supersedes v1 (same date).

# Operator and hardware context

**Operator:** solo enterprise software architect / developer, building for personal use plus a handful of trusted household members and friends over a home LAN and Tailscale. Not a commercial product, but it must run reliably unattended **while the host is up**, and it will be maintained by one person.

**Hardware:**

- Laptop: Acer Predator Helios Neo 16 PHN16-73-9915 — Intel Core Ultra 9 275HX, RTX 5070 Ti Mobile 12 GB VRAM, 32 GB RAM (expandable to 96 GB), Ubuntu 26.04 LTS.
- Optional eGPU: RTX 3090 24 GB as a **second, separate VRAM pool**. Different models can be placed on different GPUs; a single model spanning both pools over Thunderbolt is bandwidth-limited and is not assumed.
- The host is a mobile laptop that sleeps and travels. Every availability promise in this document means **best-effort while the host is up** (see 2.8 and "Operating reality").

---

# Decided constraints (fixed — treat as given; do not re-derive, do not contradict)

D1. **Central topology.** All execution happens on the operator's machine; members connect over LAN/Tailscale from any of their devices. No runners on member devices (kept only as a far-future option, 12.8).

D2. **Per-person credentials on the host.** Each person's provider credentials live in that person's own store on the host (separate directories / env files). Every run authenticates as its owner; subscriptions are never pooled or cross-used. OS-level user separation is deliberately omitted (mutual-trust household): **the sandbox is the load-bearing isolation boundary, and credentials never enter task sandboxes** (S5).

D3. **Dual execution substrate behind one adapter contract.** Every provider connection goes through an execution adapter with a common minimum contract: start, stream progress, checkpoint, pause, resume, cancel, report usage. Per provider, the adapter either wraps the provider's first-party tool (where subscription terms require it) or calls an API directly (including local models).

D4. **Consumption metering, reactive limits.** The platform measures consumption exactly and treats provider limit events as normal, recoverable scheduling events. It does not model or predict providers' quota windows (Section 3).

D5. **Two currencies.** Between flat-rate options, scheduling and routing use consumption pressure — measured against the person's automation budgets and observed limit events — never dollars, which are marginal-zero there. Dollars, from the user-maintained API-equivalent price table, are the reporting currency for receipts and comparisons, and become a routing input only for explicitly enabled metered use.

D6. **Single-coordinator topology with a depth cap.** One coordinator per task with isolated helpers; sub-helpers only within a configurable depth cap (default: 2 levels below the coordinator); no lateral messaging at any depth; every spawn is logged with its reason.

D7. **Checkpoint-and-gate invariant.** Progress is checkpointed after every paid model call; all non-idempotent outward effects exist only as gated proposals until approved (4.8).

D8. **Worker model: template → overlay → instance.** Versioned templates (personal by default; household-shared only through operator approval), per-user overlays, per-run instances (7.8).

D9. **Per-project git, GitHub as the off-host home.** Every project can have a git repository; accepted work lands as attributed commits by the accepting user. Remotes live on GitHub: each user's project repositories sit as private repos under that user's own GitHub account, pushed with credentials from that user's store (D2); a shared project's repo lives under its owner's account, with the other members invited as collaborators. Platform-state snapshots go to one dedicated private repository under the operator's account (11.3).

D10. **Approval principle.** Everyone approves their own objects — specs, plans, permissions, deliverables, personal workers, personal lessons. Promotion of anything to household-shared (worker templates, house knowledge, platform settings) requires operator approval.

---

## Known problems this program must solve — each with a named owner

(Research sessions should keep hunting for additional problems and give each one an owner.)

- Deferred error correction → 5.3–5.6, 4.6
- Permission configuration replacing permission prompts → 4.2–4.4, S3.2
- Session amnesia → 4.3, 4.8, S4
- Context rot → 4.3 (freshness re-validation), S4.2, S4.10
- Silent failure modes → 4.6, S2.7
- Quota-state opacity (providers expose no remaining-window state) → 3.1, 3.2
- Provider ToS / behavior drift → S2.8, D3
- Model deprecation vs stored worker configurations → 7.3
- Prompt injection and credential exfiltration → 4.7, S5, D2
- Validating auto-composed workers → 7.2, 7.6
- Host availability (mobile laptop) → 2.8, "Operating reality"
- Local resource contention → 3.11
- Platform-state loss (disk death) → 11.3
- Parallel same-project write collisions → S1.11, 4.3

## Operating reality (facts the features live in — not design choices)

- The platform is self-hosted on the operator's own hardware and serves the operator plus a small circle of trusted people over a home network and Tailscale, with private remote access. It is personal infrastructure, not a product — but it must run reliably unattended while the host is up, and it will be maintained by one person.
- All frontier-AI usage must be covered by the flat-rate consumer subscription each person already pays for. Subscriptions are per-person and are never pooled or shared: each person's runs use only that person's subscription, for that person's work, authenticated as that person (D2). Whether a model is flat-rate or metered is declared per model by each user and can change with little notice; at least one major provider permits subscription-covered programmatic use only through its own first-party tool (hence D3). Pay-per-token usage is acceptable only as an explicit, deliberately enabled exception — never a default and never a silent fallback.
- Providers do not reliably expose remaining-quota state. The platform therefore meters its own consumption exactly (3.1) and treats provider limit events as routine, recoverable interruptions (3.2).
- Users range from the technical operator to people with no IT or AI knowledge. The platform must produce properly built automation for both.
- A local GPU is available, so locally run models are a permanent free tier — concretely: the platform's own background intelligence (health watching, change detection, inbox risk-ranking, routine classification) runs on local models only, costs nobody any allowance, and keeps working when every paid window is empty.
- Human control is a product requirement, not a safety afterthought: the humans decide at defined points, from any of their devices.
- Availability is best-effort while the host is up; scheduled work therefore has explicit missed-slot behavior (2.8).

---

## 1. Giving the platform work

1.1 **Natural-language intake for everyone.** Anyone can describe a task in plain language — coding, documents, research, content, data work — with no prompting skill required.

1.2 **The platform refuses to run on vagueness.** It detects when a request is too under-specified to execute well and interviews the requester, asking the questions a good project manager would ask for that *kind* of task (a software task, a research task, and a content task each have different must-know details).

1.3 **Understanding is confirmed, not assumed.** The platform restates the task in its own words until the requester confirms it matches the picture in their head. The confirmed result is an explicit specification with numbered acceptance criteria — the contract all later work is measured against.

1.4 **Nothing agreed can silently disappear.** Every acceptance criterion in the specification is traceably owned by some part of the eventual plan; if a criterion would be dropped, that is surfaced, never silent.

1.5 **The platform attacks its own plan before showing it.** Prior to presenting a plan, it stress-tests it — "assume this failed: why?" — and fixes the weaknesses it finds. If this self-critique concludes the *specification itself* is a bad idea (not just incomplete), that concern is raised to the requester as an explicit decision rather than being quietly absorbed.

1.6 **Cheap preview before expensive execution.** For every open-ended task, the requester sees a plan and approves or redirects it at negligible cost before the expensive execution starts. Reviewing a plan costs seconds; a wrong execution burns hours of quota.

1.7 **Ask, don't assume.** Whenever a consequential choice is ambiguous, the platform asks one clarifying question rather than guessing.

1.8 **Planning effort scales with stakes.** Low-stakes routine tasks get a slim intake — including the option of zero mandatory interaction; high-stakes tasks get the full treatment — automatically, regardless of what convenience level the user chose. The size-guess verification of 2.5 is itself stakes-gated: trivial tasks never require a confirmation click.

1.9 **Real-world data means real research.** Tasks whose output depends on facts in the world (prices, products, laws, technical interfaces, prior art) always include live research as a required step — correctness is never left to the model's memory or initiative.

1.10 **Ceremony has a designated engine.** The platform's own thinking around a task — interviewing, restating, plan critique, verification review, lesson proposal — runs on the requester's designated *utility model*. The platform recommends the best-fitting model for these duties from the person's connected set — best result per unit of allowance consumed, not merely the cheapest — and recommends the same default to everyone so platform behavior is uniform; the user can override. Ceremony consumption is billed to the requester and itemized separately on the receipt (3.6).

## 2. What kinds of work it handles

2.1 **Broad-spectrum knowledge work with honest maturity levels.** From launch the platform *accepts* tasks across software development, marketing, research, web deep research, and content creation, automatically creating whatever is needed (workers, skills, hooks, tools, integrations, workflows, orchestration) when a request arrives for a domain not yet covered. Two domains launch with the **full verified pipeline**: software development (verified by executable checks) and web research (verified by source/citation rubrics). All other domains run in **degraded mode** per 7.6 until their purpose-built quality check exists: visibly marked, mandatory requester review, never unsupervised. The platform never refuses a domain; it is honest about verification maturity. (Sequencing: Section 15.)

2.2 **All task shapes:** quick one-shot requests, iterative tasks with revisions, long-running multi-stage jobs, scheduled recurring tasks ("every Monday…"), and event-triggered tasks ("when X arrives…").

2.3 **The platform tells task types apart** and runs each the best-suitable way to produce the best result quality per unit of allowance consumed, depending on the selected effort mode (Eco, Balanced, Smart — defined in 3.5). Each task gets analyzed and automatically assigned an existing workflow or worker following current best practice.

2.4 **Automatic orchestration within the fixed topology.** Orchestration of workflows, workers, and tasks is automatic and follows current best practice **within the decided topology (D6)**: one coordinator per task, isolated helpers, sub-helpers only within the configurable depth cap (default 2), no lateral messaging, every spawn logged with its reason (7.7). Best practice chooses the structure and depth for each task and domain *within* the cap, never beyond it.

2.5 **Misjudged task size self-corrects cheaply.** The platform's initial guess about how heavy a task is gets checked against the actual plan and the actual outcome; for non-trivial tasks it is displayed to the user to verify or change (stakes-gated, 1.8). An underestimated task is upgraded before expensive work starts; an overestimated one is caught by its preview and cost limits. The classifier only needs to be roughly right, because being wrong is cheap.

2.6 **External-service chores are first-class machinery.** Routine integrations with outside services (mail, spreadsheets, business tools, webhooks) run as cheap deterministic automation, never burning premium AI judgment. Generated automations are **born and governed as machinery** (Section 7): versioned, supervised on first run, minimally permissioned, attachable to schedules, with rollback and retirement — while their *definition* is presented at birth like a deliverable: a readable diff, a preview, commentable. Reviewed like code, managed like a worker.

2.7 **Gap advice.** If a requested task type has no well-suited model among the connected subscriptions, the platform says so and recommends — with current reasoning and pricing — what subscription would close the gap.

2.8 **Missed schedules have defined behavior.** Because the host may be asleep or away when a scheduled slot passes, every schedule carries a missed-slot policy — run-once-late (default), skip, or notify-only — so "every Monday" never silently becomes "whenever the lid opens."

## 3. Cost & quota management

3.1 **Exact consumption metering.** The platform always shows consumed tokens/usage: for each person and each model, per task as well as per day, week, and month. Which models are currently flat-rate versus metered is declared by each user per model. A user-maintained API-equivalent price table — shipped with sensible defaults, updated manually in settings when providers change prices — converts consumption into the reporting currency for receipts and comparisons (D5).

3.2 **Limit events are normal, recoverable scheduling events.** When a provider limit interrupts a run, the run parks losing nothing (4.8); the resume time is taken from the provider's own signal where available, otherwise retried on a schedule; parked work resumes without anyone babysitting. The platform does not predict provider windows — it reacts to them (D4).

3.3 **Automation never starves the human.** Each person sets automation budgets — consumption caps for background and batch work, per model per period — enforced through the exact metering of 3.1. Batch work prefers each user's configured off-hours, and every person has a one-switch "pause my automation." The platform's own consumption never blocks a person's interactive use.

3.4 **Every run is billed to its requester.** Whose task it is determines whose allowance it consumes — always visible, never mixed — including ceremony consumption (1.10), itemized. Event- and webhook-triggered tasks bill the person who registered the trigger.

3.5 **Per-task effort modes, defined against depletion.** Eco = an adequate result at the lowest consumption; Balanced = the best result per unit of consumption; Smart = the best possible result within the person's automation budgets, parking on limit events. Between flat-rate options, routing never uses dollars (marginal cost there is zero); dollars enter routing only for explicitly enabled metered use (D5). When the chosen level isn't available, the platform downgrades gracefully and says so.

3.6 **Honest receipts.** Every job ends with an account of what it consumed — real money where metered, API-equivalent value (from the 3.1 price table) where flat-rate — itemized into ceremony versus execution, and always including the comparison that keeps the whole platform honest: what this would have cost done directly, without the platform.

3.7 **Hard ceilings on every run:** time, steps, and cost limits; detection of runs going in circles; detection of runs gone silent. No task can quietly burn a week's allowance.

3.8 **Runs survive disasters.** Platform crashes, restarts, machine sleep, and limit interruptions never destroy paid AI work: work that finished during the outage is recovered as a result; work still in progress resumes from its last checkpoint; re-spend is bounded by the last checkpoint (4.8), and outward actions are never repeated.

3.9 **Never fully offline.** With every paid allowance exhausted, the platform keeps operating on local models for whatever remains feasible and parks the rest with resume times.

3.10 **Metered spending is opt-in only.** Per-token billing can never be entered silently — it requires an explicit, deliberate flag per use.

3.11 **Local resources are scheduled, not assumed.** GPU/VRAM, RAM, and CPU are shared between sandboxes, local inference, and the operator's own interactive use of the machine; the platform arbitrates them like any other scarce resource, and the operator's interactive use of the laptop always wins.

## 4. Safety & control during execution

4.1 **Isolated execution with enumerated exceptions.** Every run works in its own isolated space; parallel jobs cannot interfere with each other or with anyone's live files. The only sanctioned sharing: read-only common caches (packages, models), the registered project store via the workspace-clone mechanism (S1.6), and resources a project explicitly shares. Everything else is isolated. Concurrent tasks inside the same project are additionally coordinated at the artifact level (S1.11).

4.2 **Destructive and outward-facing actions are blocked by default.** Deleting, publishing, pushing, sending — anything that changes the world outside the workspace — is collected as a *proposal* and released only through human approval.

4.3 **Blocked is not failed — and resume is freshness-checked.** When a running job needs a permission or an answer, it pauses in place, notifies the requester, appears in the approval inbox reachable from any device (including a phone), and — once answered, even hours later — continues from its last checkpoint. If the pause exceeded a configurable freshness threshold (default 24 h), or the work's target changed while paused — the repository moved on, sources changed, price or limit assumptions shifted — the platform first re-validates the remaining plan against current reality at low cost, then continues, adjusts with a note, or escalates the change as an explicit decision before spending anything significant.

4.4 **Minimal powers per worker.** Every worker gets only the tools and access rights its kind of task requires — nothing more.

4.5 **A stop that stops.** Any run can be cancelled cleanly at any time; and the whole platform supports a maintenance mode that finishes in-flight work while accepting nothing new.

4.6 **Nothing fails silently.** Every run continuously reports its state; a stalled, looping, or dead run is noticed, contained, and surfaced — never discovered days later.

4.7 **Untrusted content is treated as hostile — with honest limits.** Workers that read the open web or other untrusted input run in the tighter confinement classes of the ladder (S5). Credentials never exist inside tool sandboxes; provider sessions live outside them (D2). Injected content can still *steer a model's reasoning* — that risk is not fully removable. The guarantee is blast radius: no credential access, no un-proposed outward effect, and the verification gate judging steered output. Containment, not immunity.

4.8 **Checkpoint-and-gate invariant.** Progress is checkpointed after every paid model call; all non-idempotent outward effects exist only as gated proposals until approved. Any crash, pause, or retry can therefore at worst repeat the bounded work since the last checkpoint — never an outward action, never paid work older than the checkpoint.

## 5. Quality assurance

5.1 **Nothing is delivered unverified.** Every deliverable passes a verification step matched to its type before the requester sees it — code against executable checks, other work against the confirmed specification.

5.2 **Two independent questions, always:** *does it meet the spec?* and *is it actually good?* A result can be formally compliant and still be a bad product; the platform must be able to catch that, and such a finding can reopen the specification itself as an explicit human decision.

5.3 **Free and cheap checks run before expensive ones.** Obviously broken output (empty, placeholder, truncated) dies instantly at zero cost; deeper review effort scales with what survives and with the stakes.

5.4 **Rework is bounded.** Revisions are limited in number and cost; only genuine blockers trigger another round — polish suggestions travel along as notes instead of spinning loops; and re-review judges against the *original* criteria, so goalposts cannot drift.

5.5 **Feedback is concrete and carried forward.** Reviewer findings are attached to the exact places they concern and handed to the next attempt as numbered points, so a retry fixes the named problems instead of regenerating and hoping.

5.6 **Every stage has a working escalation path.** Any part of the pipeline that hits something it cannot resolve can raise the issue to a human as an explicit decision — and this path is proven by tests, because a finding that dies in a log is a defect of the platform.

5.7 **Different work, different judges.** All domains (software development, marketing, research, etc.) get purpose-built quality checks individually adjusted for their own needs; a domain without a real quality check yet is honestly marked as such (see 7.6). The initial rubrics for the launch domains are researched and drafted with the strongest available frontier model following current best practice, enter through the knowledge-approval gate (8.3) — versioned, attributable, removable — and the benchmark practice (11.2) tests whether rubric-driven review actually catches real defects, so the rubrics themselves stay falsifiable.

5.8 **The human is the final gate.** The requester accepts or rejects; nothing self-approves into the world.

## 6. Deliverables & review

6.1 **Everything arrives as a reviewable change.** Every deliverable — code, documents, PDFs, images — is presented as a comparison against what existed before, with revision-over-revision comparison across rounds. Each type of deliverable has a proper frontend implementation to be viewed and compared: for code or text a side-by-side comparison, for pictures a picture view, and so on.

6.2 **Review like a pro, for every format.** The requester can comment on exact places in the deliverable; those comments feed the next revision (5.5).

6.3 **Accepting is one action** and produces a clean, attributable change in the project's git repository or document store, committed as the accepting user (D9, 11.3).

6.4 **Built software can be tried instantly:** one click launches the produced application in a disposable environment, installing anything needed automatically to run it; whole projects can be previewed side-by-side as before-versus-after without touching the live copy.

## 7. A self-extending workforce

7.1 **The platform builds its own workers.** When a task type arrives that no existing worker or procedure fits, the platform composes a new one following the latest state of the art and best practices — including the deterministic automations of 2.6, which are born here too.

7.2 **New workers are born supervised — and "validated" means something specific.** Validation = structural checks passed (configuration lint, permission audit, dry run on a sample task) plus human sign-off — explicitly *not* a quality guarantee. A newly composed worker is shown for approval before first use and watched closely on its first run; in domains without a real quality check (7.6), its first N outputs (configurable) additionally require requester review regardless of oversight settings. Only then is it promoted to normal service.

7.3 **Workers are versioned assets.** Every worker and procedure definition is versioned with rollback; the platform records which version produced which outcome. **Revalidation trigger:** when an underlying model is changed, deprecated, or replaced, every worker version tuned against it is flagged and must be revalidated before further unsupervised use.

7.4 **Recurring registration.** A worker can be attached to a schedule or an external trigger (with a missed-slot policy, 2.8), after which its results simply appear — finished, verified, and receipted.

7.5 **The workforce accumulates and specializes.** Over time the roster reflects the household's actual life: the weekly competitor summary, the database-performance specialist, the brand-copy writer — each with its domain knowledge and its domain quality check.

7.6 **Honest capability marking.** A worker in a domain that has no real quality check yet runs visibly marked and supervised, and cannot graduate to unsupervised operation until such a check exists.

7.7 **Routing is accountable.** Every decision about which worker, which model, and which effort level ran a task is recorded with a plain-language reason, so routing itself can be audited and improved.

7.8 **Worker ontology: template → overlay → instance.** A worker is a versioned **template** (definition, equipment, permissions) — personal by default; promotion to the household-shared roster requires operator approval (D10). Each user has a per-user **overlay** on the templates they use: their lessons, preferences, and accumulated experience with that worker. Every task spawns a fresh per-run **instance** whose working notes expire with the task. Two people running same-domain tasks therefore work in parallel on their own instances with their own overlays, and nothing leaks between them (10.1).

## 8. Learning & memory

8.1 **The platform improves only with permission.** Lessons and evals are proposed automatically from real outcomes — the requester's edits, comments, accepted and rejected work — but nothing is learned until a human approves it.

8.2 **Learning is traceable and reversible.** Every adopted lesson is attributable to its origin and can be individually removed; a bad lesson cannot silently poison future work.

8.3 **Experience distills into durable knowledge.** With approval, recurring insights become permanent playbooks, style guides, and quality rubrics; proven excellent results become reusable reference examples.

8.4 **House knowledge flows into every job.** Each task automatically receives the relevant slice of accumulated knowledge — the brand voice, the project's conventions, the domain playbook — so workers behave like they've been here for years.

8.5 **Personal memory per user.** The platform remembers each person's preferences, context, and history, and applies them to their tasks.

8.6 **Worker memory has hygiene.** What a worker remembers has defined lifetimes and defined writers — short-lived scratch context, medium-term experience, and permanent operator-taught rules — so memories cannot silently accumulate garbage.

8.7 **One shared truth.** All workers, whatever runs them, see the same current project knowledge and task state — no worker acts on a stale or private version of reality.

8.8 **Learning and knowledge move between people only by choice.** Lessons and personal knowledge entries are individual by default; other household members can see what exists and adopt an entry for themselves — with its origin visible. Project knowledge is shared among that project's members (8.7 depends on it); house knowledge changes require operator approval (D10, S4.5).

8.9 **Conflicts resolve by declared precedence.** Explicit task specification > project truth > personal preferences > house defaults. Substantive conflicts are surfaced as a question, never silently resolved.

## 9. Workspace & visibility

9.1 **A live board of all work:** every task as a card moving through its stages in real time, grouped into projects with follow-ups for long engagements, with templates for one-click recurring setups and a registry of known projects/repositories so requests need no path-explaining.

9.2 **Task detail shows everything that matters:** the confirmed specification, the approved plan, per-stage progress, and the cost receipt.

9.3 **A fleet overview:** what is running right now, on whose account, at what burn rate — with consumption and automation-budget meters per person and model, limit-event status ("parked until…"), and filterable history.

9.4 **A workforce map:** a readable visual view of how the workforce is built — which workers exist, what each is equipped with, how multi-stage procedures connect — so an advanced user can audit and understand the machinery.

9.5 **A conversational assistant** that knows the platform's state: ask what's running, what's blocked on you, how much someone has consumed — or start a task by chatting.

9.6 **Decisions reach you.** Pending approvals notify the right person and are answerable from a phone; approval requests are ranked by risk and batchable where safe (risk tiers and batching rules: S3.2), so oversight stays sustainable instead of becoming rubber-stamping.

## 10. Multiple people

10.1 **True multi-user:** each person has their own workspace, preferences, memory, and credentials; nothing of one person's access can ever leak into another person's runs. This is enforced through per-person credential stores, runs authenticating as their owner, and per-user overlays (D2, 7.8) — not through OS user separation, which is deliberately omitted.

10.2 **Fair attribution:** every run is owned; consumption, receipts, and histories are per person.

10.3 **Multiple doors (later; sequencing 15.5):** tasks can also arrive through familiar messaging channels — a household member messages a bot, a task appears — with every entrance subject to the same scheduling, attribution, and gating rules.

## 11. Trust & auditability

11.1 **Full audit trail per run:** every step, decision, cost, and verification verdict is recorded and inspectable after the fact. Full traces are kept for a configurable retention period (default: 6 months), then compacted; summaries, verdicts, and receipts are kept indefinitely.

11.2 **The platform must prove it deserves to exist.** It maintains a benchmark practice comparing its results against simply using a frontier model directly — same tasks, blind scoring, pre-declared per-domain metrics and thresholds. Operationally: tasks are sampled at a configurable low rate; the requester scores the pair as a blind A/B pick; the duplicate run's consumption is drawn from the requester's automation budget with their opt-in. "Is this platform adding value?" stays a permanently measured question with a current answer.

11.3 **Platform state survives disk death.** Per-project git repositories are pushed to their GitHub remotes on every accepted change (D9). Platform state — task history, receipts, memory stores, worker templates, house knowledge, settings — is snapshotted on a schedule to one dedicated private GitHub repository under the operator's account. Because the target is GitHub, snapshots are shaped for it: **client-side encrypted before leaving the host** (a private repo is access-controlled, not end-to-end encrypted, and this data includes every member's memories and receipts); exports inside the archive are text-first (dumps rather than binary database files) so snapshots stay small; snapshot history kept in the remote is bounded rather than growing forever; and raw run traces are excluded — full traces remain local under the 11.1 retention policy, while their compacted summaries, verdicts, and receipts are part of the snapshot set. If the snapshot ever outgrows what a git repository comfortably holds, the escape hatches are release assets or a second target — GitHub blocks files over 100 MB and is not a blob store. Secrets are vaulted separately and never leave the host unencrypted. The restore procedure is tested, not assumed.

## 12. Later & optional capabilities (explicitly future)

12.1 **Local image generation** on the operator's own hardware as a platform tool for content work (the only flat-cost programmatic image path).

12.2 **Web-operating workers** that can browse and act on websites, under the tightest confinement class (S5, C4).

12.3 **Ambient repository upkeep:** background maintenance chores on code repositories (triage, housekeeping) under the same write-approval rules.

12.4 **Voice and vision companions as separate satellite tools**, never core platform features.

12.5 **Sharing worker templates between users**, with provenance (governed by D10).

12.6 **Supervised self-tuning:** the platform proposing improvements to its own workers' configurations from observed failure patterns — always measured, always human-approved.

12.7 **Growth beyond one machine** when a single host genuinely becomes the ceiling.

12.8 **Per-person satellite runners** on members' own devices — only if the central topology (D1) ever hits a genuine wall (availability or provider terms) — coordinated by the same control plane, subject to the same gates.

## 13. For AI beginners as well as advanced users

13.1 The tool handles everything automatically based on current state-of-the-art practice.

13.2 The tool displays everything in a well-organized, modern frontend overview — everything automatically configured — so advanced users can see what the tool did and can approve it as well as change it.

13.3 Everything can also be configured and set up manually by advanced users via the frontend.

13.4 A settings tab includes every single setting, well organized, for the administrator to change — including the flat-rate/metered flags, the API-equivalent price table, automation budgets, freshness thresholds, depth cap, retention period, and missed-slot defaults.

13.5 **Approvals explain themselves.** Every approval card carries plain-language help: what this decision does, what could go wrong, and what the platform recommends and why — detailed enough that a non-IT user gets a real idea of right and wrong before deciding. Everyone approves their own objects; only promotion to household-shared assets requires the operator (D10, S3.2).

## 14. What it deliberately does not do

14.1 No freely chatting agent swarms — delegation is always one coordinator with isolated helpers; sub-helpers only within the configured depth cap (default 2), no lateral messaging at any depth, every spawn logged with its reason (D6).

14.2 No unsupervised self-modification — and workers can never alter their own permissions, budgets, or approval gates, ever.

14.3 No silent metered billing, no silent provider switching.

14.4 No standing army of specialist agents attached to every task by default — machinery is added only when a task earns it.

14.5 No decorative breadth before the core proves its value in the benchmark practice (11.2): full-pipeline breadth waits for the benchmark gate; accepting other domains in honestly degraded mode (2.1, 7.6) is allowed from launch and does not count as breadth in this sense. No voice, no visual gimmicks, no extra surfaces until the gate passes.

## 15. Build sequence (decided)

15.1 **v0:** software development end-to-end — intake → specification → plan → execution → verification → review → accept → receipt — plus the consumption/metering layer (3.1–3.6), checkpointing and gating (4.8), confinement classes C0–C2 (S5), and the approval inbox (S3.2). Operated single-user (operator only).

15.2 **v0.1:** web research as the second full-pipeline domain (adds confinement class C3).

15.3 **Benchmark gate:** the 11.2 practice runs and passes its pre-declared thresholds before anything further is added.

15.4 **v1:** household onboarding; degraded-mode domains (marketing, content creation, general research) per 7.6; task templates; scheduling and event triggers with missed-slot policies (2.8).

15.5 **Later:** Section 12 items, multi-channel ingress (10.3, S3.8), editing the workforce through the visual map (S3.3).

15.6 **The data model is multi-user from day one.** Every record carries its owner even while operation is single-user; retrofitting identity later is the one cost this sequence refuses to pay.

---

## S1. The daily work environment
*(expands main list 9.1, 9.2, 6.3, 6.4)*

S1.1 **Projects as living containers.** Related tasks group into projects that carry shared context: what this engagement is about, what has been delivered so far, what was decided along the way. A follow-up task created inside a project inherits that context automatically — "apply the changes we discussed to the latest version" works without re-explaining anything.

S1.2 **Follow-ups from deliverables.** Any finished deliverable can spawn a successor task in one action — a revision, an extension, a counterpart ("now the English version") — linked to its predecessor so the lineage stays visible.

S1.3 **The live board.** All tasks appear as cards on a board and move through their stages in real time as work actually progresses — no refreshing, no polling. A card shows at a glance: what it is, whose it is, what stage it's in, what effort mode it runs at, what it has cost so far, and whether it's waiting on a human.

S1.4 **Personal filters.** One view answers "what needs *me* right now" (approvals, questions, review-ready deliverables); others answer "what's mine," "what's running," "what finished today."

S1.5 **Task templates.** Recurring kinds of work are saved as templates: pre-filled specification slots, default effort mode, default oversight level. Starting the monthly report or the weekly summary is one click plus the variables that actually change.

S1.6 **A registry of known projects and repositories.** Code repositories and document collections are registered once — with their conventions, commands, and danger zones captured — so from then on a request can just say "in the shop backend" and the platform knows exactly where that is and how to behave there. Onboarding a new repository is itself a task the platform performs.

S1.7 **The task detail view shows everything that matters:** the confirmed specification with its numbered acceptance criteria, the approved plan, per-stage progress with a live activity feed, every decision a human made along the way, every version of the deliverable, and the cost receipt.

S1.8 **Working with results is native, not exported.** Deliverables are reviewed inside the workspace (see main list Section 6): compare against the previous state, comment on exact places, request a bounded revision, or accept — and acceptance produces a clean, attributable change in the connected repository or document store.

S1.9 **Try it immediately.** An application the platform built can be launched with one click in a throwaway environment to click through; a whole project can be shown side-by-side as it-was versus it-would-become before anything is accepted.

S1.10 **The same workspace everywhere.** Oversight actions — approvals, answers, accept/reject — work fully from a phone; review-heavy work is comfortable on a desktop. It is one workspace across devices, not two products.

S1.11 **Parallel work inside one project is coordinated.** Multiple follow-up tasks may run concurrently in the same project. Tasks touching disjoint artifacts proceed in parallel; tasks that would write the same artifacts are detected at planning time and sequenced or explicitly branched. When an accepted sibling moves the project forward, any still-running task in that project is freshness-checked against the new state (4.3) before spending anything significant. A collision discovered only at accept time is surfaced as a reviewable merge — never silently overwritten.

---

## S2. Observability & trust requirements
*(expands main list 11.1, 11.2; consolidates 3.6, 3.7, 4.6, 7.7)*

S2.1 **Every run is fully traceable.** For any task, a person can see the complete story: every step the worker took, every tool it used, what it produced at each stage, in order — while it runs and for the full retention period (11.1), after which compacted summaries, verdicts, and receipts remain.

S2.2 **Live inspection.** A running task can be watched in real time — current activity streaming as it happens — not just "started" and "ended."

S2.3 **Every verification verdict is recorded with its reasons:** what was checked, against which criteria, what passed, what failed, and what was flagged as a note rather than a blocker — per round, so the progression across revisions is visible.

S2.4 **Every human decision is part of the record:** who approved which gate, who answered which question, who accepted the deliverable, and when. Accountability covers the humans, not only the machines.

S2.5 **Cost is observable at every altitude:** per run (the receipt), per task, per project, per person, per time period — including consumption meters, automation-budget remainders, burn rate over time, and limit-event history — and always including the honesty figure: what the same work would have cost done directly.

S2.6 **Routing is explainable.** For every task: which worker ran it, on which model, at which effort level, and the plain-language reason why — inspectable per task and analyzable across many tasks, so routing quality is itself auditable.

S2.7 **The platform watches its own health.** Stalled runs, looping runs, silent runs, and abnormal spend are noticed and surfaced by the platform itself — problems announce themselves; they are never discovered days later by accident. This watching runs on local models (Operating reality) and costs no allowance.

S2.8 **Change in the outside world is detected.** When a connected AI service changes its behavior — formats, limits, billing status — the platform notices and alerts before failures or costs spread, rather than degrading quietly. Adapter behavior changes (D3) are part of this watch.

S2.9 **Learning is auditable.** Every adopted lesson and knowledge change shows its origin (which task, which human approval) and can be traced forward to the work it influenced — and removed again (see S4.8).

S2.10 **History is queryable.** All of the above is filterable and searchable — by project, person, status, date, worker — and the conversational assistant can answer questions over it ("what did the marketing tasks cost this month?", "why did Tuesday's run fail?").

S2.11 **The platform continuously proves its own worth.** The standing benchmark practice of 11.2 — same tasks, blind requester scoring, pre-declared per-domain thresholds, sampled at a configurable rate, duplicate-run consumption opt-in from the requester's automation budget — keeps "is this platform adding value?" a permanently measured question with a current answer.

---

## S3. Frontend requirements
*(expands main list 9.3, 9.4, 9.5, 9.6, 10.3)*

S3.1 **A web dashboard as the home surface ("mission control").** One screen shows the whole operation live: everything running, queued, parked (with "parked until…" times), blocked-on-a-human, and recently finished; consumption and automation-budget meters with burn rates per person and model; who owns what; and filterable history by project, person, status, and date. Any item drills down into its live activity and full trace.

S3.2 **A unified approval inbox.** Everything awaiting a human — specification sign-offs, plan approvals, permission requests, new-worker approvals, escalated findings — arrives in one queue, ranked by risk, batchable where safe, answerable from any device. Risk tiers: **Low** = read-only or reversible-inside-the-workspace → batchable; **Medium** = writes to the person's own stores → owner approves individually; **High** = outward or irreversible (send, publish, push to shared, metered spend, permission changes) → never batchable, owner approves, operator co-approves when platform-level. Plans always carry both **Approve** and **Re-plan** actions. Any approval pending past a threshold is auto-flagged "assumptions may be stale" with one-click re-plan. Answering an item resumes the paused work in place (main list 4.3); the inbox is the single place where "the platform needs you" lives.

S3.3 **The visual wiring display (workforce map).** A readable graphical view of how the workforce is built: every worker that exists, what each is equipped with (tools, knowledge, permissions, helpers), and how multi-stage procedures connect step by step. Its purpose is audit and understanding — an advanced user can see exactly what the machinery is, not just what it did. Editing the machinery *through* this view is explicitly a later, optional capability (15.5); viewing comes first.

S3.4 **The task board and task detail** as described in S1.3–S1.7 — the daily driver surface.

S3.5 **Review surfaces for every deliverable type.** Side-by-side and inline comparison for code and text; document and PDF changes shown as readable differences; images compared visually; comments anchored to exact places; revision-over-revision navigation (main list Section 6).

S3.6 **The conversational assistant as a first-class surface:** a chat that knows the platform's live state — ask status questions, get summaries, launch tasks — available inside the dashboard and from mobile.

S3.7 **Notifications that carry the decision.** Pending items push to the right person and deep-link straight to the answerable card; the common oversight loop (notified → glance → decide) completes on a phone in seconds — while the host is up (Operating reality).

S3.8 **Multi-channel ingress (a later phase — see 10.3, 15.5).** Tasks can also enter through familiar channels — a message to a bot in a chat app (e.g., a Telegram/Slack/Discord-style messenger), or an event from a connected repository — not only through the web workspace. Every entrance goes through the same intake, the same attribution and budget rules (3.4), and the same gates; results and questions flow back on the channel the task came from.

S3.9 **Live by default.** All surfaces update in real time as state changes; nothing requires manual refresh to be current.

S3.10 **Multi-user aware throughout.** Every surface respects identity: people see their own work and what's shared with them; approvals route to the person who owns the decision; the fleet view distinguishes whose account is burning.

---

## S4. Full memory requirements
*(expands main list 8.4, 8.5, 8.6, 8.7, 8.8; connects to 8.1–8.3)*

S4.1 **Per-user memory.** The platform remembers each person — their preferences, recurring context, style, and history — and applies it automatically to their tasks. This memory belongs to the person: they can see what is remembered about them and have it corrected or removed.

S4.2 **Worker memory with defined lifetimes.** What a worker remembers is layered by design: short-lived working notes that exist for the task at hand and then expire (the per-run instance, 7.8); a medium-term experience record of what the worker has done and encountered, kept as compact summaries (the per-user overlay, 7.8); and permanent rules taught explicitly by the operator, which always apply.

S4.3 **Defined writers per layer.** Each memory layer has a defined author: workers write their own working notes and experience records; only humans write permanent rules; nothing writes into a layer it doesn't own. Memory therefore cannot silently accumulate garbage or self-reinforce errors.

S4.4 **One shared project truth.** All workers — regardless of which task, session, or underlying service runs them — read the same current project knowledge and task state. No worker ever acts on a stale or private version of reality, and an update made by one worker is immediately the truth for all. Project knowledge is shared among that project's members (8.8).

S4.5 **The house knowledge base.** Durable knowledge lives in a curated, versioned collection: domain playbooks, style and voice guides, business context, quality rubrics, and reference examples of excellent past work. Changes to it happen only through operator approval (D10, main list 8.3), and its history is inspectable.

S4.6 **The right knowledge arrives by itself.** Every task automatically receives the relevant slice of accumulated knowledge — the brand voice for content work, the conventions and danger zones of the repository being touched, the domain playbook — without the requester attaching anything. What was provided to a run is visible in its trace (auditable memory use).

S4.7 **Outcome memory: wins and lessons.** The platform keeps a ledger of real outcomes — successes worth repeating and corrections worth remembering. Lessons are proposed automatically from evidence (edits, comments, accepted and rejected work) but adopted only with human approval (main list 8.1); proven excellent results can be promoted into reusable reference examples.

S4.8 **Reversible, attributable learning.** Every adopted lesson carries its provenance and can be individually removed, cleanly undoing its influence on future work — a bad lesson cannot permanently poison the workforce.

S4.9 **Scoped by default, shared by choice.** A lesson learned on one worker applies to that worker's template overlay for that user unless deliberately generalized; a lesson or personal knowledge entry from one person applies to another person's work only if that person sees it and adopts it (main list 8.8) — with its origin visible.

S4.10 **Forgetting is a feature.** Expiry of short-lived memory, pruning of stale experience, and removal of superseded knowledge are designed behaviors, not accidents — the memory system stays curated rather than growing into a landfill.

S4.11 **Declared precedence on conflict.** Explicit task specification > project truth > personal preferences > house defaults (main list 8.9). Substantive conflicts are surfaced to the requester as a question, never silently resolved.

---

## S5. Confinement classes (the isolation ladder)
*(expands main list 4.1, 4.4, 4.7; governed by D2)*

S5.1 **C0 — Connectors.** No model in the loop: fixed, deterministic API calls to outside services (2.6). Per-service scoped credentials; network egress to that one service only.

S5.2 **C1 — Trusted reasoning.** Model works on local, trusted inputs only. Workspace-scoped filesystem; no network egress; no credentials.

S5.3 **C2 — Workspace-write.** C1 plus a writable clone of the registered repository or document store (S1.6). Egress only to allowlisted package registries; every outward push or publish is a proposal (4.2).

S5.4 **C3 — Web-reading.** Fetch and search through an egress allowlist. No credentials inside the tool sandbox — provider sessions live outside it (D2). Everything fetched is treated as data, never as instructions; output from C3 workers receives tighter verification.

S5.5 **C4 — Web-acting (future, 12.2).** Per-site scope, per-action approval, disposable browser profile, full action log. The tightest class; nothing graduates into it silently.

S5.6 **The honest caveat.** Injected content can still steer a model's reasoning inside its sandbox — that risk is not fully removable by any confinement. The platform's guarantee is blast radius: no credential access, no un-proposed outward effect, and a verification gate judging potentially steered output (4.7). Containment, not immunity.

---

**In one line:** a factory for supervised AI work — any knowledge work, for several people, on the subscriptions they already pay for — dependable enough to run while the host is up, honest enough to audit, and self-extending enough that it grows new workers instead of the operator writing them.