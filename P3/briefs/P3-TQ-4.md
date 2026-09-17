> **EXPIRED 2026-09-18 (TQ-4a half) — merged into main (grounding `84b5ee9`, impl `d3a58c5`, drain r1 `59303a8`, coordinator inline `7035f8b`; evaluation FAIL → drained, re-check FAIL on one LOW → inline pin; CONVENTIONS §79). Single-use artifact: never read as truth; code + spec are the truth. Superseded at drain: `packFromCapture` composes PER SLOT from `EffectiveCommands` with per-check `Origin` (R8 as written dropped detected rungs beside one owner slot); F5's rule applies to an INSPECTABLE tree only; R17 (rubric v4) is NOT landed — it is the RUBRIC-V4 packet with the B13 re-run. §TQ-4b remains the gate's (item B14).**

# P3-TQ-4 — the execution rung at bootstrap (amendment A16; findings TQ-F3 + TQ-F7's platform half)

Grounded 2026-09-17. Binding contract: `Spec/core-architecture-v1.md`; canonical text `Spec/drafts/`.
Amendment **A16** (`Spec/drafts/S00-front-matter.md:196`), marker sites `S07-verification-quality.md:37`
(S07.3 code-ladder), `:120` (S07.8 bootstrap bullet), `S13-deliverables-review-git-backup.md:109`
(S13.7 re-scan line). Findings `P3/design/taskquality-webshop-findings-2026-09-16.md:19` (TQ-F3),
`:27` (TQ-F7).

**This brief scopes TQ-4a only.** TQ-4b (§TQ-4b below) is the adoption question the coordinator
carries to the gate; nothing in §1–§18 depends on its answer.

---

## 0. The defect, in one line

A task that created a project's entire build system was verified by two judge reads and nothing
else: the executor's scaffold declared `vite build` and `node --test tests`, the plan wrote
machine-walkable given/when/then lines per AC, and the platform ran none of it — because the S13.7
registry capture could only be filled by hand, and the A14 bootstrap posture therefore had no rung
of its own (`internal/verify/bootstrap.go:127` `bootstrapV1` emits four UNVERIFIABLE-HERE rungs and
consults no pack checks).

The live reference tree is on disk, read-only:
`~/.sinet-rework-sitting/projects/worktrees/interview-test1/t-3120e8e3d14591d3` — 23-file React/Vite
app, `package.json` scripts `dev: vite` / `build: vite build` / `preview: vite preview` /
`test: node --test tests`, **no `lint` script**, **no `node_modules`**, `tests/catalog.test.js` +
`tests/cartReducer.test.js`.

---

## 1. What the code does today (file:line, verified this session)

| Fact | Where |
|---|---|
| `Store.Scan(store, defaultBranch) (Draft, error)` — the S13.7 onboarding scan. Pure over the filesystem, safe on a worktree, `.git` never inspected, **read-only**. | `internal/project/scan.go:70` |
| Toolchain rules propose commands **from the marker file alone** — `package.json` ⇒ `npm run build` / `npm test` / `npm run lint` / `npm start` / `npm run preview`, regardless of which scripts exist. | `internal/project/scan.go:33-41` |
| `scanHash(d)` fingerprints the drafted commands + conventions + zone hashes; `DriftCheck` compares against it. **Changing `Scan`'s output changes every registered project's drift baseline.** | `internal/project/scan.go:157` |
| `Commands{Build,Test,Lint,Run,Preview}` — **no `Dev` slot**. | `internal/project/project.go:51` |
| `Capture{Version,Conventions,Commands,DangerZones,ScanHash,Family,CapturedBy,CapturedTS}` — immutable versioned content. | `internal/project/project.go:62` |
| `EditCommands(ctx, projectID, by, cmds)` — owner-only, active-only, full replacement, **carries every other member forward byte-equal**, retry-safe (equal set ⇒ no version, no event), `Origin: OriginEdit`. | `internal/project/registry.go:403`, `:421`, `:424-433` |
| `validCommands` iterates `commandSlots` (single line, no NUL, valid UTF-8, ≤ `commandMaxRunes`). | `internal/project/registry.go:357` |
| The Commands door: `POST /api/projects/{project}/commands`, full replacement, object-is-the-unit, unknown slot keys 400 **with the vocabulary reflected from `ProjectCommands`' own json tags**. | `internal/api/projects.go:811-830`, `:183` |
| `ProjectCapture` serves conventions + commands + danger zones to every member who can see the entry. | `internal/api/projects.go:200-213` |
| `CheckPackFor(ctx, domain, taskID)` → `packFromCapture` → `packChecks(e.Capture.Commands)`; empty ⇒ `verify.BootstrapPack(domain, version)`. | `internal/shell/project_seams.go:316`, `:468-481` |
| `packChecks` maps lint+build ⇒ `StageStatic`, test ⇒ `StageUnit`, argv `/bin/sh -lc <cmd>`, `FindingCategory: CatACBlocker`, **no `ACKey`, no `StepID`** (it is already not an acceptance check). `run`/`preview` are deliberately not checks. | `internal/shell/project_seams.go:509-533` |
| `BootstrapPack` carries **no checks by construction**; `executes()` is `len(Checks)>0`; `bootstrap()` is `Posture == PostureBootstrap`. | `internal/verify/bootstrap.go:54`, `:59`, `:63` |
| `validateInput`: `if pack.executes() && v.Runner == nil` ⇒ **PreambleRefusal** (parks the run behind a card). | `internal/verify/pipeline.go:287` |
| The bootstrap V1 branch materializes the workspace exactly as the pack branch does, then calls `bootstrapV1(pack, in.Steps, in.Coverage, ws)`. | `internal/verify/pipeline.go:466-484` |
| `bootstrapV1` emits `ladder:<stage>` UNVERIFIABLE-HERE for all four of `ladderOrder` with `BootstrapAttribution = "check-pack:absent"`, then decides PLAN contracts from the tree (P3-TQ-3, §74). | `internal/verify/bootstrap.go:127-152` |
| `ladderOrder = {static, unit, smoke, e2e}` — cheap-first. | `internal/verify/v1.go:64` |
| `RunV1` derives every verdict from the exit status platform-side (rule 3), stops later stages after the first failing stage with first-upstream-failure attribution, records `EvidenceRef`/`EvidenceSHA`. | `internal/verify/v1.go:411-495` |
| `SandboxCheckRunner` composes via `adapters.Confiner` at class **`"C2"`** (default), `cmd.Dir = req.Workspace`. | `internal/verify/v1.go:341`, `:363`, `:385` |
| `VerificationWorkspace` = a locked utility checkout at the revision's snapshot pin, then a **`.git`-less throwaway copy** (§59; S07.3 rule 1 / P-T06-2). Empty dir + nil error is the honest absence. | `internal/shell/project_seams.go:335-349` |
| Sandbox `Compose` ro-binds `{/usr,/bin,/sbin,/lib,/lib64,/etc/alternatives,/etc/ssl/certs}` into **every** class; C1 = `WorkspaceMode "ro"`, `Network NetNone` (empty netns, no route, no proxy). | `internal/sandbox/sandbox.go:484`, `:217-218`, `:128` |
| `internal/preview` composes the dev-server plan and then **cannot serve live**: `launchSandboxed` ends at `markUnavailable(deferredReason("dev-server"))` — the host substrate *and* the spawn/probe/route activation code are both missing. | `internal/preview/manager.go:336-377` |
| `Store.Rescan` — the landed on-demand re-scan: new capture version, family carried forward, never re-derived. **The precedent `RescanDetected` follows.** It has no HTTP door. | `internal/project/onboard.go:231-253` |
| `preview.Detect` already derives a **dev** command heuristically: `package.json` → `npm run dev`, `pnpm run dev` with a pnpm lock. A **separate table** from `project.toolchainRules` — the preview one detects *dev*, the registry scan detects build/test/lint/run/preview. | `internal/preview/disposition.go:76`, `:108-121` |
| `validCommands` accepts an **all-empty** set — that is the sanctioned road back to the bootstrap posture. `commandMaxRunes = 500` (runes, not bytes). | `internal/project/registry.go:321`, `:357` |
| `ValidateAxis1` binds a mechanical fact only when the `"AC-<n>"` key is present in the V1 map **and** the state is exactly `PASS` or `FAIL`. `ACOutcomes()` only admits outcomes with `ACKey != ""`. | `internal/verify/v2.go:250`, `:269`; `internal/verify/v1.go:290-295` |
| `intake.AC{N, Plain, Structured, StructuredKind}` — `StructuredKind` is `"ears" | "gwt" | ""`. | `internal/intake/artifact.go:37-41` |
| `memory.selectorsMatch` **refuses any entry declaring `Domain`, `TaskType` or `Triggers`** — such entries never inject. A selector-free house L2 entry injects into every task. | `internal/memory/source.go:196-204` |
| The composer playbook is a governed house L2 object, selector `machinery:worker-composer`, and its own comment states it **"never injects into stage briefs"**. | `internal/memory/seeds.go:205-212`, `internal/worker/playbook.go:17-31` |
| `SeedSoftwareRubric()` v3, four axis-2 items, each pinned to one of the four ratified probes; `GoldenSet{TPR:1.0, TNR:0.5, Measured:true, MeasuredOn:"2026-09-17"}`. | `internal/verify/seeds.go:122-175` |

---

## 2. The seven questions, settled

**(1) What rescan exists, and where does provenance go.**
`Store.Scan` (`scan.go:70`) is the S13.7 heuristic scan and is already read-only and worktree-safe;
there is **no** rescan-at-verify path and no provenance field anywhere in `Capture`. A16 requires the
detected set to be *requester-visible and editable through the Commands door* while *a hand-captured
command always outranks a detected one, per command*. Two members, never one: `Capture.Commands`
(the owner's) and `Capture.Detected *Commands` (the platform's). Merging them would make the
precedence rule unrecoverable after one write and would let a detected command graduate the project
through `packChecks`. Precedence is resolved **at read time**, per slot, in exactly one place
(`EffectiveCommands`). The audit axis is `CaptureInput.Origin` — a third value `OriginDetected =
"detected"` beside the landed `OriginEdit = "edit"` (`registry.go:231`), so the `registry.captured`
trail distinguishes *the platform noticed this* from *someone decided this*.

**(2) How the ladder resolves a pack per revision, and the smallest change.**
`resolvePack` → the `CheckPackFor` seam → `packFromCapture` (`project_seams.go:468`): `packChecks`
over the **hand-captured** commands; empty ⇒ `BootstrapPack`. The smallest change is one branch in
`packFromCapture`: when `packChecks(Commands)` is empty **and** `Detected` holds a rung, return the
bootstrap pack carrying `Checks: detectedChecks(*Detected)` and `Provenance: ProvenanceDetected`.
Realized as evidence by construction — `packChecks` already sets **no `ACKey` and no `StepID`**
(`project_seams.go:521-527`), so `ValidateAxis1` can never bind one and `stepContracts` can never
let one decide a PLAN contract; `Posture` stays `PostureBootstrap`, so the landed advisory-V2 /
mandatory-V3 / `SetVerified`-never path (§70) is untouched. **Graduation is decided from
`Commands` alone and never from `Detected`** — that is the whole of "detected commands do not
graduate the project".

**(3) Rule-1 confinement for running detected commands.**
The rungs run in the **existing** verification sandbox through the **existing** `CheckRunner`
(`SandboxCheckRunner`, class `"C2"`, network-off today because egress classes ride an empty netns
until the deferred host substrate lands — §12/§15, and P-T06-2 makes network-off mandatory
permanently). Nothing here loosens confinement and no egress exception is ever requested. The
workspace is §59's `.git`-less throwaway copy, so rule 1's history-strip already holds.

Consequence, and it is the live case: a scaffold's `node_modules` is not in the reviewed tree, so
`npm run build` → `vite build` **cannot run**. Exit-code-classifying that would produce a FAIL that
blames the work for a platform condition. So the precondition is decided **before execution, from
the tree**, and the rung records UNVERIFIABLE-HERE with its reason (R10). Two preconditions, both
decidable, both npm-shaped because `package.json` is the only manifest the detected path consults:

- **P1 — the script exists.** `npm run <s>` / `npm test` is detected only when `package.json`'s
  `scripts` declares `<s>`. This is why `Store.Scan` alone is not enough: it proposes
  `npm run lint` for the live tree, which has no `lint` script, and running it would fail for a
  reason that has nothing to do with the work.
- **P2 — the script's tool is resident.** The script's first word must be a host-resident
  interpreter (`node`, `npm`, `npx`, `sh`, `bash` — all under `/usr`, which `Compose` ro-binds into
  every class, `sandbox.go:484`); anything else resolves from `node_modules/.bin` and needs
  `node_modules/` present in the tree. `node --test tests` passes P2; `vite build` does not.

Non-npm toolchains keep `Scan`'s proposal unrefined — `go build ./...` / `go test ./...` /
`go vet ./...` need only the host toolchain, which is bound. An honest cut, not a gap.

**(4) The walk-step schema.**
Derived from the frozen ACs' **structured** sub-line only (`intake.AC.Structured` with
`StructuredKind == "gwt"`; S06.6: verification binds to the sub-line where one exists) — never from
the plain line, the executor's report, or the deliverable text. `Given X, when Y, then Z` splits
deterministically into three `WalkStep`s: `navigate` / `act` / `assert`, each carrying its verbatim
clause. `WalkStep.Composable` is a separate fact from the clause, because turning prose into an
executable step is not deterministic; an uncomposable step carries no `Target`/`Expect` (it names no
state it could not have read) and its AC's `Reason` says why.

- An AC **with no gwt sub-line** ⇒ `Walkable: false`, zero steps, a plain-words `Reason`. It is a
  criterion the walk does not cover; the judge weighs the plain line at V2. In outcome vocabulary
  that is **N-A**, and N-A is only ever reachable *once the walk has actually run* — which is why
  today, with no driver, every AC is **UNVERIFIABLE-HERE**, not N-A.
- Recorded outcome: `WalkOutcome{ACKey, State, FailedStep, AttributedTo, Detail}` — S07.3's
  contract vocabulary unchanged (PASS / FAIL / N-A / UNVERIFIABLE-HERE) with
  first-upstream-failure attribution at walk granularity (`FailedStep` = the first failing step
  index, `-1` when none).
- Assertions are on **DOM state, never rendered pixels** (TQ-F7: on this host a frame-starved tab
  renders nothing while the DOM is entirely correct; a pixel assertion would call that app broken,
  and the inverse — an app that never paints — is the executor quality rule's job, not this rung's).

**(5) Where the TQ-F7 executor quality rule lives.**
Two governed L2 knowledge objects, both through the 8.3 gate, following §57's pattern exactly
(frozen snapshot + supersession under its own provenance + a build-time content digest; **never** a
live pointer):

- **Judge half — rubric.** A fifth item `axis2/frontend-feedback` on
  `SeedSoftwareRubric()` under the ratified probe `ProbeImplicitExpectations`, bundle `Version` 3→4,
  `VerifiedOn` restamped. Domain-scoped by construction (`Domain: DomainSoftware`), so it needs no
  selector. Reaches the judge through the landed rubric path (`pipeline.go:271`).
- **Executor half — playbook.** `ComposerPlaybookSeed()` gains a *Web deliverables* section;
  `ComposerPlaybookSeedVersion` `seed-1` → `seed-2`. **The reach is indirect and the brief says so
  rather than pretending otherwise:** the playbook steers the S08.6 composer
  (`internal/stage/compose.go:67-69`), the composer drafts the worker template, and the template's
  behavioral content is what reaches the executor. There is no direct route today —
  `memory.selectorsMatch` (`source.go:196`) refuses every entry declaring a domain or task-type, so
  a selector-scoped house entry injects into **nothing**, and a selector-free one would inject a
  frontend rule into every task in the house. Widening `selectorsMatch` is a ratified-behavior
  change (§17) and is **out of scope**.

Rule content (verbatim intent, TQ-F7): correctness feedback must be visible on the **first frame**
without an animation completing; no `AnimatePresence mode="wait"` gating between views; respect
`prefers-reduced-motion`; **every action leaves DOM-observable state** (a cart badge is a number in
the DOM, not only a pop animation) — the last clause is what makes the rule and the walk agree.

**(6) ⚙ settings: none.** No new key, no default or clamp moved — A16 itself says so
(`S00-front-matter.md:196`: "No ⚙ default or clamp moves → no S18 re-sweep; tally 118/33"). The
118/33 tally stays green. Structural constants introduced, each with its reason, all flagged to the
settings-tab ledger (the `settings-tab: see & change everything` standing item):

| Constant | Value | Reason it is structural, not ⚙ |
|---|---|---|
| `hostResidentInterpreters` | `{node, npm, npx, sh, bash}` | It describes what `Compose`'s ro-bind set actually contains (`sandbox.go:484`) — a fact about the sandbox profile, which S11 declares STRUCTURAL and versioned in code (§12). An operator turning it into a number could only make the platform lie about its own mount table. |
| `detectedDepsDir` | `node_modules` | Same class: a property of the npm toolchain, not a preference. |
| `walkStepTimeout`, `walkACCap` | TQ-4b | A16 pre-declares these two as "at most two structural constants → the settings-tab ledger". Not introduced at TQ-4a; named here so TQ-4b does not re-litigate. |

**A new ⚙ default appearing during implementation is a STOP** (S00.9 + the S18 sweep). None is
expected.

**(7) Invariants.** Each is a named test in §4:
(i) a detected pack never graduates the posture; (ii) a hand-captured command outranks a detected
one **per slot**; (iii) a rung the sandbox cannot compose is UNVERIFIABLE-HERE, **never PASS and
never FAIL**; (iv) the rescan is idempotent and read-only on the tree; (v) a detected rung carries no
`ACKey` and no `StepID`, so it reaches neither `ValidateAxis1` nor a PLAN contract; (vi) `Store.Scan`
is byte-unchanged, so no registered project's `scanHash` drift baseline moves.

---

## 3. Requirements

Each numbered requirement carries its S-ref. The executor implements exactly these.

### A. The capture side (S13.7 A16)

**R1 [S13.7 A16].** `project.Capture` gains `Detected *Commands` (`json:"detected,omitempty"`).
`nil` = no scan has ever proposed commands for this project — an honest absence, distinct from
"scanned and found nothing" (a non-nil empty `Commands`). Every capture written before this packet
reads back `nil`. No migration: `captured.commands`/content is a JSON column.

**R2 [S13.7 A16].** `project.Commands` gains `Dev string \`json:"dev,omitempty"\`` — A16 names
"build/test/lint/**dev**" and the walk is served by the dev command. Add the slot to `commandSlots`
(`registry.go`) so `validCommands` covers it and to `api.ProjectCommands` so the Commands door's
reflected vocabulary accepts it (`projects.go:183`). Detect it from the tree's own `scripts.dev`.
`internal/preview/disposition.go:109-115` already derives a dev command heuristically, but
**`internal/project` must not import `internal/preview`** — A16 keeps the preview and its host
substrate on their own row, and the two tables answer different questions (the preview's is a
*launch* disposition, this one is a *capture* slot). One line read from the manifest, not a shared
abstraction. **No consumer at TQ-4a** — declared inert surface for TQ-4b.

**R3 [S13.7 A16; S07.3 rule 1].** `project.RescanDetected(ctx, projectID, by, tree string)
(Capture, bool, error)` — the A16 re-scan.
- Derives the detected set as `Store.Scan`'s toolchain proposal **refined against the tree**: for
  the `package.json` toolchain, a slot survives only when the named script exists in `scripts`
  (P1), and `Dev` is detected from a `dev` script. Other toolchains take `Scan`'s proposal as is.
- **`Store.Scan` itself is byte-unchanged** — changing it would move every registered project's
  `scanHash` drift baseline (`scan.go:157`) and rewrite drafts a human already approved.
- **Read-only on the tree.** Nothing writes, nothing stats outside it, `.git` is never inspected.
- **Idempotent.** A detected set byte-equal to the current one mints no version and appends no
  event, returning `minted=false` — the landed `EditCommands` retry-safety precedent
  (`registry.go:421`); otherwise a drain that re-enters the verify leg grows the capture history one
  version per round.
- Mints with `Origin: OriginDetected`, carrying conventions / commands / danger zones / scan hash /
  family forward **byte-equal**.

**R4 [S13.7 A16].** `project.EffectiveCommands(c Capture) Commands` — per slot, the hand-captured
command when non-blank, otherwise the detected one. The entire precedence rule, in one place, so no
consumer re-derives it.

**R5 [S13.7].** `EditCommands` carries `Detected` forward byte-equal (`registry.go:424-433`), like
every other non-`Commands` member. An owner edit never wipes what the platform noticed.

**R6 [S13.7 A16; S15.2].** `api.ProjectCapture` serves `detected` — requester-visible. The Commands
door stays a `commands`-only writer: an owner writes their own slot, and the platform's detected set
is not theirs to type. Editing is *through the Commands door* exactly as A16 says — the owner types
the command they want and it outranks the detected one by R4.

### B. The ladder side (S07.8 A16, S07.3)

**R7 [S07.8 A16; S07.3].** `verify.CheckPack` gains `Provenance Provenance`
(`""` = the owner's captured pack; `ProvenanceDetected = "detected"`). Distinct axis from
`Check.Provenance`, which records an acceptance check's separate authoring context (S07.3 rule 4) —
the doc comment must say so. **Placed in the type region of `v1.go`, away from `stepContracts` and
`RunV1`'s tail** (P3-TQ-6's executor is concurrently editing those).

**R8 [S07.8 A16].** `packFromCapture` (`project_seams.go:468`): when `packChecks(Commands)` is empty
**and** `Capture.Detected` holds a build/test/lint rung, return
`BootstrapPack(domain, version)` with `Checks` = the detected rungs (ids prefixed `detected:`, so
the record says what kind of rung it is) and `Provenance: ProvenanceDetected`, `VerifiedOn` = the
capture date (S07.3 rule 7: a suite is exactly as fresh as the scan it came from). Graduation still
reads `Commands` alone.

**R9 [S07.8].** `validateInput` (`pipeline.go:287`) — the runner guard becomes
`pack.executes() && !pack.bootstrap() && v.Runner == nil`. A bootstrap round **never parks** for want
of a runner (S07.8: "never a verification refusal and never parks the run"); its rungs record
UNVERIFIABLE-HERE instead. One named line.

**R10 [S07.8 A16; S07.3 rules 1/3].** `bootstrapV1` gains `ctx` and a `CheckRunner` and runs the
detected rungs:
- **cheap-first** over `ladderOrder`, pack order within a stage;
- the verdict is derived **platform-side from the exit status** (rule 3), never in-band;
- first-upstream-failure attribution: after a stage fails, later stages' rungs are UNVERIFIABLE-HERE
  attributed to the first failing check id;
- `EvidenceRef`/`EvidenceSHA` recorded as the pack branch does (S07.3 "evidence artifacts
  retained").

**R11 [S07.3 rule 1].** Precondition **before** execution, decided from the tree (§2(3) P1/P2), plus
"no runner wired": the rung is recorded `CheckUnverifiable`, `AttributedTo: DetectedUnrunnable`
(`"command:unrunnable"`), with a plain-words `Detail` naming the actual reason. It is **never handed
to the runner**, never PASS, never FAIL, and never the occasion for an egress exception.

**R12 [S07.3 rule 4; S07.5].** A detected rung is EVIDENCE: `ACKey` empty (so `ValidateAxis1` never
binds it and `V1Result.ACOutcomes()` never lists it) and `StepID` empty (so it decides no PLAN
contract — TQ-3 owns those, §74). It raises **no finding of its own**: it cites no frozen criterion,
and S07.5 is explicit that a finding citing none can only be a note. The outcome reaches the judge
through the landed `verify/v1-outcomes` evidence slice and the requester through the durable round
record and mandatory V3.

**R13 [S07.8].** Rung/stage accounting: a ladder stage a detected rung covers is reported **once**,
by that rung; a stage no detected rung covers keeps the `BootstrapAttribution`
(`"check-pack:absent"`) placeholder with its landed sentence. Nothing is silently skipped, nothing
reported twice.

**R14 [S07.8 A16; §70].** Detected commands **never graduate**: `RoundRecord.Posture` stays
`PostureBootstrap`, `ReviewMandatory` stays true at every stakes tier, `VerifiedItems` stays empty on
an advisory SHIP, and the `BootstrapPostureNote` disclosure still rides the round. The disclosure
sentence is **unchanged** at TQ-4a — it is still true (no build/test/lint command is *captured*),
and rewording requester copy is not this packet's business.

### C. The walk (S07.3 A16)

**R15 [S07.3 A16; S16.4].** The e2e rung on a **web-shaped** bootstrap tree records
`CheckUnverifiable`, `AttributedTo: WalkUnavailable` (`"walk:no-browser-adopted"`), `Detail:
WalkUnavailableReason` — the honest reason (no browser is adopted for verification walks yet),
never PASS and never the generic absent-command sentence, which would be a false explanation.
Web-shaped is decided from the tree: an `index.html` at its root (the S13.8 static/dev-server
disposition marker). A non-web bootstrap tree keeps the absent-command placeholder, because there
the missing thing really is a command. `internal/verify` must **not** import `internal/preview`
(§15's import discipline) — the marker check is one local predicate.

**R16 [S07.3 A16; S06.6].** The walk **interface**, shipped so TQ-4b plugs in without re-deciding
anything: `WalkStepKind{navigate,act,assert}`, `WalkStep{Kind,Clause,Target,Expect,Composable}`,
`WalkState{PASS,FAIL,N-A,UNVERIFIABLE-HERE}`, `ACWalk{ACKey,Walkable,Steps,Reason}`,
`WalkOutcome{ACKey,State,FailedStep,AttributedTo,Detail}`, and
`WalkDriver interface{ Walk(plan []ACWalk, workspace, devCommand string) ([]WalkOutcome, error) }`.
`WalkPlanFor(spec intake.Spec) []ACWalk` is **pure and deterministic over the spec** — same spec,
same plan — with an entry for **every** frozen AC (walkable or not), splitting only the
`StructuredKind == "gwt"` sub-line. A nil driver is the honest absence, not a degraded mode.

### D. Knowledge content (S07.10, S08.6, S09.8)

**R17 [S07.10; S09.8].** `SeedSoftwareRubric()` → `Version: 4`, new item
`{ID: "axis2/frontend-feedback", Probe: ProbeImplicitExpectations}` with pass/fail behavioral
anchors carrying the TQ-F7 rule. `VerifiedOn` restamped to the packet date.
**Golden-set honesty (bounded, and this is the one judgement call in the packet):** the v3 rates were
measured against v3 *content* (`seeds.go:131` states the v3 bump moved only the pin). A content
change makes them no longer a measurement of this bundle. P-T06-5 gates *judge-model* changes, not
content changes, so this is **not** a STOP — but the bundle must not claim a measurement it does not
have. Land: carry the rates forward with `MeasuredOn` unchanged and extend the bundle's own note to
say, in one sentence, that the 26-case set was last run against v3 content and a re-run is owed.
**Do not run the golden set** — it is a paid leg needing explicit opt-in (§59; operator standing
directive). Flag the re-run as a gate item.

**R18 [S08.6; S09.8; §57].** `ComposerPlaybookSeed()` gains the web-deliverable feedback section;
`ComposerPlaybookSeedVersion` → `"seed-2"`. Governed by **supersession under this packet's own
provenance** — a new `EnsureTQ4KnowledgeGovernance` in `internal/memory`, minting a new version and
retiring the old via `supersedes_id` (S09.8), pinned by a build-time content digest, deferring with a
log until an operator account exists (D10). **Do not extend `EnsureRW12TaxonomyGovernance` and do not
touch its digests** — §57 states that rule by name: *"For the next packet that edits a question set:
do not update the digests, and do not extend this Ensure — mint your own, with your own provenance."*
The same applies to the rubric snapshot (`b2Seeds()` ships `verify-rubric-software.json`,
`internal/memory/seeds.go:88`): a governed object's content is what its **row** was committed with,
never what is on disk, so the equality check reads the content hash from the `knowledge.write` event.

---

## 4. Acceptance tests (committed RED at grounding)

Run each with the filter, one package at a time — never the full battery, never
`./internal/stage/` unfiltered:

```
go test -p 1 -count=1 -run 'TestTQ4' ./internal/verify/
go test -p 1 -count=1 -run 'TestTQ4' ./internal/project/
```

### `internal/verify/tq4_evidence_test.go`

| Test | Binds | Red because |
|---|---|---|
| `TestTQ4DetectedRungsRunAsEvidence` | R10, R13 | `bootstrapV1` never consults `pack.Checks`; the runner is never called. |
| `TestTQ4DetectedRungIsEvidenceNeverTheACVerdict` | R12 | no `detected:` outcome is recorded at all. |
| `TestTQ4UnrunnableDetectedRungIsUnverifiableNeverPass` | R11 | no `detected:` outcome is recorded at all. |
| `TestTQ4DetectedCommandsNeverGraduateThePosture` | R14 | no `detected:` outcome is recorded at all. |
| `TestTQ4RungWithoutADetectedCommandKeepsItsHonestPlaceholder` | R13, R15 (negative half) | the covered stage records only the placeholder. Also binds that a **non-web** tree's e2e rung keeps the absent-*command* reason — blaming an unadopted browser for a Go CLI is a false explanation in the other direction. |
| `TestTQ4WalkRungIsUnverifiableWithItsRealReason` | R15 | the e2e rung is attributed `check-pack:absent`. |
| `TestTQ4MissingRunnerDoesNotParkABootstrapRound` | R9, R11 | `validateInput` refuses with a PreambleRefusal. |

### `internal/verify/tq4_walk_test.go`

| Test | Binds | Red because |
|---|---|---|
| `TestTQ4WalkPlanSplitsTheFrozenGwtSubLine` | R16 | `WalkPlanFor` returns nothing. |
| `TestTQ4ACWithoutAGwtSubLineIsNotAWalkFailure` | R16 | `WalkPlanFor` returns nothing. |
| `TestTQ4WalkPlanIsPureOverTheSpec` | R16 | `WalkPlanFor` returns nothing. |
| `TestTQ4UncomposableStepIsNeverAPass` | R16 | `WalkPlanFor` returns nothing. |

### `internal/project/tq4_detected_test.go`

| Test | Binds | Red because |
|---|---|---|
| `TestTQ4RescanDetectsOnlyScriptsTheTreeDeclares` | R2, R3 | `RescanDetected` is not built. |
| `TestTQ4HandCapturedOutranksDetectedPerSlot` | R4 | `EffectiveCommands` returns the hand-captured set alone. |
| `TestTQ4RescanIsIdempotentAndReadOnly` | R3 | `RescanDetected` is not built. |
| `TestTQ4EditCommandsCarriesDetectedForward` | R5 | `RescanDetected` is not built (the precondition cannot be seeded). |
| `TestTQ4RescanIsDistinguishableFromAnOwnerEditInTheAuditTrail` | R3 | `RescanDetected` is not built. |

**16 tests, all committed RED at grounding, every one failing for its stated reason.** Two of them
(`…IsEvidenceNeverTheACVerdict`, `…NeverGraduateThePosture`) were vacuous as first written — a
posture that records nothing satisfies every claim about what it records — and now assert the
detected rung **exists and ran** before asserting what it does not carry.

**What the grounding commit also contains, and why `go build ./...` stays green (§3 carve-out):**
the minimal **inert type surface** the red tests need in order to compile —
`project.Commands.Dev`, `project.Capture.Detected`, `project.OriginDetected`,
`project.EffectiveCommands` (returning today's behavior), `project.RescanDetected` (a not-built
sentinel), `verify.CheckPack.Provenance`, and `internal/verify/evidence.go`'s constants and walk
schema with `WalkPlanFor` returning nil. **Type surface only — no behavior.** `commandSlots`,
`api.ProjectCommands` and `packFromCapture` are deliberately NOT touched at grounding, so the
Commands door's reflected vocabulary and every capture's stored bytes are unchanged until the
executor lands R2/R6/R8.

The `internal/verify` fixtures use the landed `tq3_contracts_test.go` / `bootstrap_gf4_test.go`
harness (`newFix`, `f.seedTask`, `f.verifier`, `f.events`, `input`, `deliverable`, `spec`,
`scriptRunner`, `writeTree`); the `internal/project` fixtures use `harness_test.go`'s `newFix` and
`editcommands_gf5_test.go`'s `gf5Seed`. **No new test dependency, stdlib `testing` only** (§3).

The webshop fixture tree is the live one, verbatim in shape: `package.json` with
`{build: "vite build", test: "node --test tests", dev: "vite"}`, **no `lint`**, a `tests/` dir, an
`index.html`, and **no `node_modules`**.

---

## 5. Seams, and the stub for the one whose phase has not come

| Seam | Disposition |
|---|---|
| `verify.CheckRunner` (`v1.go:324`) | **existing**, reused unchanged. Detected rungs run through it exactly as captured rungs do. |
| `verify.WorkspaceProvider` / `VerifyInput.Workspace` (`pipeline.go:36`, `:150`) | **existing**, reused unchanged (§59's stripped revision). |
| `stage.Config.CheckPackFor` (`stage.go:272`) | **existing**, reused; `packFromCapture` gains the detected branch behind it. `internal/stage` never imports `internal/project` (§23) — the wiring stays at the composition root. |
| `verify.WalkDriver` | **NEW, stubbed — its phase has not come.** No implementation exists; a nil driver is the honest absence and every AC records UNVERIFIABLE-HERE with `WalkUnavailableReason`. TQ-4b supplies it. |
| `internal/preview`'s launch/probe code | **not touched.** The dev-server lane composes but cannot serve live (`manager.go:367`); the walk is a second consumer of the same substrate and stays on its own row (A16: "the S13.8 preview and its S11.4 host substrate … stay on their own row"). |

---

## 6. Files expected to change

```
internal/project/project.go       Commands.Dev; Capture.Detected
internal/project/registry.go      commandSlots += dev; EditCommands carries Detected forward
internal/project/detected.go      NEW  OriginDetected, EffectiveCommands, RescanDetected, the P1 refinement
internal/project/tq4_detected_test.go   NEW  red
internal/api/projects.go          ProjectCommands.Dev; ProjectCapture.Detected (read projection)
internal/shell/project_seams.go   packFromCapture detected branch; detectedChecks; the rescan call
internal/verify/evidence.go       NEW  Provenance, DetectedUnrunnable, Walk* schema, WalkPlanFor, WalkDriver
internal/verify/v1.go             CheckPack.Provenance  — ONE field, type region only
internal/verify/bootstrap.go      bootstrapV1 runs detected rungs; the walk rung's reason
internal/verify/pipeline.go       validateInput runner guard (:287); bootstrapV1 call site (:478)
internal/verify/tq4_evidence_test.go, tq4_walk_test.go   red (committed at grounding)
internal/verify/seeds.go          rubric v4 + the frontend-feedback item
internal/worker/playbook.go       seed-2 content
internal/memory/tq4governance.go  NEW  EnsureTQ4KnowledgeGovernance + digests
internal/shell/shell.go           one call site for the new Ensure, beside the landed ones (:431)
P3/CONVENTIONS.md                 §76 at landing (the executor's, not grounding's)
```

**Concurrency discipline — other agents are writing on other branches right now.** Keep every change
to these regions to the **minimal named line** and say so in the commit:
- `internal/verify/{v1.go,v2.go,pipeline.go}` around `stepContracts` / `ValidateAxis1` / the drain's
  judged-round assembly → **P3-TQ-6, executor running now.** This packet touches `v1.go` only to add
  one struct field in the type region, and `pipeline.go` at exactly two lines (`:287` guard,
  `:478` call). Nothing else.
- `internal/review/*`, `internal/api/deliverables.go`, `internal/stage/review_sink.go` → **P3-SIT-1.**
  Untouched here.
- `internal/stage/skeleton.go` recovery regions, `internal/project/workspace.go`,
  `internal/api/reads.go` → **P3-TQ-2.** Untouched here; the rescan deliberately rides
  `project_seams.go` rather than `skeleton.go` for exactly this reason.

---

## 7. Adopted components touched

**None.** No new `components.lock` entry, no pin bump, no modification to any adopted component.
The lock gate (`go run ./tools/lockgate`) must stay green untouched. The rungs run through the
already-ratified sandbox stack (srt primary / native funeral-plan fallback, §12) with **no profile
change, no class change, no new ro-bind**.

The one adoption question this packet raises is **deferred entire to TQ-4b** (§TQ-4b). A16 does not
pre-authorize it: S16.4 requires the full ten-check onboarding gate plus a lock entry, and
checklist #10 means a packet session proposes and a phase gate approves.

---

## 8. CONVENTIONS that bind

- **§2** — stdlib-first (no third-party Go module without an S16 adoption + lock entry in the same
  commit); gofmt-clean, vet-clean; no ⚙ value as a constant; doc comments cite spec sections where
  they clarify a constraint, never research narration.
- **§3** — stdlib `testing` only; colocated `_test.go`; fixtures never write outside `t.TempDir()`.
  **Amendment-A carve-out applies**: this brief's acceptance tests are committed RED, scope limited
  to this packet's own paths, `go build ./...` stays green (an inert type surface to make red tests
  compile is fine — behavior is not), and the red window is declared in the commit message.
- **§4** — adoption rail: exact pins, `components.lock` + gate, never `latest`.
- **§5** — commit subject `P3-TQ-4: <summary> (<spec sections>)`; **stage by explicit pathspec, never
  `git add -A`**; packet sessions never push, never force-push, never edit `Docs/`, `Spec/`,
  `Research/` or `P3/STATE.md`.
- **§12** — sandbox profiles are STRUCTURAL and versioned in code, not ⚙; C1 reads no egress ⚙; the
  boundary is bwrap namespaces + the empty netns. **No profile widening in this packet.**
- **§15** — `internal/verify` imports storage/eventlog/ledger/intake/adapters and **never
  `internal/gates`** (enforced by a conformance test on the import graph); pass/fail derives
  platform-side from the wait status; the verification profile MUST stay network-off (P-T06-2).
- **§25** — the preview module and its port pool are a separate consumer; do not touch.
- **§58** — no terminal without a door; only the *deterministic preamble refusal* converts to a card.
  R9 removes a park, it adds no new terminal.
- **§59** — the verification workspace is the revision, stripped (`.git`-less copy); an honest
  absence is not a failure; **a test that can bill must be unable to start by accident** (no paid leg
  here, and the golden-set re-run is explicitly NOT run — R17).
- **§70** — bootstrap is **computed, never defaulted**; captured commands are versioned DATA and
  nothing executes at capture time; the posture disclosure is identity-keyed with exactly two
  sanctioned exemptions; an advisory verdict releases nothing. All four hold unchanged.
- **§74** — TQ-3's step contracts are decided from the files, never the report. A detected rung must
  not disturb them: it carries no `StepID` (R12).
- **§57** — governance content is a **snapshot of what was ratified, never a live pointer**; mint
  your own Ensure with your own provenance and digest; a governed object's content is what its row
  was committed with, never what is on disk. Binds R17/R18 directly.

---

## 9. Acceptance checklist (concretely checkable)

1. `go build ./...` green; `gofmt -l .` empty; `go vet ./...` green.
2. `go test -p 1 -count=1 -run 'TestTQ4' ./internal/verify/` — all 11 green.
3. `go test -p 1 -count=1 -run 'TestTQ4' ./internal/project/` — all 5 green.
4. `go test ./internal/verify/ ./internal/project/ ./internal/shell/ ./internal/api/ ./internal/memory/ ./internal/worker/` green, **no new skips**.
5. `go test ./...` green (the coordinator's own full battery at landing).
6. `go run ./tools/lockgate` green; `git diff --stat components.lock` **empty**.
7. `grep -n 'Posture' internal/shell/project_seams.go` shows graduation reading `Capture.Commands` only — `Detected` appears in **no** graduation condition.
8. A detected-pack round's `RoundRecord`: `Posture == "bootstrap"`, `ReviewMandatory == true`, `len(VerifiedItems) == 0`, `PostureNote != ""`.
9. Every recorded `detected:` outcome has `ACKey == ""` and `StepID == ""`; `V1Result.ACOutcomes()` is empty on such a round.
10. `detected:build` on the webshop fixture: `State == UNVERIFIABLE-HERE`, `AttributedTo == "command:unrunnable"`, non-empty `Detail`, and the runner was **never called** for it.
11. `detected:lint` is absent from the pack entirely (P1: the tree declares no `lint` script) — and the e2e rung's `AttributedTo == "walk:no-browser-adopted"` with `Detail == WalkUnavailableReason`.
12. `RescanDetected` called twice on the same tree mints exactly one version (`minted` true then false) and appends exactly one `registry.captured` event; `git status` of the fixture tree is clean and its mtimes unchanged.
13. `EffectiveCommands`: for a capture with `Commands.Test = "make check"` and `Detected.Test = "npm test"`, the result's `Test == "make check"`; with `Commands.Build == ""` and `Detected.Build == "npm run build"`, the result's `Build == "npm run build"`.
14. `git diff internal/project/scan.go` is **empty** (R3's byte-unchanged rule) — no project's `scanHash` moves.
15. `git diff internal/verify/v1.go` touches only the `CheckPack` type block; `git diff internal/verify/pipeline.go` touches only lines around `:287` and `:478`. Declared in the commit message.
16. `SeedSoftwareRubric().Version == 4`; the new item's `Probe` is one of the four ratified probes; `Validate` on the bundle passes; the bundle's note states the golden-set re-run is owed.
17. `ComposerPlaybookSeedVersion == "seed-2"`; the new Ensure supersedes rather than re-seeds; `EnsureRW12TaxonomyGovernance` and its digests are **byte-untouched**.
18. `grep -rn 'settings\.' internal/verify/evidence.go internal/project/detected.go` finds no new ⚙ key; the 118/33 tally is unchanged.
19. No `*-api-key.txt` staged; no `Docs/`, `Spec/`, `Research/`, `P3/STATE.md` edit.
20. No world, server, sandbox or browser was started by this packet's tests.

---

## 10. STOP conditions

- A new ⚙ default or clamp becomes necessary → **STOP**, S00.9 amendment + S18 re-sweep.
- The walk needs any sandbox profile change (a new ro-bind, a class change, an egress exception) →
  **STOP**; that is TQ-4b's adoption question, not a TQ-4a implementation detail.
- A detected rung is about to be recorded PASS without having run, or FAIL for a precondition the
  work did not cause → **STOP**; it is UNVERIFIABLE-HERE (R11).
- The golden set would need a live paid re-run to land R17 → **STOP and ask**; it is a paid leg
  under explicit opt-in only (§59).

---

## §TQ-4b — the deferred adoption question (for the gate, not for this executor)

**The question, precisely.** The A16 e2e walk requires a headless browser **inside the verification
sandbox's empty netns**, together with the deliverable's dev server. Nothing in `components.lock`
supplies one, and S16.2/S16.4 make that a gate decision, not an implementation choice: a new
component needs a lock entry with all nine mandatory fields *and* operator ratification (checklist
#10). **TQ-4a therefore ships the schema and the honest UNVERIFIABLE-HERE, and assumes nothing.**

**Host findings (probed this session; nothing installed):**
- `google-chrome` **153.0.8010.47** at `/usr/bin/google-chrome` → `/etc/alternatives/google-chrome` →
  `/opt/google/chrome/google-chrome`. `/etc/alternatives` *is* ro-bound by `Compose`
  (`sandbox.go:484`) but **`/opt` is not** — Chrome is currently **unreachable inside any sandbox
  class**. Host-managed, unpinnable by Sinet (auto-updating Debian package).
- `chrome-headless-shell` at
  `~/.cache/ms-playwright/chromium_headless_shell-1234/chrome-headless-shell-linux64/chrome-headless-shell`,
  plus a full `chromium-1234/chrome-linux64` — present in a user cache, **not installed by Sinet,
  not pinned, not on the rail**; a build-number-only identity (`1234`), no version string on disk.
- `playwright` is **not** installed (`npm ls -g` = claude-code, kimi-code, opencode-ai, promptfoo,
  repomix; `npx --no-install playwright --version` fails). `jsdom 30.0.0` **is** on the rail — but it
  is a DOM simulator with no dev server and no real browser, and cannot walk a served app.
- `node v22.22.1` and `npm` are at `/usr/bin/…`, i.e. **already inside every sandbox class** — which
  is why TQ-4a's evidence rungs need no adoption at all.

**Two candidates, costed.**

1. **Pinned `chrome-headless-shell`, bind-mounted read-only into the netns, driven over CDP by
   platform code with no npm framework.** S16.4 cost: a `library`/`os-mechanism`-shaped entry with an
   exact pin + content hash (the cache build has no version identity, so Sinet must fetch and pin its
   own), BSD-3 path-scoped license check, a funeral plan (Firefox/`geckodriver` is on this host as a
   named fallback), a watch row, and operator ratification. Sandbox cost: a **new read-only bind** of
   the browser tree — a profile change, and S11 declares profiles STRUCTURAL; plus a second process
   inside the netns and `--tmpfs /tmp` sized for a browser profile dir. Code cost: Sinet writes and
   owns the CDP client. **Adopt-unmodified holds trivially** (config/wrapping only).

2. **Playwright as an adopted npm component.** S16.4 cost: an `npm` lock entry plus its browser
   downloads (which are a *second* pinned artifact, with their own hash), Apache-2.0, a watch row,
   ratification. Sandbox cost: the same new bind **plus a Node runtime and `node_modules` tree bound
   into the sandbox**, i.e. a materially larger trust base inside the boundary; and the
   downloads happen at install time, which must not become a per-run network need (S07.3 rule 1
   forbids any egress exception). Screen cost: S16.4 #2 (organ-grade, single-purpose) is arguable —
   Playwright brings a test runner, a config system and a reporter that Sinet does not want.
   Benefit: no CDP client to own, and the DOM-assertion vocabulary already exists.

**Four facts the gate must weigh with the candidates, none of them solvable by picking a browser:**

- **`Compose` ignores caller-supplied binds.** It reads `prof, _ := Profile(c)`
  (`sandbox.go:321`), and `Profile` returns `ROBinds: nil` / `Mounts: nil` for **every** class
  (`sandbox.go:213-224`), so the loops at `:417-423` and `:424-447` are always empty in practice: a
  caller **cannot** inject an extra read-only tree through `Confinement`. The only caller-reachable
  extra binds are `Spawn.EnginePrefix` (`sandbox.go:256`, bound at `:349-351`, mirrored to srt's
  `allowRead` at `srt.go:173-179`) and `Spawn.ROConfig`. So binding a browser is either an
  `EnginePrefix`-shaped reuse or a **structural profile edit** — the gate should say which.
- **Nothing in `internal/sandbox` brings `lo` up.** A grep across `internal/sandbox/*.go` finds no
  `loopback` / `127.0.0.1` reference at all. The design relies on bwrap's own netns loopback on the
  native path and on srt's behavior on the srt path (`NetNone` compiles to
  `deniedDomains: ["*"]`, `srt.go:165-167` — a *domain* deny-all, which is not obviously a loopback
  block and is untested here). **A dev server binding a loopback port inside the empty netns is an
  ASSUMPTION, not a verified capability.** TQ-4b must probe it before costing anything else; if
  loopback is down inside the netns, neither candidate works and the walk needs a different shape.
- A second process inside the sandbox **is** permitted: the seccomp filter is default-ALLOW and
  denies exactly three syscalls — `ptrace`, `process_vm_readv`, `process_vm_writev`
  (`seccomp.go:60-66`) — so `clone`/`fork`/`execve` are open. Caveat: `--unshare-pid` makes the
  first process PID 1 in the namespace and **nothing in the package reaps its children**.

- **A16 names C1; the landed check runner uses C2.** `SandboxCheckRunner` defaults to class `"C2"`
  (`v1.go:363`, §15) because build/test rungs want a writable workspace, while A16's walk text says
  "the C1 verification sandbox". C1 is `WorkspaceMode "ro"` (`sandbox.go:217`) and a Vite dev server
  writes a cache into the tree. Either the walk runs at C2 (network-off, which P-T06-2 requires
  anyway) or C1 gains a writable scratch — **a confinement question the gate must answer explicitly**,
  not a detail to settle in code.
- **There is no live serving code.** `internal/preview`'s dev-server lane composes a plan and then
  reports unavailable (`manager.go:336-377`): the deferred half is the host substrate **plus** the
  spawn/probe/route activation code. The walk needs only the in-netns half (start the server, probe
  the loopback port with `internal/preview/probe.go`'s netns probe, drive the browser in the same
  netns) — it needs no Caddy route and no host routing — but that in-netns half **does not exist
  today** and is TQ-4b work regardless of which browser wins.

**What TQ-4a hands TQ-4b.** `WalkDriver`, `ACWalk`/`WalkStep`/`WalkOutcome`/`WalkState`,
`WalkPlanFor`, `WalkUnavailable`/`WalkUnavailableReason`, and the detected `Dev` command slot.
TQ-4b implements `Walk` and flips the e2e rung's reason from "no browser is adopted" to a real
outcome; **no TQ-4a type changes shape when it does.**

---

## §Adjacent, deliberately NOT in TQ-4a

`P3/STATE.md:248` records P3-SIT-1 deferring *"feeding the tree diff to the V2 judge"* to P3-TQ-4.
That is a judge-input-slice change (S07.5) with no relation to A16's execution rung, and the
coordinator's TQ-4a/TQ-4b split does not carry it. **Flagged for the coordinator, not implemented
here** — absorbing it silently would widen the packet past its spec refs.
