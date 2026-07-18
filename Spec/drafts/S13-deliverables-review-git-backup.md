## S13 — Deliverables, review, git & backup

**Scope:** Everything a task produces on its way out of the platform: the deliverable/revision/comment schema, per-type review behavior, the findings→retry drain, git topology and the broker-mediated accept flow, the project/repository registry, disposable previews, follow-up lineage, and the 11.3 encrypted-snapshot + restore-drill pipeline.
**Binding inputs:** R13 §4 as ratified [G2 D2.1(e)] · `Spec/frontend-components-v1.md` §2 (the carried N15 behavior spec — binding; cited below as [FC-v1 §2]) · G2 D2.5 (backup posture), D2.6 (git identity), Def.13 (PDF), Def.14 (binaries) · G1 rider 1 · feature list D2, D7, D9, 5.4–5.5, 6.1–6.4, S1.2, S1.6, S1.8–S1.9, S1.11, S3.5, 2.6, 11.3 · siblings S01 (units, portpool, Caddy, timers, CI) and S02 (durable set, claims, effect journal, workspace seam) · P-T12-1..4.

Terms coined here: **deliverable** — the long-lived reviewable entity a task produces (one per task output). **revision** — one immutable numbered snapshot of a deliverable (1..N). **orphan** — the explicit anchor state of a comment that no longer attaches anywhere in the current revision. **drain point** — the single code path that hands review feedback to a retry. **snapshot commit** — a platform-owned tree-level commit on a run branch (the safety net and revision raw material). **merge card** — the reviewable decision card produced when an accept does not apply cleanly. **snapshot ledger** — the SHA-256 integrity record of every 11.3 snapshot. **escrow identity** — the off-host recovery copy of the operator's age identity (paper + passphrase-encrypted file).

### S13.1 Deliverable & revision schema

The Gerrit/Phorge shape [R13 §4.1; G2 D2.1]: a **deliverable** is a long-lived entity; its content history is **revisions 1..N**, immutable once minted, every revision hop persisted. Lineage is NEVER compressed (GitHub's force-push timeline compression is the named anti-pattern) [R13 §2.2/§4.1].

Table families (rows in `platform.db`, owner-attributed per 15.6, under all S02.1 disciplines; indicative names — exact DDL at the P3 schema workshop jointly with S02.2):

| Table family | Holds |
|---|---|
| `deliverables` | task ref, project ref, type, current revision, state (in-review / accepted / superseded), owner |
| `deliverable_revisions` | deliverable ref, number 1..N, minting run/attempt ref, content pin (S13.1 below), verification-verdict ref [XREF:S07], created-at |
| `review_comments` | one schema for human comments and verification findings (S13.3): anchor record, anchor status, body, lifecycle, consumption stamps; findings add `{severity, description, suggested_change?, finding_number}` [R13 §4.1/§4.4] |
| `repo_registry` | the S1.6 registry (S13.7) |
| `snapshot_ledger` | the 11.3 integrity ledger (S13.10) |

These families join the S02.9 durable set by reference — they are part of the 11.3 snapshot payload.

- **Everything arrives as a reviewable change (6.1).** Revision 1 is presented against the pre-task state (project HEAD at branch base); revision N against N−1 by default (the default rework view [FC-v1 §2]); any revision pair is diffable on demand — revision-over-revision navigation is a schema capability, not a UI nicety [R13 §4.1].
- **Content pin.** A revision pins its content immutably: repo-backed types pin a snapshot-commit sha (S13.5); binary types pin content-addressed object-dir hashes (S13.2). Minted revision refs are retained in the platform-owned project store under a platform ref namespace (`refs/sinet/deliverable/<id>/rev-<n>` [coordinator-draft]); workspace GC MUST NOT delete the only copy of a minted revision's objects [XREF:S02].
- **Minting.** A revision is minted when a candidate passes to review — one per round; the verification handoff that triggers minting is [XREF:S07].
- **Automation definitions ride the same machinery.** A newly composed worker/automation definition presents at birth as a deliverable — readable diff, commentable, previewable (2.6); the composer's approval-as-diff station consumes this schema [XREF:S08].

### S13.2 Review surfaces per type (6.1, S3.5 — behavior normative here, rendering [XREF:S15])

Every deliverable type has a defined comparison behavior at v0; no type is refused, and a type without a rich surface gets the honest fallback, visibly labeled (the 2.1 honesty posture).

| Type | v0 comparison behavior | Source |
|---|---|---|
| Code / text (incl. markdown source) | Side-by-side and unified line diff with intraline highlighting; the unified diff is computed host-side (`git diff` between revision pins) and rendered by the S15 diff widget | [R13 §4.3; FC-v1] |
| PDF | **Extracted-text diff only at v0** (MIT-licensed extraction; AGPL extractors excluded as defaults); pixel-overlay lane is post-v0 | [G2 Def.13; R13 §4.3] |
| Images | Visual compare, GitHub's trio: 2-up / swipe / onion-skin over the two revisions; optional server-side pixel-diff as an aid | [R13 §2.1/§4.3] |
| Binaries / other opaque | Content-addressed **local object dir + hash refs from committed text**; the review surface shows a metadata card (name, size, hash, type) per side and a changed/unchanged verdict by hash; download to inspect | [G2 Def.14; R13 §4.6] |
| Any other text-extractable | Plain extracted-text diff, labeled as the fallback surface | [R13 §4.3] |

- Binary bytes are host-local review material outside the 11.3 payload; the committed hash refs make loss detectable and regeneration targeted. A binary that must be reviewable on GitHub itself is promoted to LFS — the only sanctioned exception [G2 Def.14].
- Semantic docx redlining (Python-Redlines, OOXML-embedded comments) and pandiff prose diffs are the parked enhancement lane, gated on the R13-OQ7 license/maintenance audit [XREF:S16] (see Deferred).
- Cross-format compare has no OSS equivalent (buy-not-build territory); Sinet stays per-format by design [R13 §2.1].

### S13.3 Comment & anchor model — the carried N15 behavior contract

This subsection is the normative behavior contract for anchored review; the widget that renders it is S15's binding pick [FC-v1; XREF:S15]. Human review comments (6.2) and verification findings [XREF:S07] share this one schema [R13 §4.1].

- **Anchor record** (binding shape): `{file_path, side ∈ {old|new}, line_no, line_text}`, bound to (deliverable, revision) [FC-v1 §2]. `line_text` is the quote selector, `(side, line_no)` the position selector — the W3C redundancy principle ("selectors hint each other") in compact, closed-world form [R13 §2.2].
- **Server-side re-validation.** Anchors are re-validated server-side on every render and every port to a new revision; client- or agent-supplied positions are never trusted as placement authority [FC-v1 §2].
- **Re-anchoring ladder** — porting a comment onto revision N+1 tries, in order [R13 §4.2; FC-v1 §2]:
  1. Map `(side, line_no)` through the known N→N+1 diff (Sinet's closed world always has the diff — strictly easier than the open-web problem).
  2. Exact `line_text` match at the mapped position.
  3. `line_text` search within `⚙ review.anchor_drift_lines` (default ±2) of the mapped position.
  4. Degrade to a **file-level comment** (Gerrit's next-best-location contract, line→file), keeping the original quote visible.
  5. **ORPHAN** — an explicit schema state that keeps the quote and the original-revision link, stays visible on the review surface, and still drains (as file-level). Never silently dropped. Hypothesis's ~22% open-web orphan rate is the proof this state is mandatory even closed-world (P-T12-2) [R13 §2.2/§4.2].
- **Anchor status is recorded** per comment per revision (`exact / mapped / drifted / file / orphan`). Silent mis-anchoring is worse than orphaning; anchoring quality gets an 11.2 spot-check (P-T12-2) [XREF:S14].
- **Lifecycle:** `open → consumed`. Consumption is batch-stamped — one `consumed_at` plus the consuming attempt ref for the whole drained batch — so "what did that rework receive" is auditable after the fact [FC-v1 §2]. New round → new comments; a consumed finding re-review deems unfixed produces a fresh finding [XREF:S07].
- **Synthetic render surface.** Any comment without a live anchor in the current view (file-level, orphaned, cross-cutting) renders in a dedicated, always-visible strip of the review surface. Invariant: **no comment may exist without a render location** — the class of the real 44-invisible-comments failure this rule was born from [FC-v1 §2].
- **Escape-first rendering.** All deliverable and comment content renders escaped/text-first on every surface; exactly **one** sanctioned raw-HTML channel exists — the sandboxed rendered-document view [XREF:S15] — and nothing else may render raw markup [FC-v1 §2]. The contract is normative here; enforcement mechanics are S15's.
- For docx deliverables (post-audit lane), comments are additionally embedded natively in OOXML so users' own tools see them [R13 §4.2] (parked with the docx lane).

### S13.4 The findings→retry drain point (5.5)

Exactly **one** code path hands review feedback to a retry [FC-v1 §2]. At rework dispatch it:

1. Collects every `open` comment/finding on the deliverable's current revision.
2. Numbers them `[F1..Fn]` (stable within the attempt), each carrying anchor (or its file/orphan degradation) + severity + description + optional suggested change — the finding schema of S13.1 [R13 §4.4].
3. Marks the batch `consumed` with the shared `consumed_at` + attempt ref (S13.3).
4. Delivers the numbered points into the next attempt's brief [XREF:S05].

- Orphaned and file-degraded findings are **still delivered** — delivery is never conditional on anchoring success (P-T12-2).
- Severity distinguishes blockers from notes: only genuine blockers trigger another round; polish travels as notes (5.4). Rework bounds, re-review against the frozen ACs, and the verification ladder that generates machine findings are [XREF:S07].
- **Pre-registered benchmark question (R13-OQ6):** whether anchored `[F#]` findings measurably beat file-level notes for retry quality is empirically open field-wide; it is registered with the 11.2 practice [XREF:S14]. If the answer is no, the ladder simplifies to file-level numbered findings [R13 §4.10] — the schema above degrades to that shape without migration.

### S13.5 Git topology: branch-per-pipeline + platform snapshot commits

- **Branch-per-pipeline.** Each pipeline gets one long-lived run branch in its workspace (substrate: worktree + overlayfs per S02.10 [XREF:S02]; sandbox composition [XREF:S11]), accumulating commits across stages and review rounds. During a review round-trip: same branch, new commits — the platform NEVER amends or force-pushes a branch under review (zero field support for mid-review rewrites) [R13 §4.6/§2.5].
- **Fresh branch per attempt** when the base moved or an attempt is abandoned (the attempt model); the old attempt's branch stays until revision-retention GC clears it [R13 §4.6].
- **Snapshot commits** are platform-owned, tree-level (they capture bash side effects, not just tool edits), junk-excluded via platform ignore rules, written at checkpoint boundaries — where they serve as the Claude-lane checkpoint artifact ref [XREF:S02 §S02.4d] — and at stage/round boundaries, where they are revision raw material (S13.1). They are squashed away at accept (S13.6); they never reach a user-facing remote.
- **The net is the commits — never:** engine checkpoints (bash-blind, session-scoped, 30-day expiring), `git stash` (untracked-file skips, documented data loss), or jj (unsupported worktrees/hooks/submodules/LFS; zero mainstream adoption; parked at S02) [R13 §2.6/§4.6].
- **Platform-owned utility checkouts** (accept staging, preview-at-rev) are plain worktrees with the shipped lifecycle rules: `worktree lock` while in use, `worktree prune` sweeps, never auto-delete dirty [R13 §4.6; XREF:S02 GC].
- A Gemini-style shadow repo for mid-step capture is deliberately not built; it is a cheap bolt-on if mid-step capture ever proves necessary (Deferred) [R13 §4.6].

### S13.6 Accept flow (6.3, D9) — one action, broker-mediated

Accepting is one action on the deliverable's approval card (risk tier High — outward push [S3.2]). The push executes through the effect journal as a **class-A effect** — `git push --force-with-lease=<ref>:<expect>` is S02.7's own natively-idempotent exemplar; the CAS expect-sha and the candidate revision pin are the payload hash pinned at approval [XREF:S02]. The flow, host-side [R13 §4.5]:

1. **Applies-cleanly gate.** The candidate revision is applied to current project HEAD on a throwaway ref (a depth-1 merge queue; a second in-flight accept validates against HEAD-plus-first). A collision surfaces as a reviewable **merge card** — options: agent-auto-resolve (a bounded rework [XREF:S07]) / resolve manually / abort to a new attempt — never a silent overwrite (S1.11 verbatim; consistent with S02.8, which owns claims and the sibling-accept freshness trigger) [XREF:S02].
2. **One clean attributed commit:** author = committer = accepting user, set per-invocation (env/`-c` — no global host state); committer email is the ID-based noreply form (survives renames) or a connected address from the per-user store, so contribution graphs attribute; Conventional-Commit subject; body carries the task/session link as provenance. The run branch's snapshot commits are squashed by the **platform** — never by GitHub UI (the squash-authorship hazard is thereby structurally avoided) [R13 §2.4/§4.5].
3. **Attribution trailers**, deterministic and displayed on the card **before** accept (the VS Code mis-attribution lesson): `Co-Authored-By: <engine> <model> <vendor-noreply>` (GitHub renders it) plus `Assisted-by: <engine> (<model>) via Sinet` as the machine-parseable provenance line. Trailer text is fixed platform config in the settings registry (string entries, audit-trailed), never per-run improvisation [R13 §4.5].
4. **Push as the user** over broker-held SSH keys with explicit CAS: `--force-with-lease=<ref>:<sha-at-approval>`; multi-ref updates use `--atomic`; pacing respects GitHub's documented ≤6 pushes/min/repo and 2 GB push cap (household volume is orders below both). A lease failure is a normal collision → back through step 1 to a merge card; NEVER a blind retry against a new sha [R13 §2.4/§4.5].
5. **Signing [G2 D2.6]:** the operator SSH-signs from day one; members opt in at enrollment. All-or-nothing per user — a user whose commits are not all signed MUST NOT enable vigilant mode. The broker performs signing (`gpg.format=ssh`); keys never leave it [XREF:S11]. gitsign/Sigstore is NEVER used (renders Unverified on GitHub — worse than no signature). No GitHub Pro at v0; revisited when a second member joins [G2 D2.6].
6. **Enrollment (one-time per member):** keypair generated inside the person's store on the host (never leaves it; store hygiene [XREF:S11]); the user uploads the pubkey to their GitHub account (auth + optionally signing); the ID-based noreply address is captured into the store; repo owners invite collaborators (only owners can — onboarding steps route to the owning user). Owner-exit path: repository transfer (1-day acceptance; stored remotes are rewritten — redirects are never trusted) [R13 §4.5].

**P-T12-1 — THE BROKER IS THE GUARDRAIL.** Fine-grained PATs cannot push to repos where the holder is an invited collaborator (documented resource-owner gap) — SSH is forced, not preferred. And on Free private repos GitHub enforces **zero** branch protection — any collaborator credential could force-push main. Therefore, as a security property of the platform:

- Member git credentials exist ONLY inside the broker (D2); they never enter sandboxes and are never exported [XREF:S11].
- The broker refuses any push to a protected ref that does not originate from an approved accept effect; protected-ref policy is registry data (S13.7), default = the project's default branch, "main only via accepts."
- Every broker push is CAS (`--force-with-lease=<ref>:<expect>`; the bare form is forbidden — trivially defeated by background ref updates).
- **This property is tested:** a conformance-suite entry attempts a non-accept push to a protected ref and MUST observe a broker-side refusal [R13 P-T12-1].

ToS posture: one GitHub account per human, each person's runs pushed with that person's own credentials for that person's work — several humans' accounts operated from one host violates nothing found; the tripwire is pooling one person's token for others' work, which D2 already forbids [R13 §2.4].

### S13.7 Project & repository registry (S1.6)

The registry of known projects and repositories lives in `repo_registry` (S13.1): per entry — name/alias, local project-store path, remote URL, owning user, invited members, default branch, **protected refs** (broker policy input, S13.6), captured **conventions**, **commands** (build/test/lint/run/preview), and **danger zones**. Rows are owner-attributed; captured content is versioned.

- **Onboarding a repository is itself a task** the platform performs: register → clone into the project store → scan → draft conventions, commands, and danger zones → the owner approves the draft (D10 — their own object) → entry goes active. Re-scan on demand or when drift is detected.
- **The registry feeds:** intake resolution ("in the shop backend" needs no path-explaining) [XREF:S06]; conventions/danger-zones injection into stage briefs and the Ledger's pinned constraints [XREF:S05]; workspace creation [XREF:S02]; broker protected-ref policy (S13.6); preview commands (S13.8). Project-scope knowledge beyond repo facts is S09's [XREF:S09].
- **Documents ride the identical machinery:** D9's "document store" is the same per-project git repo — no surveyed platform maintains a separate versioning model for documents, and neither does Sinet [R13 §4.6]. A v0 project without a repo gets one created/registered by the onboarding task.

### S13.8 Previews (6.4, S1.9)

One click launches a built application from a deliverable revision in a disposable environment; nothing touches the live copy or the reviewed workspace.

- **Runner:** heuristic host-toolchain detection — `package.json` → pnpm/npm, `pyproject.toml` → uv, `mise.toml` → mise, `index.html` → static server (providers cribbed from Railpack) — executed **inside the settled sandbox stack** [XREF:S11]. No container daemon per preview; containers are not the substrate [R13 §4.8].
- **Zero mutation:** installs and preview writes land in a discardable overlay upper (or a throwaway clone when dependency-dir-sized) over the read-only revision checkout; teardown deletes the upper. The preview CANNOT mutate the reviewed workspace — sandbox composition is [XREF:S11] [R13 §2.8/§4.8].
- **Ports & routing:** dev server binds 0.0.0.0 inside the run-sandbox netns; the port comes from `sinet-portpool` [XREF:S01]; the platform **probes the netns for listening ports** rather than trusting config (multi-port → a picker). Routing is a Caddy admin-API route per live preview — subdomain/port-based, NEVER path-prefix (breaks arbitrary apps); WebSocket upgrade headers forwarded and `allowedHosts` injected, or Vite HMR silently dies. Household access rides the tailnet front chain [XREF:S01] [R13 §4.8].
- **Lifecycle:** `systemd-socket-proxyd --exit-idle-time` + `StopWhenUnneeded=` give on-demand start and idle-stop for free from the substrate — `⚙ preview.idle_stop` (default 15 min [coordinator-draft]); teardown releases the port and removes the Caddy route. Concurrency is capped at `⚙ preview.max_concurrent` (default 3 [coordinator-draft]) — preview dev servers compete with interactive use of the host (3.11; arbitration [XREF:S10]).
- **Before-vs-after (S1.9):** two instances — the accepted revision via an immutable worktree-at-rev checkout (S13.5 utility checkouts) and the candidate — in a dual-iframe view with synced navigation [XREF:S15]. The port-pool daemon [XREF:S01] and this comparison UI [XREF:S15] are the only novel code in the whole preview feature [R13 §4.8].
- **Non-web:** ttyd serves CLIs over the same routing; notebooks self-preview; static output gets the static server. Anything else (desktop/embedded) shows an explicit, honest **"no preview available for this type"** state — never a broken iframe [R13 §2.8/§4.8].
- **Sidecar-needing projects** (Postgres/Redis-class) surface as a labeled "requires container tier" state at v0; the rootless-podman tier is the named escape hatch, never the default (Deferred) [R13 §4.8].
- Previews are read-only with respect to the world: launching one is a Low-tier, reversible action (4.2 boundary untouched — no proposal needed); exposure exists only through the tailnet front chain [XREF:S01].

### S13.9 Follow-ups from deliverables (S1.2)

Any finished deliverable can spawn a successor task in one action. The successor row carries a `successor_of` link to (deliverable, revision); lineage is visible in both directions on the task detail and board cards [XREF:S15]. The successor enters normal intake with the project's inherited context plus the predecessor deliverable ref as brief material [XREF:S06; XREF:S05]. Revision / extension / counterpart framings ("now the English version") are intake presets over the same link, not schema variants. Concurrent follow-ups in one project are coordinated by the S02.8 claims machinery, and an accepted sibling fires the freshness trigger [XREF:S02].

### S13.10 Platform-state snapshots & the restore drill (11.3)

**Pipeline, per scheduled snapshot** [G2 D2.5; R13 §4.9]: the S02.9 durable set — `VACUUM INTO` a temp file → text-first `.dump` (raw trace payload bodies excluded; full traces stay local under 11.1 retention [XREF:S02]) — plus the platform's git-versioned file stores (worker templates, knowledge/memory files [XREF:S08; XREF:S09]) as text-first exports → `tar | zstd | age -r <operator recipients>` → **one encrypted blob per snapshot**, committed to the operator-owned private snapshot repo (D9), keep-N on one branch, pushed CAS by the broker. Cadence `⚙ backup.interval` = daily; retention `⚙ backup.keep` = 30 [G2 D2.5]. The daily timer rides S01.7's persistent calendar timers (a slot missed in suspend fires once on next activation) [XREF:S01].

- **Client-side encryption is unconditional:** a private repo is access-controlled, not end-to-end encrypted, and this data includes every member's memories and receipts (11.3). Nothing leaves the host unencrypted.
- **Secrets are vaulted separately and are NEVER in the snapshot payload** [11.3; G2 D2.5]: per-person credential stores and broker secrets ride the broker's own encrypted-at-rest mechanics; their backup/escrow is a broker duty [XREF:S11], deliberately decoupled from this pipeline.
- **Blob discipline:** stay under GitHub's enforced 100 MB file limit — chunk the archive at ~90 MB if ever needed; pushes sit far below the 2 GB cap. If snapshots outgrow repo comfort, the named escape hatch is release assets (<2 GiB/file, unlimited total) — also the closer-to-true-delete lane [R13 §4.9].
- **History bounding is client-side make-believe at the remote (P-T12-3):** GitHub retains rewritten-away blobs indefinitely (true deletion is a Support-run GC). Acceptable ONLY because every blob is age-encrypted — with the consequence that key compromise is retroactive over everything ever pushed. Mitigations: escrow hygiene (below), **annual snapshot-repo rotation** `⚙ backup.repo_rotation` = 12 months [G2 D2.5] (a fresh repo bounds what any one old key's remnants cover; the old repo is archived), and a Support-ticket purge as the nuclear option.
- **Recovery material (escrow identity):** age's spec forbids mixing passphrase and key recipients in one archive, so recovery is **a paper copy of the operator's age identity + a passphrase-encrypted copy of the identity file stored off-host**; snapshots encrypt to key recipients only. TBD-OPERATOR(age identity escrow created — paper + passphrase copy off-host — before the first snapshot push). An X-Wing post-quantum hybrid recipient is an optional add once the Go CLI plugin status verifies (watchlist [XREF:S14]) [R13 §4.9].
- **Integrity — the ledger is load-bearing (P-T12-4):** age has NO sender authentication; encryption proves who can read, never who wrote. Every snapshot's SHA-256 + size + created-at is recorded in `snapshot_ledger` (in `platform.db`) AND inside each subsequent archive. A compromised remote swapping blobs is defeated by the ledger check, nothing else.
- **Restore drill — scheduled and TESTED** (`⚙ backup.drill_interval`, default 3 months [coordinator-draft]): fetch the newest blob from the remote → verify against the ledger (**fail-closed on mismatch** — the drill aborts and raises a High flag [XREF:S14]) → decrypt with the **escrow** identity (proving the recovery material, not just the live key) → unpack → rebuild the DB from the dump → `integrity_check` + the S02.9 invariant assertions [XREF:S02]. Backup that is not restore-tested does not exist; the drill result is an event-log record either way.
- Litestream continuous replication remains the deferred *additive* lane (implementation phase, pending its silent-failure bug triage) [G2 D2.5; XREF:S01]; the dump-snapshot lane above is the load-bearing 11.3 mechanism.

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1)

| Name | Default | Clamp/range | Ratified by |
|---|---|---|---|
| `review.anchor_drift_lines` | 2 | 0 – 10 [coordinator-draft clamp] | FC-v1 §2 (carried N15 behavior) |
| `backup.interval` | 24 h (daily) | 6 h – 7 d [coordinator-draft clamp] | G2 D2.5 |
| `backup.keep` | 30 | 7 – 365 [coordinator-draft clamp] | G2 D2.5 |
| `backup.repo_rotation` | 12 mo | 6 – 24 mo [coordinator-draft clamp] | G2 D2.5 (annual) |
| `backup.drill_interval` | 3 mo [coordinator-draft] | 1 – 12 mo | R13 §4.9 (⚙ flagged unnumbered) |
| `preview.idle_stop` | 15 min [coordinator-draft] | 1 min – 24 h | R13 §4.8 (⚙ flagged unnumbered) |
| `preview.max_concurrent` | 3 [coordinator-draft] | 1 – 10 | 3.11 posture [coordinator-draft] |

**Known problems owned here:**

- **P-T12-1** — the broker is the only ref protection on Free private repos → stated as a security property (S13.6): broker-only keys, protected-ref policy from registry data, CAS-only pushes, and a mandatory broker-side refusal test in the conformance suite. Pro re-entry when a second member joins [G2 D2.6]; PAT-gap closure on the watchlist reopens transport choice [XREF:S14].
- **P-T12-2** — comment orphaning is guaranteed at some rate (~22% open-web baseline) → explicit ORPHAN state, synthetic render surface (no comment without a render location), orphaned-findings-still-delivered, all with tests (S13.3–S13.4); silent mis-anchoring gets an 11.2 spot-check [XREF:S14].
- **P-T12-3** — encrypted remnants make key compromise retroactive → escrow-identity hygiene, annual snapshot-repo rotation, Support-purge as last resort (S13.10).
- **P-T12-4** — snapshot substitution is undetectable by encryption alone → the SHA-256 snapshot ledger (DB + in-archive) and the fail-closed ledger verify in the restore drill are load-bearing (S13.10).

**Deferred / parked:**

- PDF pixel-overlay lane → post-v0; re-entry when PDF deliverables become routine (v0.1 web-research domain or operator demand) [G2 Def.13].
- docx native redlining (Python-Redlines/Docxodus + OOXML-embedded comments) and pandiff prose diffs → re-entry: R13-OQ7 license/maintenance audit passes [XREF:S16] AND a document-heavy domain activates (v1).
- LFS for binaries → only when a binary must be reviewable on GitHub itself [G2 Def.14].
- Rootless-podman container tier for sidecar-needing previews → re-entry: first real project that needs sidecar services; until then the labeled "requires container tier" state ships [R13 §4.8].
- Desktop-GUI preview (xpra-class) → v1+ at most; v0 ships the explicit no-preview state [R13 §2.8].
- Gemini-style shadow repo (mid-step capture) → only if mid-step capture proves necessary [R13 §4.6].
- jj as workspace/snapshot engine → parked at S02 (compat matrix + mainstream adoption are its re-entry, on the watchlist) [XREF:S02; XREF:S14].
- diffity (or successor) as a turnkey review component → re-entry per R13 §4.10 if it matures license-clean with comments+agents coverage; watchlist [XREF:S14].
- X-Wing PQ recipient for snapshots → re-entry when the age Go CLI plugin status verifies; watchlist [XREF:S14].
- Fine-grained-PAT collaborator gap / Free-plan enforced rulesets → watchlist; closure reopens broker transport choice / demotes the broker to defense-in-depth respectively [R13 §4.10; XREF:S14].

**Coverage:**

| Feature-list item | Where |
|---|---|
| 6.1 reviewable change + revision-over-revision navigation | S13.1, S13.2 |
| 6.2 comments on exact places feeding the next revision | S13.3, S13.4 |
| 6.3 accept = one action → attributed commit (D9) | S13.6 |
| 6.4 / S1.9 instant try-out + before-vs-after | S13.8 |
| 5.5 findings carried forward as numbered points (logic → S07) | S13.4 |
| 5.4 notes-vs-blockers travel (bounds → S07) | S13.4 |
| S1.2 follow-ups with visible lineage | S13.9 |
| S1.6 project/repository registry; onboarding-as-task | S13.7 |
| S1.8 native review in the workspace (surface → S15) | S13.1–S13.3 |
| S1.11 accept-time collision → reviewable merge (plan-time claims → S02) | S13.6 step 1 |
| S3.5 review surfaces per type (behavior; widgets → S15) | S13.2, S13.3 |
| 2.6 automation definitions presented as deliverables (governance → S08) | S13.1 |
| D9 per-project git, per-user remotes, platform snapshot repo | S13.5–S13.7, S13.10 |
| D2 credentials broker-held, never in sandboxes (git scope) | S13.6 |
| 11.3 encrypted snapshots + tested restore + secrets separation | S13.10 |

**Open items for G4:** none. Drafting-time sub-choices are flagged inline as [coordinator-draft]: the `refs/sinet/…` revision-retention namespace (S13.1), the restore-drill cadence (3 mo), the preview idle-stop (15 min) and concurrency cap (3), and the proposed clamp ranges in the ⚙ table. The anchor-model shape follows the binding frontend workshop artifact (layer 3) where it compacts R13 §4.1's selector sketch — recorded here for traceability, no conflict left open.
