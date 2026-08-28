# P3-GF13 — the per-string keep ledger (drain r1)

Companion note to `P3/briefs/P3-GF13.md`, demanded by its §3/§6.6: the triage
must prove itself, and family names were why survivors went unexplained at
evaluation. Every non-test string literal under `internal/` that still matches
the citation classes is listed here, per string, with the reason it stays.

**Method.** AST scan over every non-test `*.go` file under `internal/`, string
literals only (comments never enter, so a citation in a comment — house style,
expressly kept — cannot appear). Classes:
`\(S[0-9]+\.` · `\(S[0-9]+;` · `\(Spec S` · `\(D[0-9]+` · `P47-` ·
`\([0-9]+\.[0-9]+\)` · `\bD10\b` · `\bB[0-9]-[0-9]\b` · `\bA1[0-9]\b`.

**Census (trued at drain r2 from a fresh sweep).** 728 hits at `61d7df7` →
**547**. Of those, **86** are class-(c) `%w:` internal sentinels (the flagged
deferred conduit) and the remaining **461** fall in the families below. **Zero
unexplained requester-surface survivors.**

Round history: 728 → 553 (purge + drain r1) → 547 (drain r2 closed six door
strings and the onboarding activity line).

---

## A. The flagged deferred conduit — NOT fixed here, owed as its own packet

**`writeSurface` serves `err.Error()` for any unmapped error**
(`internal/api/intake_handlers.go`), so every `fmt.Errorf("%w: …")` sentinel in
the tree is one unmapped error away from the wire. 86 literals. This is GF11's
D-2 §38 class, structural, and a packet of its own: **sentinel-not-text coverage
is owed.** The brief scopes it OUT; only the sentinels R1–R9 already touched
were rewritten, plus `verify.ErrNoCheckPack`, which is not fallback-served at
all — `escalate.go` composes it deliberately into the infrastructure card's
summary ("verification cannot run — " + cause), which the walk read verbatim.

Sentinels a walker plausibly meets and that are therefore the deferred packet's
first targets, named so the next executor does not have to re-find them:
`intake.ErrGateOpen`, `intake.ErrNotRequester`, `intake.ErrMarkersOpen`,
`intake.ErrCitationUnresolved`, `verify.ErrNotRequester`, `project.ErrNotOwner`,
`worker.ErrDegraded`.

## B. Frozen — byte-untouched whatever the sweep says (brief §2)

| String | Site | Reason |
|---|---|---|
| `human cancel (4.5)` | `run/canceldetail.go:31` | RW-19 frozen wire literal (HANDOFF §5) |
| `human cancel (4.5)` | `api/reads.go:1138` | the reader half of the same contract |
| `'requester cancelled'`, `'requester: rethink'`, `'human cancel (4.5)'`, `'no reason given'` | `history/catalog.go:258-271` | SQL literals of the `status.tasks_cancelled` two-leg read |
| cancel MINT PREFIXES | `stage/answer.go`, `stage/ladderanswer.go`, `intake/answer.go` | held by the divergence pin (`cancelrow_rw19_test.go`) |
| `intake cancelled (4.5): finalize-with-card` | `intake/answer.go:141` | cancel-family prose sibling, frozen-adjacent (§2 item 6) |
| `cancel is always available (4.5)` | `intake/answer.go:619`, `:983` | same |
| `intake cancelled (4.5)` | `intake/answer.go:1004` | same |
| `cancel is always available (4.5); parked→finalized…` | `stage/answer.go:119` | same |
| `verification cancelled at the card (4.5)…` | `stage/answer.go:137` | same |
| `(4.5)` + cancel prose | `stage/cancel.go:283`, `:408`, `stage/ladderanswer.go:172`, `:196` | same |
| `[P47-x]`, `(kind)` sub-lines | `intake/artifact.go:604/:624/:715` | the D9 artifact of record has a BYTE-STABLE re-render duty (S06.6) |
| `reason_too_long` | `api/actions.go:86` | refusal CODEs are machine values |

## C. Ledger BASIS columns and record LABELS — audit provenance on a record surface

The brief's §3 permits keeping a citation-bearing BASIS while the DECISION
sentence goes plain. Every decision sentence in this family WAS rewritten; what
remains is the `why` argument of `RecordDecision` (the platform's own audit
note about which rule produced the entry) and the stage-record LABELS the
ledger indexes by. They are the record's provenance, not a sentence addressed
to anybody.

`intake/answer.go` — `:266` interview card · `:367` family question · `:394`,
`:407` coverage decision card · `:435` research decision card · `:453`
SPEC-DOUBT · `:502` refused-emission card · `:542` approval-card routing block ·
`:566` compose-when-earned · `:604`, `:611` approval card · `:793` approval
record · `:849` claim collision · `:1046` explicit requester action.

`intake/pipeline.go` — `:287` intake run · `:304`, `:430` Stage-0 intake
record · `:436` Stage-0 triage classification · `:519` question-set fallback ·
`:1226` Stage-2 spine · `:1359`, `:1364` Stage-3 critique · `:1413` TIER-UP ·
`:1605`, `:1608` SPEC/PLAN artifact labels · `:1675` answered-marker proof ·
`:1804` spent-re-emission basis.

`intake/delta.go:301` delta-only card · `stage/answer.go:81`, `:164`, `:215`
(retry / accept-best-effort / guidance bases) · `stage/compose.go:194`, `:296`
composition ceremony ·
`stage/skeleton.go:470/:524/:645/:748/:850/:1136` stage-work labels ·
`stage/split.go:129` · `stage/ladderanswer.go:91` retry basis ·
`stage/engines.go:598` stage-input label · `verify/v2.go:146` verification
input slice · `memory/source.go:262` selection provenance.

## D. Operator / telemetry surfaces — §§30/40/44's content-vs-telemetry line

Precision is the point on these, and none is a requester's task page.

- **`scheduler/limits.go`** (14) — `Classify`'s rate-limit verdicts. Consumed by
  `watchlist/canary_auth.go` as an operator lane-health record; naming which
  S10.5 class fired is what makes it actionable. **Deviation from the brief's
  REWRITE bullet, declared.**
- **`worker/lint.go` (8) + `worker/skill.go` (1)** — template-authoring
  diagnostics: the audience is whoever writes a worker template, a D10 operator
  act, and the finding names the rule so it can be fixed. **Deviation, declared.**
- **`preview/manager.go:338`, `:392`, `:451`, `:683`** — host-fault
  diagnostics: sandbox boundary, Caddy admin endpoint, non-loopback bind refusal.
  The audience is the person configuring the host. Requester-facing preview
  reasons (`:224`, `:227`, `:229`, `:272`, `:365`, `:406`, `:534`, `:554`) were
  all rewritten. **F9 decided and documented.**
- **`verify/escalate.go:73-87`** — `RouteTable.RaisedBy`. Grep-proven to have no
  reader outside the table: it documents the ratified S07.7 raisers. Never served.
- **`verify/escalate.go:418`** — the dead-man canary deliverable body, an ops
  artifact that exists to prove the escalation path is alive.
- **`api/settings.go` (10), `settings/index.go` (13), `settings/schema.go`** —
  the operator settings surface quoting its own registry; the standing
  settings-tab directive owns that surface.
- **`api/oversight.go` (4)** — operator-scope cards (suppress / dismiss /
  acknowledge). The three requester-facing `answerable` reasons in
  `api/approvals.go` (`:319`, `:349`, `:441`) were rewritten; the platform-scope
  operator ones (`:330`, `:338`, `:369`) stay, same reason.
- **`api/chatapi.go:409`, `api/historyapi.go:101`** — operator-only route notes.
- **`conformance/registry.go` (23)** — operator gate cards whose precision is the
  point (brief §3 names this keepable).
- **Log lines, never served:** `stage/onboard.go:369/:416/:552`,
  `stage/resume.go:103`, `stage/compose.go:180/:300`, `stage/cancel.go:387`,
  `stage/local.go:200`, `stage/router.go:143`, `stage/runner.go:567`,
  `stage/skeleton.go:929`, `stage/ladderanswer.go:73/:151`,
  `scheduler/scheduler.go:514`, `recovery/recovery.go:259`.

## E. Machine values and registry metadata — prose changes only, never values

`stage/resume.go:133` (`cause` detail-key value) · `intake/triggers.go` seed
Source + rule ids · `eventlog/contract.go` (32) event-type registry ·
`units/units.go` (19) · `local/manifest.go` (12) · `lockfile/*` · `adapters/*`
lane and substrate metadata · `memory/*taxonomy*` + `memory/seeds.go` +
`verify/seeds.go` + `watchlist/seed.go` + `intake/taxonomy.go` (seed-file
provenance strings, which the brief names keepable) · `history/catalog.go`,
`history/layer0.go` (SQL and column contracts) · `retention/*`.

## F. Engine PROMPTS — model-facing, not requester-facing

`stage/engines.go:383/:439/:454/:682/:746/:757/:808`, `stage/compose.go:66/:120`,
`worker/playbook.go`. These instruct a model and cite the spec on purpose. The
one change made here was ADDITIVE: `pairSchema` gained the plain-words rule
forbidding the model from quoting internal ids back at a requester (R1).

---

## G. Corrections made at drain r2

**Filed wrong, now REWRITTEN.** `stage/onboard.go:428` was listed under family
C as a ledger basis. It is not: it is a `TransitionOptions.Reason`, which is
served as the activity line on the owner's onboarding run card — the exact
class drain r1's F3 purged. It now reads like its F3 siblings. The ledger
entry is withdrawn, and the lesson is that "which STRUCT FIELD carries it"
decides the audience, never which package it sits in.

**Contract-class survivors on requester doors, now REWRITTEN.** Six strings the
class scan caught but no family explained, all served to a person standing at a
door: `api/actions.go:244` and `:262` (the follow-up door's bad-version and
no-such-version refusals), `api/objects.go:103` (the 404 on a requester's own
object read), `api/previewapi.go:187` (the preview-stop answer, which now names
all four things the stop actually did).

**Genuine ops keeps on the same handler, LEDGERED here per string.**

| String | Site | Reason |
|---|---|---|
| `this revision pins an object whose bytes are not in the object dir (S13.1 retention)` | `api/objects.go:124` | 500-class platform-integrity answer: the pinned bytes are GONE. The caller cannot ask this differently, it is paired with an `Error` log naming the deliverable and sha, and the retention rule it cites is what an operator needs to chase it. Not a door a requester can act on. |
| `this revision pins an object whose stored bytes hash differently (S13.2 content addressing)` | `api/objects.go:132` | Same class, same posture: the bytes are present and are NOT the bytes the URL names. One vocabulary for one failure, deliberately shared with `reviewErr`'s content-drift answer. |

---

## Deferred-ledger items raised by this packet

1. **The `writeSurface` `err.Error()` fallback** (§A) — sentinel-not-text
   coverage owed; 86 literals one unmapped error from the wire.
2. **`chooseFlatLane`'s raw `%v`** (`worker/routing.go`) — a Go error rendered
   into requester prose. Pre-existing; coordinator ruled do-not-fix here.
3. **Bare-token citations OUTSIDE the declared classes** (evaluator's nit,
   drain r2). The sweep's regex requires a paren or a known prefix, so a spec
   id written as bare prose slips it: `api/actions.go:253` ("the S13.9 framings
   are the landed set", a served `badRequest` on the follow-up door) and the
   "S10.4 pause switch" not-wired family. Coordinator's disposition: these ride
   this deferred conduit item rather than being fixed piecemeal, because
   closing the CLASS needs a widened scan, not four hand edits. **Note for
   whoever takes it:** `actions.go:253` sits one line from two refusals drain
   r2 rewrote, so that door is knowingly half-purged until then — it is the
   cheapest first target and the most visible.
