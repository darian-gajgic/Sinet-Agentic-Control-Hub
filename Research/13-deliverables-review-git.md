# 13 — Deliverables, review & git integration

**Topic:** T12 · **Wave:** B2 · **Depth:** FULL (medium breadth — mechanics over UI polish) · **Written:** 2026-07-17
**Method:** deep-research harness — 5 fan-out angles (diff/anchoring mechanics · accept→commit identity & credentials · git topology for parallel agent writers · disposable previews · encrypted snapshots to GitHub), ~140 extracted findings with primary-source fetching, then a **3-vote adversarial verification pass over the 12 most load-bearing claim bundles: 24 votes — 22 SUPPORT / 2 PARTIAL / 0 REFUTE**. Both PARTIALs are folded in as corrections: (1) Claude Code worktrees branch from a *recently-refreshed* origin/HEAD (fetch only when >24 h stale, 5 s cap, cached fallback), not an unconditional fresh fetch; (2) the Hypothesis orphaned-annotation figure is **~22%**, not 27% (arXiv 1512.06195 abstract). Verifier precision notes also folded: GitHub docs phrase the Free-plan protection gap as feature *availability* (rulesets/branch protection listed public-repo-only on Free; push rulesets need Team+); contribution-graph email must be "connected" (docs wording), not "verified"; age's post-quantum recipients are hybrid ML-KEM-768 (X-Wing); Gerrit's CommentPorter is cited via its source-file commit (the rendered doc URL 404s); jj docs live at docs.jj-vcs.dev. **Provenance caveat:** the encrypted-snapshots angle's full deliverable text was lost after verification; its conclusions are captured from the agent's progress reports and were part of the verification pass. Citations for that angle were re-established against primary sources on 2026-07-17 (marked "re-established" in §8); the two claims whose exact original sourcing could not be reconstructed are marked "distilled, single-provenance" inline. All URLs accessed 2026-07-17. Engine/inference spend: $0.

---

## 1. Scope

Feature-list items covered (per brief T12): **§6** (everything arrives as a reviewable change; comment-anchored review; accept = one action → attributed commit; instant try-out), **S1.8–S1.9** (native review, disposable preview), **S1.11** (parallel same-project coordination — artifact level; state mechanics are report 08 §4.9's, field validation is here), **S3.5** (review-surface mechanics; frontend stack itself is T13), **D9** (per-project git, per-user GitHub remotes, invited collaborators), **11.3** (encrypted platform-state snapshots to GitHub + tested restore), **5.5** (feedback anchored to exact places, carried into retries as numbered points).

Binding inputs consumed as settled, cited never re-derived: **G1 ratified architecture** (SQLite-WAL event log, sole-controller posture, systemd transient units + bwrap sandbox stack, reflink workspaces; all ⚙ numbers operator-editable settings), **report 03** (approval cards — the accept action is a card; frozen numbered ACs), **report 04** (two-axis verification ladder — review findings enter it), **report 08** (durable-state layer: `artifact_claims` registry mechanics §4.9, effect journal §4.5, 11.3 durable-set and dump shape §4.7, snapshot-ref-per-checkpoint, R08-OQ5 backup posture), **report 10** (confinement classes, ssh-agent-shaped credential broker, gh-aw safe-outputs pattern), **report 14 §6.2** (N15/N19 verdicts pre-registered to defer to this report — position stated in §6). Hard constraints: D9 (accepted work = attributed commit by the accepting user), D7 (push is a gated proposal until accept), D2 (per-user credentials, broker-held, never in sandboxes), adopt-don't-fork, subscription-coverage rule (nothing here consumes paid inference).

---

## 2. Current state of the art (mid-2026)

### 2.1 Review surfaces per format — mostly adoptable, two license landmines

**Code/text.** GitHub-style diff components are commoditized. The 2026 agent-tool convergence is on GitHub-style rendering: `@git-diff-view/*` (React/Vue/Solid/Svelte/Ink, split+unified, claimed sub-second template rendering on 10k-line diffs, releases through ~2026-06) and **diffity** — an "agent-agnostic, GitHub-style diff viewer and code review tool that works with Claude Code, Cursor, Codex" whose comments agents read and resolve via `/diffity-resolve` — near-identical prior art for Sinet's loop [S9, S10, P]. The primitives beneath: diff2html (MIT) for unified-diff-text → GitHub-style HTML with word/char intraline highlighting [S5, P]; jsdiff (BSD-3, v9-era, actively maintained) for computing intraline diffs [S6, P]; @codemirror/merge (MIT) for split/unified editor views [S3, P]. Monaco's diff editor is IDE-grade but ~5–10 MB uncompressed vs CodeMirror's tree-shakeable ~50–300 KB — wrong weight for a read-mostly dashboard [S1, S2, S tier]. **Health-check trap:** CodeMirror/ProseMirror repos show "archived" on GitHub (2026-04) — that is a hosting migration to the maintainer's self-hosted forge (code.haverbeke.berlin, active 2026 commits), not abandonment [S3, S4, P]. google/diff-match-patch *is* genuinely archived (read-only since 2024-08-05); its bitap fuzzy-match primitive lives on in maintained ports (`diff-match-patch-rs`, Rust/wasm, with a cross-library compat mode) [S7, S8, P].

**Markdown/prose.** pandiff produces semantic prose diffs for any pandoc format — CriticMarkup, HTML, or docx-with-track-changes output, "respecting document structure to avoid producing broken markup"; a Jan-2026 practitioner writeup confirms it works today (single-maintainer project; activity level unverified — flagged) [S11, S12, P/B].

**Office documents.** Docx redlining is genuinely off-the-shelf in 2026: **Python-Redlines/Docxodus** emits native docx tracked-changes redlines (default engine: a modernized .NET 8 fork of Open-XML-PowerTools with move/format-change detection, prebuilt binaries embedded in the wheels; browser-WASM sibling exists; license unverified — flagged) [S13, P]. LibreOffice document-compare is UI/macro-only — **there is no `soffice --compare` flag**; headless CLI only converts [S14, S15, P]. The lightweight text-level fallback is the `redlines` PyPI package [S16, P].

**PDF.** Visual-first, old but alive: vslavik/diff-pdf (pixel overlay, GPL) and JoshData/pdf-diff (extracted-text compare) — neither is a polished embeddable component; expect glue work [S17, S18, P]. Extraction licensing: PyMuPDF is ~8–12× faster but **AGPL-3.0**; pdfplumber is MIT and better at tables; an arXiv comparison found PyMuPDF/pypdfium2 generally strongest on text [S19–S21, A/B/P]. Draftable — the commercial cross-format benchmark — confirms there is no open-source equivalent (REST API or paid self-hosted container only): cross-format compare is buy-not-build; stay per-format [S22, P].

**Images.** GitHub's three review modes are the idiom to copy — **2-up, Swipe, Onion Skin** (opacity slider; documented in GitHub's non-code-files docs; all trivially implementable as CSS/JS over two `<img>`s) [S23, P]. Pixel-diff engines: pixelmatch (Mapbox, few hundred LOC, browser+Node) and odiff (MIT, Zig+SIMD, v4.3.8 2026-04-17, adopted by Argos/Lost Pixel/playwright-odiff) [S24, S25, P].

### 2.2 Anchored comments and anchor drift — two archetypes, one honest design

The field splits into **external anchor records** (must be migrated across revisions) and **content-embedded markers** (migrate for free, die with the text):

- **GitHub PRs** anchor by `{commit_id, path, line, side, start_line}`; the old diff-offset `position` parameter "is closing down." GitHub **does not re-anchor**: comments superseded by later commits are marked *outdated* with their original diff-hunk context preserved; a Sept-2025 change added commenting on unchanged lines; a known UX hole makes comments vanish from Files-changed when a later commit touches the line [S26–S28, P]. The industry's biggest review system chose "mark stale + preserve context" over re-anchoring — a legitimate cheap fallback tier.
- **Google Docs** is the cautionary tale: Drive API docs state anchors "are immutable, and their position relative to the content of a document cannot be guaranteed between revisions"; for Docs/Sheets/Slides the anchor field is saved but *ignored* — API comments appear unanchored. The polished in-editor behavior is proprietary and unreplicable via API [S29, S30, P].
- **Word (OOXML)** embeds anchors in content: `commentRangeStart/End` + `commentReference` linked by id — anchors move with text by construction [S31, P]. For docx deliverables Sinet can embed comments natively.
- **W3C/Hypothesis** is the strongest re-anchoring model: store redundant selectors (RangeSelector + TextPositionSelector + TextQuoteSelector with 32-char prefix/suffix), try strategies in order, end with bitap fuzzy search; selectors "hint each other" [S32, S33, P]. Failure is quantified: **~22% of sampled Hypothesis annotations were already orphaned** [corrected figure — arXiv 1512.06195 abstract], and 53% of still-attached ones were at risk from page changes; Hypothesis ships an explicit *orphaned* state [S34, S35, A/P]. Library reality: Apache Annotator **retired from the Apache Incubator 2025-08-11**; dom-anchor-* micro-libs unmaintained (single-source on staleness — flagged); the living implementation is the Hypothesis client's anchoring code — adopt the data model, expect to vendor ~small anchoring logic [S36, S37, P].
- **Editor-native anchors** (Yjs relative positions, ProseMirror position mapping) are superior *inside a live editor only* — they do not survive Sinet's regeneration boundary (agent retries produce new files, not shared editing sessions) [S38, S39, P].
- **Gerrit is the closest prior art for revision lineage + comment porting.** One logical change accumulates numbered patchsets; since 3.4, `GET ported_comments/` maps comments onto later patchsets, and the CommentPorter source states the degradation contract verbatim: a comment that "can't be matched to its exact position in the target patchset… we'll map it to its next best location. This can also include a transformation of a line comment into a file comment" (cited via the opendev source commit; the rendered Javadoc URL 404s) [S46–S48, P]. Phabricator died 2021; the live fork Phorge retains the same schema shape — revision entity, immutable numbered diffs 1..N, comments per (diff, file, range), arbitrary diff-vs-diff comparison [S51, S52, P/S]. GitHub's minimal lineage record per force-push — (old_head, new_head), with back-to-back events compressed — shows why Sinet should persist *every* revision hop [S49, S50, P/S].

### 2.3 Findings→comments→retry loops are mainstream, not novel risk

Claude Code `/code-review` posts findings as inline PR comments (`--comment`) or applies them (`--fix`) [S40, P — in-environment skill definition]. Copilot code review ships inline diff comments with description + one-click suggested change + severity, and "Fix with Copilot" hands a finding to the coding agent [S41, P]. Bugbot (Cursor) and CodeRabbit spawn dedicated fixing agents from findings, human gate before merge; Cursor's 52%→76% resolution figure is vendor-reported via aggregators (flagged) [S42, S43, P/S]. Academic support for the format: structured feedback with explicit localization consistently improves repair rates (FeedbackEval; Loc2Repair across three repair backbones) [S44, S45, A]. **Gap that matters:** no published study isolates *comment-anchored* (vs test/trace) feedback for agent retries — Sinet's 5.5 design rests on adjacent evidence and is a design hypothesis to measure in-house (→ §7).

### 2.4 Accept→commit: identity, attribution, signing, credentials, limits

- **Identity is a per-invocation solved problem**: env vars / `-c user.name=… -c user.email=…` / `--author` override all config, so the accept step sets author=committer=accepting-user with zero global host state [S53, P]. Contribution-graph attribution is purely email-based and default-branch-gated: "Commits must be made with an email address that is **connected** to your account on GitHub, or the GitHub-provided noreply email" [docs wording — precision-corrected], counted only on the default branch (or gh-pages) [S54, P]. The **ID-based noreply form** (`<id>+<username>@users.noreply.github.com`) survives username renames; the old username-only form does not — the per-user store should capture the ID-based form [S55, P].
- **Attribution polarity is vendor-split with no standard**: Claude Code ships human-as-author + `Co-Authored-By: Claude …` trailer; Copilot coding agent inverts it (Copilot authors, human co-authors) [S65–S67, P]. The VS Code `git.addAICoAuthor` affair (default flipped to "all" ~2026-04, mis-attributed non-AI code, reverted ~2026-05) is fresh field evidence that silent agent self-attribution is rejected — attribution text must be deterministic and user-visible at accept time [S68–S70, S/P]. An `Assisted-by:`/`Generated-by:` trailer movement is emerging in OSS (argued semantically better than Co-authored-by's implied personhood; Artsy RFC merged 2026-06) but is not a standard [S73–S75, B]. Co-authored-by syntax is strict (blank line before trailers; email-matched for credit) [S64, P].
- **Signing:** GitHub verifies exactly GPG, SSH, S/MIME — no Sigstore; gitsign commits still render Unverified (open since 2022) [S56, S57, P]. Vigilant mode makes signing all-or-nothing per user: enable it only if *every* commit is signed with a connected committer email [S58, P]. The "free Verified" path — API-created GitHub-App commits — is **voided by any custom author/committer info**, so it is structurally incompatible with D9's human attribution [S56, S59, P]. Practitioner consensus: per-user SSH signing keys are the workable automation option; signing is optional hardening at household trust level [S60–S62, B].
- **Credential transport — the single most consequential find:** a fine-grained PAT "is limited to access resources owned by a single user or organization" and **cannot be used on repos where the holder is merely an invited collaborator on another personal account's repo** — a documented, still-current gap. D9's shared-project shape (repo under owner's account, members invited) therefore *breaks* with fine-grained PATs for every non-owner member. **SSH keys authenticate as the person and work on any repo that person can write to** — this alone decides the broker transport [S78, P]. Deploy keys authenticate as the repo (wrong attribution story, single-repo); classic PATs are corralled ("recommend fine-grained whenever possible", auto-removed after 1 yr non-use); GitHub Apps are enterprise-correct but heavy and conflict with human attribution [S78, S79, P].
- **Free-plan reality:** unlimited private repos + unlimited collaborators; personal repos have exactly two roles (owner / collaborator — collaborators push directly but cannot manage settings/invites/deploy keys) [S81–S83, P]. **Branch protection and rulesets are listed as available on Free for public repos only; private-repo enforcement needs Pro+ (push rulesets Team+)** [availability phrasing per docs — precision-corrected]; community threads confirm the non-enforcement error in practice [S84, S85, P/S]. Consequence: on Sinet's private household repos **there is no server-side guardrail at all — any collaborator credential can force-push main. Sinet's broker isn't augmenting GitHub's protection; it IS the protection.** ($4/mo Pro per repo-owning user would buy enforced rulesets — operator option, §7.)
- **Push mechanics:** `--force-with-lease=<refname>:<expect>` is a true compare-and-swap (the bare form is "trivially defeated if some background process is updating refs"); `--atomic` covers multi-ref updates [S93, P]. Documented traffic limits: recommended ≤6 pushes/min/repo, 2 GB push cap — household volume is orders of magnitude below everything [S94, P]. **ToS-clean:** one account per human, logins never shared, each person answerable for their account's activity; several distinct humans' accounts operated from one machine with their own credentials violates nothing found; the tripwire is pooling one user's token for others' work (exactly what D2 already forbids) [S96, P].
- **Squash-authorship hazard:** GitHub-side squash of an agent-authored PR can make the agent the author of the landed commit (community-reported, single-source — flagged; the merge docs are silent on squash author rules). Sinet's host-side accept commit avoids the hazard entirely [S91, S92, S/P].

### 2.5 Git topology for parallel agent writers — the field converged on Sinet's shape

- **Worktree mechanics:** linked worktrees share one object store, refs, and (by default) config+hooks; each gets its own HEAD/index. Git *refuses* to check the same branch out in two worktrees (`--force` overrides) — so "stages share a branch" is forced-sequential by git itself, harmless for Sinet's sequential pipelines, fatal for intra-pipeline parallelism on one branch [S97, P]. Shared `.git` internals are a sandbox hazard: an agent editing config/hooks affects every worktree — deny writes outside refs/objects, or use full clones. Submodule support in multiple checkouts is explicitly "NOT recommended" (man-page BUGS) — fall back to Sinet's reflink clones for submodule repos [S97, P]. Per-worktree dependency dirs duplicate (node_modules); never hand-delete worktree dirs [S102, B].
- **What platforms ship:** Claude Code — worktree per session (`--worktree`, desktop auto-creates), branch `worktree-<name>` based on a **recently-refreshed origin/HEAD** (fetches only when the cached ref is >24 h stale, 5 s fetch cap, cached fallback — corrected from "freshly-fetched" by verification), `git worktree lock` while an agent runs, exit cleanup auto-removes only *clean* worktrees [S98, P]. Cursor — worktree per task, default cap 25/machine, 6-hourly sweeps, `/apply-worktree` to land changes [S103, P]. Vibe Kanban — worktree+branch per **task attempt** with a rebase-conflict flow (Bloop the company shut down 2026-04-10; the Apache-2.0 project continues community-maintained) [S104, S105, P]. Hosted agents are uniformly branch-per-task + (draft) PR with incremental commits pushed during the run (Copilot: "can only push to branches it creates"; Devin: review comments → new commits on the same PR — Devin details via search summaries, flagged) [S108–S110, P]. Sculptor is the dissenter that matches Sinet: container-per-agent + sync-back, "parallel agents without the hassle of git worktrees" — Sinet's reflink-clone-in-sandbox is Sculptor-shaped, and that's fine [S107, P]. GitButler's virtual branches solve human attention (one dir), not sandbox isolation — not applicable [S112, S113, B/P].
- **Revision-after-review:** hosted agents append new commits to the same branch/PR; **nobody amends or force-pushes during review**. The shipped retry pattern when the base moved or an attempt is scrapped: a *new attempt* = fresh worktree+branch from the updated base (Vibe Kanban) [S110, S124, P].
- **Stacked PRs** went GitHub-native (private preview 2026-04, `gh stack` CLI) — solves review decomposition of one big change, not parallel-writer isolation; irrelevant to v1 [S125, S126, A].

### 2.6 Snapshot safety nets — commits win; engine checkpoints and jj lose

- **Engine checkpoints must not be the durability net.** Claude Code checkpoints, verbatim limits: "Checkpointing does not track files modified by bash commands… Only direct file edits made through Claude's file editing tools are tracked"; session-scoped, 30-day cleanup [S115, P]. Gemini CLI's trio (shadow-git whole-tree snapshot in `~/.gemini/history/<hash>` + conversation + pending tool call, atomically restorable, off by default) is the proven cheap *pattern* — a second GIT_DIR over the same worktree catches bash side effects too [S114, P].
- **Real commits as the net is long-shipped practice:** aider commits every edit (and pre-commits dirty state so "you never lose your work"); Copilot pushes commits to the draft PR as it works [S116, S108, P]. **git stash is unfit for automation** (untracked files skipped by default; conflicted pop leaves the stash; documented data-loss case) — never in machinery [S117, S118, P/B].
- **jj (Jujutsu):** working-copy-as-commit + op log/undo is elegant, but it snapshots **only when a jj command runs** — operationally identical to Sinet committing at step boundaries, which it already has natural hooks for; the git-compat page lists **git hooks, submodules, LFS, and `git worktree` itself as unsupported**, colocated repos leave git detached-HEAD, "there may still be bugs" (docs at docs.jj-vcs.dev — precision note); and **no mainstream shipped agent platform uses jj** as of mid-2026 (skills and hobby wrappers only; absence-of-evidence after targeted search — flagged) [S119–S123, P/B]. jj's marginal value over platform-owned commits is ~zero for Sinet while its incompatibilities are concrete. Post-v0 candidate at most (consistent with report 08's parenthetical).

### 2.7 Plan-time artifact claims (S1.11) — the field still doesn't ship it

The flagship multi-agent product (Claude Code agent teams) locks only the task-claim datastructure; source-file conflict avoidance is verbatim advisory: "Break the work so each teammate owns a different set of files" [S127, P]. Below products: a niche OSS file-lock MCP middleware (forge-orchestrator, single-source on real usage) and **CoAgent** (arXiv 2606.15376), which explicitly rejects both locks ("block long inference intervals") and OCC ("abort-and-retry discards minutes of work") in favor of a serialization order fixed at launch plus plan repair — research independently converging on Sinet's declare-and-sequence-at-plan-time position [S128–S130, P/R/B]. **Verdict: no mainstream shipped product does plan-time write-set detection; Sinet's glob-intersection claim registry (report 08 §4.9 owns the mechanics) remains ahead of the field — build it, with CoAgent as the direction-is-sound citation.** Accept-time: GitHub's merge queue is the industrial idiom — validate each accept against latest-target-plus-queued-work on a throwaway ref, evict on failure; Sinet's applies-cleanly-on-HEAD gate is a depth-1 merge queue, and a second in-flight accept must validate against HEAD+first [S134, P]. Vibe Kanban ships the clearest conflict-surfacing UX (status → "Rebase conflicts", banner: agent auto-resolve / open editor / abort to new attempt) — direct precedent for Sinet's merge card including delegate-resolution-back-to-an-agent [S124, P]; Cursor/Conductor don't document conflict handling at all (absence — flagged) [S103, S106, P-absence].

### 2.8 Disposable previews — convergent UX, host-toolchain substrate

Every local product converged on: dev server in an isolated workspace + allocated/detected port + (proxied) URL + embedded browser with console visibility. Vibe Kanban: per-workspace dev server, port-pool daemon (agent asks, daemon assigns, returns URL — ~200-line pattern; port story is a secondary-source writeup, flagged), built-in browser [S104, S136, P/S]. Claude Code desktop ships one-click preview (starts dev servers, embeds the app, click-element-to-feedback; mechanism closed) [S137, P]. Cursor: in-IDE agent browser with dev-server/port *detection* — probe the netns for listening ports rather than trusting config [S138, P]. Replit documents the iframe gotchas: server must bind 0.0.0.0 for port auto-detection; multi-port needs a picker [S141, P]. OpenHands' tracker documents how dynamic host ports break behind reverse proxies — a proxy must be told about every ephemeral port programmatically [S139, S140, P].

**Auto-install:** nixpacks is officially in maintenance mode; Railway replaced it with Railpack (active, MIT) — but Railpack and every builder in its family **require a BuildKit/Docker daemon** [S144–S146, P]. The no-container path is genuinely maintained 2026 tooling: **mise** (pinned toolchains per project) + **uv** (verified-synced Python envs per invocation) + pnpm, driven by a small detection heuristic (package.json → npm/pnpm; pyproject.toml → uv; index.html → static server), cribbable from Railpack's providers [S148, S149, P].

**Routing/lifecycle:** Caddy's admin API adds/removes reverse-proxy routes at runtime with zero downtime (`@id`-addressable) [S150, P]. Vite HMR behind a proxy is the known trap — forward WebSocket upgrade headers or HMR silently bypasses the proxy; inject `--host 0.0.0.0` + `allowedHosts` [S151, P]. Path-prefix routing breaks arbitrary apps — subdomain- or port-based only [S152, P/B]. Tailscale: `tailscale serve --bg` per port, or Tailscale Services (GA Feb 2026) — per-service MagicDNS names + virtual IPs independent of the node [S153, P]. **Idle-stop is free from the settled substrate:** `systemd-socket-proxyd --exit-idle-time=` + `StopWhenUnneeded=` gives on-demand start and auto-stop, no bespoke detector [S154, P]. **Throwaway-ness is free from the settled substrate:** bwrap `--overlay-src <workspace> --tmp-overlay` sends all preview writes to an unpersisted tmpfs upper — the preview *cannot* mutate the reviewed workspace (tmpfs = RAM; for huge node_modules use a reflink throwaway clone instead — background inference, flagged) [S155, P].

**Before-vs-after:** no shipped local product runs two branches side by side (absence — flagged); the closest is a Cursor community skill assembling worktree-per-branch + two dev servers + screenshot pairs [S157, P]. The cloud pattern to replicate is Vercel's dual-URL scheme (immutable per-commit URL + mutable branch URL) [S158, P]. Non-web: **ttyd** serves any CLI over HTTP/WebSocket (`--once`, reverse-proxy-friendly); notebooks self-preview (Jupyter is a web server); xpra HTML5 exists for desktop GUIs but is heavy — ship an explicit "no preview for this type" state at v1 [S159, S160, P]. Revision-pinned preview: no packaged "run app as of revision X" tool exists (absence — flagged); the primitive is `git worktree add <path> <commit-ish>` / reflink-clone-at-rev — Nexus's time-travel preview has no off-the-shelf successor and is cheap to rebuild [S135, S97, P].

### 2.9 Encrypted platform-state snapshots to GitHub (11.3)

*(Angle findings verified in the original pass; citations re-established 2026-07-17 except where marked.)*

- **age** is a maintained, boring, single-purpose file-encryption tool/format: BSD-3, v1.3.1 (2025-12-28), spec at age-encryption.org/v1 (→ C2SP repo). Two properties shape the design: **(1) passphrase mode cannot mix with key recipients** — the spec is explicit: "An scrypt stanza, if present, MUST be the only stanza in the header"; so recovery must be *paper key + passphrase-escrowed identity file*, not "encrypt to key OR passphrase" in one archive. **(2) age provides no sender authentication** (verified in original pass; re-established at summary level from the repo — encryption proves *who can read*, never *who wrote*), so a snapshot integrity ledger is needed against substitution [S161, S162, P — re-established]. Post-quantum: the spec now defines **MLKEM768-X25519 (X-Wing)** hybrid recipients (precision note), shipped in typage v0.3.0 (2025-12) [S162, S163, P — re-established].
- **GitHub limits, from primary docs:** file size enforced at 100 MB (recommended 1 MB); push enforced at 2 GB; ≤6 pushes/min recommended; LFS free tier is now **10 GiB storage + 10 GiB bandwidth, metered** (overage blocked without a payment method); release assets: <2 GiB per file, 1000 per release, **no limit on total release size or bandwidth** [S94, S164, S166, P — re-established].
- **Server-side GC is opaque:** after any history rewrite, unreachable objects persist — reachable via SHA in cached views and PR refs — and truly expunging them requires **GitHub Support** to run server-side GC. Bounding snapshot history is therefore client-side make-believe at the remote: rewritten-away blobs linger indefinitely. **This is acceptable for Sinet only because every blob is age-encrypted before push** — and it makes key compromise retroactive (any key that ever encrypted a pushed snapshot decrypts whatever GitHub still holds) [S165, P — re-established].
- **restic-in-git is structurally self-defeating** (rejected in original pass): a restic repository is encrypted binary pack files that prune constantly rewrites ("Rewriting a pack must write the new pack, update the index… and only then delete the old pack") — encrypted, ever-churning binaries are the worst possible payload for git's delta model and for the unreachable-object problem above [S167, P — re-established]. **git-native transparent encryption (git-crypt class) is the wrong shape** for whole-state snapshots: per-file smudge/clean encryption leaks tree structure, filenames, and change cadence, and defeats deltas anyway (distilled, single-provenance — mechanism argument; the verified conclusion stands on the original pass).
- **zstd** (dual BSD/GPLv2, v1.5.7) is the uncontroversial compression stage [S168, P — re-established].

---

## 3. Candidate approaches

**Deliverable/comment schema.** (a) *GitHub-shape*: anchor to (commit, path, line), never re-anchor, mark outdated — cheapest, provably livable at the largest scale, but comments die every revision round, which fights 5.5's carry-forward contract. (b) *Gerrit/Phorge-shape + W3C selectors*: deliverable = long-lived entity, revision = immutable numbered snapshot, comments carry redundant selectors and are *ported* onto later revisions through a degradation ladder with an explicit orphan state — more machinery, matches 5.5/6.1 exactly. (c) *Content-embedded (OOXML/CRDT)*: free migration but only inside formats/editors Sinet controls — a per-format bonus, not a general model. **(b) wins, with (a)'s "preserve original context" as the ladder's floor and (c) applied opportunistically to docx.**

**Diff rendering.** (a) Monaco — IDE-grade, 5–10 MB, editor runtime for read-mostly views; (b) CodeMirror-weight components (@git-diff-view / diff2html + jsdiff) — GitHub-style, small, no editor semantics; (c) per-format specialists (pandiff, Python-Redlines, diff-pdf + pdfplumber, pixelmatch/odiff). **(b)+(c): a component per format, no editor runtime.**

**Credential transport for push-as-user.** (a) Fine-grained PATs — killed by the collaborator gap (§2.4); (b) deploy keys — authenticate as the repo, wrong attribution, one repo per key; (c) GitHub App — hour-lived installation tokens, enterprise-correct, heavy, and its free-Verified signing is voided by human attribution anyway; (d) **per-user SSH keys held by report 10's broker** — work on owned *and* collaborated repos, no expiry churn, one manual upload per member at enrollment, same key type can double as SSH *signing* key. **(d) wins on every axis that matters here.**

**Topology for agent writers.** (a) Worktrees-per-run over one shared object store — cheap, but shared config/hooks under a sandboxed writer and submodule breakage; (b) **reflink clones (already G1-ratified workspaces)** — full isolation incl. `.git`, ~free on btrfs/XFS, Sculptor-shaped; (c) jj — rejected (§2.6); (d) GitButler virtual branches — solves a different problem. **(b) primary; (a) acceptable for platform-owned (non-sandboxed) utility checkouts like preview-at-rev.**

**Snapshot safety net.** (a) Platform-owned snapshot commits on the run branch at step boundaries — tree-level capture incl. bash side effects, zero new tooling, GitHub-pushable, audit-attributable; (b) Gemini-style shadow repo — adds mid-step capture without polluting branch history, cheap to bolt on later; (c) engine checkpoints — disqualified (bash-blind, expiring); (d) stash — disqualified. **(a) now, (b) only if mid-step capture proves necessary.**

**Preview substrate.** (a) Containers-per-preview (Railpack/devcontainers) — drags a BuildKit/Docker daemon into every click, the cloud multi-tenant pattern solving problems Sinet doesn't have; (b) **host-toolchain heuristic runner (mise/uv/pnpm) inside the already-settled sandbox stack** — what every shipped local product does, confinement and throwaway-ness already paid for by systemd+bwrap+netns; (c) rootless podman reserved for the one thing host toolchains can't fake: sidecar services (Postgres/Redis), surfaced as "needs the container tier". **(b) with (c) as the labeled escape hatch.**

**11.3 encryption.** (a) `tar | zstd | age` whole-archive per snapshot — one encrypted blob per snapshot, keep-N, no structure leakage; (b) restic-in-git — structurally self-defeating (§2.9); (c) git-crypt-class transparent per-file encryption — leaks structure, defeats deltas; (d) Litestream continuous replication — complementary local/off-host DB replication (report 08 R08-OQ5), not the GitHub snapshot mechanism. **(a) wins.**

---

## 4. Recommendation for Sinet

### 4.1 Deliverable schema (feeds G2 directly)

Adopt the Gerrit/Phorge shape: **deliverable** (long-lived entity per task output) → **revisions 1..N** (immutable numbered snapshots; every revision hop persisted — never compress lineage like GitHub's force-push timeline) → **comments** attached to (revision, file, range) with a redundant anchor record: `{path, range, TextQuoteSelector(exact, prefix32, suffix32), TextPositionSelector(start,end), revision_id}`. Any revision pair is diffable on demand (round-over-round, 6.1). Findings from report 04's verification ladder and human review comments share one schema; a finding adds `{severity, description, suggested_change?, finding_number}`.

### 4.2 Re-anchoring: a degradation ladder with an explicit orphan state

Port comments onto revision N+1 in order: **(1)** map offsets through the known N→N+1 diff (Sinet's closed world makes this strictly easier than Hypothesis's open-web problem — the diff is always available); **(2)** TextQuote exact match; **(3)** fuzzy bitap match seeded by position (vendor the small anchoring logic; use a maintained diff-match-patch port, never the archived original); **(4)** Gerrit's next-best location, including line-comment → **file-level comment**; **(5)** **orphaned** — an explicit schema state that keeps the quote + original revision link and stays visible (never silently dropped; GitHub's preserve-original-context is the floor, Hypothesis's ~22% orphan rate is the proof the state is mandatory). For docx deliverables additionally embed comments natively in OOXML so users' own tools see them.

### 4.3 Per-format diff surfaces (all MIT/BSD unless noted)

| Format | Compute | Render | Notes |
|---|---|---|---|
| Code/text | `git diff` + jsdiff intraline | diff2html or @git-diff-view (study **diffity** as loop prior art) | No Monaco; CodeMirror-weight only |
| Markdown/prose | pandiff | its HTML/CriticMarkup output | Single-maintainer — pin + fallback to line diff |
| docx | Python-Redlines/Docxodus | native track-changes docx (+ WASM viewer) | Verify license before adoption (flagged unverified) |
| PDF | pdfplumber extracted-text diff (MIT) + diff-pdf pixel overlay (GPL, subprocess) | two lanes: content diff + layout overlay | AGPL PyMuPDF only as a deliberate later swap; glue work budgeted |
| Images | pixelmatch (browser) / odiff (server) | GitHub's 2-up / Swipe / Onion-skin trio | Trivial CSS/JS |

### 4.4 Findings→retry contract (5.5)

Anchored findings drain into the next attempt as numbered points `[F1..Fn]`, each carrying anchor + description + severity + optional suggested change — the Copilot finding schema, delivered the Claude-Code-`/code-review` way, judged on re-review only against the frozen criteria (report 03's approved-ACs; 5.4). This loop shape is now shipped practice across four vendors — mainstream, not novel risk. The specific claim that *comment-anchored* feedback beats freeform for retries is empirically untested anywhere — pre-register it as an 11.2 benchmark question (→ §7).

### 4.5 Accept flow (6.3, D9)

One action on the report-03 approval card produces, host-side, via the report-10 broker:

1. **Applies-cleanly gate** — candidate applied to current project HEAD on a throwaway ref (depth-1 merge queue; a second in-flight accept validates against HEAD+first). Collision → reviewable **merge card** (Vibe Kanban shape: agent-auto-resolve / manual / abort-to-new-attempt), never a silent overwrite (S1.11 verbatim).
2. **One clean attributed commit**: author = committer = accepting user (per-invocation env/`-c`; ID-based noreply or connected email from the per-user store so graphs/avatars attribute), Conventional-Commit subject, body carries the task/session link as provenance (the Copilot pattern) — the agent's incremental snapshot commits stay in the run workspace, squashed by the *platform*, never by GitHub UI (avoids the squash-authorship hazard).
3. **Attribution trailers**, deterministic and shown on the card before accept (the VS Code lesson): `Co-Authored-By: <engine> <model> <noreply@vendor>` (GitHub renders it) + `Assisted-by: <engine> (<model>) via Sinet` as the machine-parseable provenance line. Trailer text is a config constant.
4. **Push as the user** over broker-held SSH keys with explicit CAS: `--force-with-lease=<ref>:<sha-at-approval>`; pacing ≤6 pushes/min/repo. The broker enforces protected-ref policy (main only via accepts) — load-bearing, because Free private repos have zero server-side enforcement.
5. **Signing (optional hardening, default per §7 OQ1):** if a user opts in, the broker SSH-signs every commit it makes as them (`gpg.format=ssh`, signing key uploaded to GitHub) — all-or-nothing per user; users who don't sign must not enable vigilant mode. gitsign/Sigstore: never (renders Unverified).
6. **Enrollment per member (one-time):** generate SSH keypair inside their per-person store (never leaves host; report 10's plaintext-store caveat applies — 0700 + encrypted backups), user uploads pubkey (auth + optionally signing) to their GitHub account; owner invites collaborators (only owners can — route onboarding steps to the owning user). Repo transfer is the exit path if an owner leaves (1-day acceptance; rewrite stored remotes, don't trust redirects).

### 4.6 Git topology + safety net (feeds G2)

- **Branch-per-pipeline** (deliverable) carrying **platform-owned snapshot commits** after each pipeline step (tree-level, catches bash side effects; junk-excluded per N9); squash to the attributed commit at accept. **Fresh branch per attempt** only when the base moved or an attempt is abandoned (the Vibe Kanban attempt model). During a review round-trip: same branch, new commits — never amend/force-push mid-review (zero field support).
- **Workspaces stay reflink clones** (G1) — full `.git` isolation under sandboxed writers, no shared-hooks/config hazard, submodules safe. Plain worktrees are fine for platform-owned utility checkouts (preview-at-rev, accept staging); apply Claude Code's lifecycle rules: `worktree lock` while running, sweep with `worktree prune`, never auto-delete dirty.
- **No jj, no stash, no engine checkpoints as the net** (§2.6). Add a Gemini-style shadow repo later only if mid-step capture proves necessary — it's cheap to bolt on.
- **Documents ride the identical machinery** — no surveyed platform has a separate versioning model for docs (absence-verified); D9's "document store" = the same per-project git repo. Text in-repo; generated binaries (images/PDFs regenerated per round bloat a plain repo) → local object dir referenced by path+hash from committed text, LFS only if a binary must be reviewable on GitHub itself (10 GiB metered free tier).

### 4.7 S1.11 coordination

Report 08 §4.9 owns the mechanics (claims table, glob-intersection, sequence-or-branch decision card, sibling-accept freshness trigger). This report adds the field verdict: **nothing shipped does plan-time write-set detection — build it as designed**; CoAgent is the independent-convergence citation; the accept-time merge card design (§4.5.1) exceeds the field's documented state.

### 4.8 Preview stack (6.4, S1.9 — composed entirely of settled substrate + two small novel parts)

Heuristic runner (package.json→pnpm/npm, pyproject→uv, mise.toml→mise, index.html→static; cribbed from Railpack's providers) → install into a **bwrap tmp-overlay upper** (or reflink throwaway clone when node_modules-sized) over the read-only workspace → dev server inside the run-sandbox netns, bound 0.0.0.0, port from a pool daemon → **Caddy admin-API route per live preview** (subdomain/port routing, never path-prefix; forward WebSocket upgrades + inject `allowedHosts` or Vite HMR dies) → household access via `tailscale serve`/Tailscale Services → **systemd-socket-proxyd idle-stop** → teardown deletes the upper. Port *probing* of the netns (Cursor's detection heuristic) over trusting config. Non-web: ttyd for CLIs, Jupyter self-previews, static server; explicit "no preview available" state for desktop/embedded at v1. **Before-vs-after (S1.9): two preview instances — accepted rev via worktree/reflink-at-rev (immutable, Vercel's commit-URL analogue) + candidate — in a dual-iframe view with synced navigation. The comparison UI and the ~200-line port-pool daemon are the only novel code in the entire preview feature.** Sidecar-needing projects surface as "requires container tier" (rootless podman), never default.

### 4.9 Platform-state snapshots (11.3) + tested restore

Pipeline per scheduled snapshot: **report 08 §4.7's durable set** (`VACUUM INTO` → `.dump`, traces excluded) + text-first exports → `tar | zstd | age -r <operator recipients>` → one encrypted blob committed to the operator's private snapshot repo, **keep-N on one branch** (⚙ N), pushed CAS.

- **Blob discipline:** stay under the enforced 100 MB file limit (chunk the archive at ~90 MB if ever needed); pushes are far below the 2 GB cap. If snapshots outgrow repo comfort, the escape hatch is **release assets** (<2 GiB/file, unlimited total; also the closer-to-true-delete lane, since normal repo history-rewrite provably does *not* delete server-side).
- **History bounding is client-side only** — rewritten-away blobs persist unreachably on GitHub and truly deleting them needs GitHub Support. Acceptable **only because blobs are age-encrypted**; consequence: key compromise is retroactive over everything ever pushed (→ P-T12-3).
- **Recovery material:** because scrypt/passphrase stanzas cannot mix with key recipients, recovery = **paper copy of the operator's age identity** + a passphrase-encrypted copy of the identity file stored off-host; snapshots encrypt to the key recipient(s) only. Optionally add an X-Wing hybrid recipient (spec'd; typage-shipped; Go-CLI plugin status unverified — flagged) for PQ hedging.
- **Integrity:** age gives no sender authentication — keep a **SHA-256 snapshot ledger** (hash + size + created-at per snapshot) in the platform DB *and* inside each subsequent archive; the restore drill verifies the fetched blob against the ledger before trusting it.
- **Restore drill (⚙ scheduled platform task, per report 08):** fetch newest blob from the remote → verify ledger hash → decrypt with the *escrow* identity (proves recovery material, not just the live key) → unpack → rebuild DB from dump → `integrity_check` + invariant checks. Backup that isn't restore-tested doesn't exist. This closes R08-OQ5's mechanics: dump-snapshots are the load-bearing 11.3 mechanism; pinned Litestream remains the optional *local/off-host continuous* replication addition, unrelated to the GitHub lane.

### 4.10 What would change the decision

- **Fine-grained PATs gain collaborator-repo access** (GitHub closes the documented gap) → PATs become a viable SSH alternative; broker transport choice reopens (watchlist item).
- **Free-plan private repos gain enforced rulesets** → server-side guardrails exist; broker gating drops from "the only protection" to defense-in-depth.
- **A maintained turnkey W3C-selector anchoring library emerges** → vendor less, adopt more (Apache Annotator's retirement is why we vendor today).
- **jj ships worktree/LFS/hooks compat + a mainstream platform adopts it** → re-evaluate as the workspace-snapshot layer (post-v0 reminder).
- **11.2 measures that anchored-comment retries don't outperform freeform feedback** → simplify 5.5's machinery to file-level numbered findings.
- **A Sinet deliverable class routinely exceeds ~90 MB encrypted** → move snapshot lane to release assets or a second target (already the spec's named hatch).
- **diffity or a successor matures into an embeddable, license-clean review component covering comments+agents** → adopt instead of composing diff2html + own comment layer.

---

## 5. What NOT to use and why

- **Fine-grained PATs as broker transport** — cannot push to repos where the member is an invited collaborator (documented resource-owner gap): breaks D9's shared-project shape outright.
- **GitHub App auto-signed "Verified" commits** — voided by any custom author/committer; structurally incompatible with attribute-to-the-human (D9).
- **gitsign/Sigstore** — renders Unverified on GitHub (no Fulcio in its trust root; open since 2022): worse than no signature.
- **GitHub-side squash-merge of agent branches** — community-reported authorship flip to the agent; Sinet's host-side accept commit makes it moot — keep it that way.
- **git stash anywhere in machinery** — untracked-file skips, conflicted-pop retention, documented data loss.
- **Engine checkpoints as the durability net** — Claude Code's are bash-blind and expiring; Gemini's are off by default; both are session-scoped conveniences.
- **jj as v1 VCS layer** — incompatible with worktrees/hooks/submodules/LFS; snapshot trigger no better than platform commits; zero mainstream platform adoption.
- **Monaco for the review dashboard** — 5–10 MB editor runtime for read-mostly diffs.
- **PyMuPDF by default** — AGPL-3.0; pdfplumber (MIT) covers v1 needs. (AGPL is *tolerable* for this repo, but zero-license-thought defaults win for bus-factor-1.)
- **`soffice --compare`** — does not exist; LibreOffice compare is UI/macro-only. Python-Redlines is the docx path.
- **Google-Docs-style external comment anchors without a ladder** — their own API docs admit anchors aren't honored; the imitable models are OOXML embedding and Gerrit porting.
- **nixpacks** (maintenance mode) and **container-native auto-builders per preview** (Railpack/pack/devcontainers force a BuildKit/Docker daemon into every click) — host-toolchain runner instead; containers only as the labeled sidecar tier.
- **Path-prefix preview routing** — breaks arbitrary apps (hardcoded `/` assets); subdomain/port only.
- **restic-in-git** — encrypted pack churn vs git's delta model; self-defeating on the unreachable-object problem.
- **git-crypt-class transparent encryption for snapshots** — leaks tree structure/filenames/cadence; defeats deltas (distilled, single-provenance mechanism note; conclusion verified in original pass).
- **Relying on remote history rewrite to bound snapshot storage** — GitHub keeps unreachable objects until Support intervenes; bound client-side and treat the remote as append-only-ish.
- **`--force-with-lease` bare form** — trivially defeated by background ref updates; explicit `<ref>:<expect>` CAS only.

---

## 6. Harvest-map verdicts

Report 14 §6.2 pre-registered that this report owns the final shape for N15/N19 and wins on divergence. **Position: no divergence — both of report 14's verdicts are confirmed, with mechanics sharpened below.** Coordinator: no reconciliation edit needed to the §6.1 consolidated table beyond noting this report as the design owner.

- **N9 (worktree.py — branch-per-pipeline, snapshot commits, three-dot capture_diff) → CONFIRM, two riders.** The field's deliverable container is exactly one long-lived branch per task accumulating commits across the run and review rounds; platform-owned snapshot commits are the shipped-at-scale net (Copilot push-as-you-go, aider). Git's same-branch-one-worktree rule makes stages-share-a-branch forced-sequential — harmless for Sinet's sequential pipelines, a hard block on future intra-pipeline parallelism (accepted). Riders: **(1)** mint a fresh branch per *attempt* on base-move/abandon (Vibe Kanban's attempt model — the field pattern N9 lacked); **(2)** N9's shared-`.git` worktree substrate is superseded by G1's reflink-clone workspaces for sandboxed writers (shared config/hooks are a confinement hazard); keep worktrees for platform-owned utility checkouts with Claude Code's lock/prune/never-delete-dirty lifecycle.
- **A7 (Archon worktree manager — STUDY) → CONFIRM (STUDY), demoted.** The study value Archon offered is now better documented by shipped products: Claude Code's worktree lifecycle rules, Cursor's caps+sweeps, Vibe Kanban's attempt/rebase flow. Study those first; A7 only if a gap remains.
- **N15 (Review v2 — PR-style review of every output type; findings→anchored-comments→retry) → CONFIRM (pattern), port post-gate as planned.** The loop shape is now mainstream across four vendors (§2.3) — validated, not novel risk — while **no OSS covers the full breadth** (report 14's sweep + this report's: diffity is code-only prior art; Draftable confirms cross-format compare has no OSS equivalent). Design deltas the port must absorb, from this report: the Gerrit degradation ladder + explicit orphan state (N15's old/new-position anchoring alone is the GitHub model and loses comments across regeneration rounds); redundant W3C-style selectors; per-format surface matrix (§4.3); findings schema gains severity + suggested-change (§4.4).
- **N19 (app_runner + time-travel preview) → REVISE (consistent with report 14).** Capability spec-required, no donor emerged, and the substrate changed twice over: disposable *venvs* → heuristic host-toolchain runner (mise/uv/pnpm) inside **report 10's C2-class sandbox** (netns port remap, bwrap tmp-overlay for zero-mutation); port remap → port-pool + Caddy admin-API + socket-proxyd idle-stop; time-travel preview rebuilt on reflink/worktree-at-rev (no off-the-shelf successor exists). Keep post-Review-v2 priority. Previews remain read-only effects inside the gate model.

---

## 7. Open questions

**Operator decisions:**

1. **Signing default at enrollment** — broker can SSH-sign every accept-commit per user (green Verified, one extra pubkey upload per member) or skip signing (neutral badge; users must not enable vigilant mode). All-or-nothing per user. *Proposal: operator signs from day one; members opt-in at enrollment.* Owner: operator at G2.
2. **GitHub Pro for repo-owning users ($4/mo each)** — buys *enforced* rulesets on private repos, adding a server-side guardrail under the broker (currently the broker is the only protection; a leaked member SSH key can force-push main). *Proposal: no at v0 (operator-only, broker-gated); revisit when a second member joins.* Owner: operator.
3. **Snapshot keep-N and cadence** (⚙): N snapshots on the branch, daily default? Plus: rotate to a fresh snapshot repo annually (bounds even the unreachable-object accumulation) or accept indefinite encrypted remnants? Owner: operator at G2.
4. **PDF deliverables first-class at v0?** No polished embeddable diff component exists — the two-lane glue (pdfplumber text diff + diff-pdf overlay) is real work. *Proposal: v0 renders PDFs with text-diff only; overlay lane post-v0.* Owner: operator/spec.
5. **Binary deliverables lane** — local object dir + hash refs (proposal) vs LFS-by-default (10 GiB metered free, blocked on overage without payment method). Owner: spec (G2), default = local object dir.

**Research/measurement:**

6. **Anchored-vs-freeform retry feedback is empirically open** — no published study isolates it; Cursor's 52→76% is vendor-reported. Pre-register an 11.2 benchmark question: do `[F#]`-anchored findings measurably beat file-level notes? Owner: T11/11.2 practice.
7. **Python-Redlines and pandiff license/maintenance verification** before adoption (both flagged unverified). Owner: spec-time dependency audit (P-T16-2 path-scoped check applies).
8. **Watchlist items:** fine-grained-PAT collaborator gap (its closure reopens transport choice); Free-plan ruleset availability; diffity maturity; jj compat matrix; X-Wing recipient support in the age Go CLI (spec'd + typage-shipped; CLI plugin status unverified). Owner: T11 watchlist cadence.

**New platform problems (→ spec Known-problems list):**

- **P-T12-1 — The broker is the only ref protection.** On Free private repos nothing server-side stops any collaborator credential from force-pushing main; Sinet's broker gating + CAS pushes + per-project snapshot remotes ARE the guardrail, and this must be stated in the spec as a security property with a test (attempted non-accept push to a protected ref must be refused broker-side). Mitigations: OQ2 (Pro), protected-ref policy, key hygiene per report 10.
- **P-T12-2 — Comment orphaning is guaranteed at some rate.** Even closed-world re-anchoring will orphan comments (open-web baseline ~22%); the schema's explicit orphan state, its UI surfacing, and orphaned-findings-still-delivered (as file-level) are platform requirements with tests, not best-effort polish. Silent mis-anchoring (worse than orphaning) needs a spot-check in 11.2.
- **P-T12-3 — Encrypted remnants make key compromise retroactive.** GitHub retains rewritten-away snapshot blobs indefinitely (Support-only deletion); anyone obtaining an old age identity decrypts every snapshot ever pushed. Key rotation does not retroactively protect. Mitigations: identity escrow hygiene (paper + passphrase copy), OQ3's repo rotation, Support-ticket purge as the nuclear option.
- **P-T12-4 — Snapshot substitution is undetectable by encryption alone.** age authenticates readers, not writers; a compromised GitHub account could swap snapshot blobs. The SHA-256 ledger (DB + in-archive) and ledger-verify step in the restore drill are load-bearing, not decorative — the drill must fail closed on mismatch.

---

## 8. Sources

All URLs accessed 2026-07-17. Tiers: P primary (official docs/repo/vendor) · A academic · R reputable secondary · B practitioner · S secondary/aggregator or single-source (flagged). Angle-5 entries marked **[re-established]** were re-fetched 2026-07-17 to replace citations lost with that angle's full text; the claims themselves were verified in the original adversarial pass.

**Diff rendering & format tooling**
- [S1] S — npm-compare.com/codemirror,monaco-editor — bundle-size comparison Monaco vs CodeMirror.
- [S2] S — pkgpulse.com/guides/monaco-editor-vs-codemirror-6-vs-sandpack-in-browser-2026 — 2026 in-browser editor comparison.
- [S3] P — github.com/codemirror/merge — split+unified merge views, MIT; GitHub repo archived 2026-04-15 (hosting move).
- [S4] P — code.haverbeke.berlin — CodeMirror/ProseMirror live forge; active 2026 commits.
- [S5] P — github.com/rtfpessoa/diff2html + diff2html.xyz — unified-diff→GitHub-style HTML, word/char intraline, MIT.
- [S6] P — github.com/kpdecker/jsdiff — BSD-3, v9-era, bundled types.
- [S7] P — github.com/google/diff-match-patch — archived read-only 2024-08-05.
- [S8] P — github.com/anubhabb/diff-match-patch-rs — maintained Rust/wasm port, compat mode.
- [S9] P — github.com/MrWangJustToDo/git-diff-view — GitHub-style diff components (React/Vue/Solid/Svelte/Ink); shiki-plugin/license not independently verified (flagged).
- [S10] P — github.com/nilbuild/diffity — agent-agnostic GitHub-style review tool; `/diffity-resolve` loop; closest prior art for Sinet's review loop.
- [S11] P — github.com/davidar/pandiff — semantic prose diffs; CriticMarkup/HTML/docx-track-changes output; single-maintainer (flagged).
- [S12] B — slhck.info/software/2026/01/14/pandiff-word-track-changes-markdown.html — 2026 working confirmation.
- [S13] P — github.com/JSv4/Python-Redlines + redlines.opensource.legal — native docx redlines; Docxodus .NET 8 engine embedded in wheels; license unverified (flagged).
- [S14] P — help.libreoffice.org …/redlining_doccompare.html — compare is UI feature; headless CLI converts only.
- [S15] B — github.com/Ignema/docx-compare — CLI pipelines pair conversion + external diff.
- [S16] P — pypi.org/project/redlines — text-level track-changes for prose strings.
- [S17] P — github.com/vslavik/diff-pdf — visual overlay compare, GPL (version unverified, flagged).
- [S18] P — github.com/JoshData/pdf-diff — extracted-text PDF compare.
- [S19] A — arxiv.org/html/2410.09871v1 — PDF-extraction comparison; PyMuPDF/pypdfium strongest on text.
- [S20] B — pdfmux.com/blog/pymupdf-vs-pdfplumber — speed + AGPL commercial-license pricing.
- [S21] P — pymupdf.readthedocs.io/en/latest/about.html — AGPL-3.0 licensing.
- [S22] P — draftable.com/rest-api + /pricing — commercial cross-format compare; API/paid self-hosted only.
- [S23] P — docs.github.com …/working-with-non-code-files — 2-up/Swipe/Onion-Skin image review modes.
- [S24] P — github.com/mapbox/pixelmatch — no-dependency pixel diff, browser+Node.
- [S25] P — github.com/dmtrKovalenko/odiff — Zig+SIMD pixel diff, MIT, v4.3.8 (2026-04-17); production users listed.

**Anchoring, lineage, review loops**
- [S26] P — docs.github.com/en/rest/pulls/comments — anchor fields; `position` "closing down"; outdated-comment behavior.
- [S27] P — github.blog/changelog/2025-09-25-… — commenting on unchanged lines; positioning-logic change.
- [S28] S — github.com/orgs/community/discussions/23138 — comments vanishing from Files-changed.
- [S29] P — developers.google.com/workspace/drive/api/guides/manage-comments — anchors immutable, not guaranteed across revisions; ignored by Docs editors.
- [S30] P — issuetracker.google.com/issues/357985444 — API-created Docs comments appear unanchored.
- [S31] P — learn.microsoft.com …/how-to-insert-a-comment-into-a-word-processing-document — OOXML commentRangeStart/End content-embedded anchors.
- [S32] P — web.hypothes.is/blog/fuzzy-anchoring/ — selector redundancy + bitap fuzzy fallback (2013 design, still shipped).
- [S33] P — w3.org/TR/annotation-model/ — W3C selector REC.
- [S34] A — arxiv.org/abs/1512.06195 — orphaned-annotation study; **~22% orphaned** (corrected by verification), 53% of survivors at risk.
- [S35] P — web.hypothes.is/blog/showing-orphaned-annotations/ — explicit orphan state shipped.
- [S36] P — incubator.apache.org/projects/annotator.html — Apache Annotator retired 2025-08-11.
- [S37] B — blog.jonudell.net/2021/09/03/notes-for-an-annotation-sdk/ — dom-anchor-* wrap; staleness single-source (flagged).
- [S38] P — docs.yjs.dev/ecosystem/editor-bindings/prosemirror — relative positions for comments.
- [S39] P — discuss.prosemirror.net/t/how-to-track-comment-positions/4500 — position mapping through steps.
- [S40] P — Claude Code `/code-review` skill definition (in-environment) — findings as inline PR comments (`--comment`) / applied fixes (`--fix`).
- [S41] P — docs.github.com …/use-code-review — Copilot suggested changes + "Fix with Copilot" → coding agent.
- [S42] P — cursor.com/bugbot — Autofix cloud agents; Fix All.
- [S43] S — pondero.ai …/cursor-bugbot-vs-coderabbit-ai-code-review-june-2026 — aggregator; 52→76% resolution vendor-reported (flagged).
- [S44] A — arxiv.org/html/2504.06939 — FeedbackEval: structured feedback → highest repair success.
- [S45] A — arxiv.org/html/2606.30963v1 — Loc2Repair: explicit localization improves resolved rate across backbones.
- [S46] P — gerritcodereview.com/2020-11-18-gerrit-news-jun-nov-2020.html — ported_comments since 3.4.
- [S47] P — opendev.org/opendev/gerrit/commit/717ad95… — CommentPorter source: next-best location, line→file degradation (doc URL 404s; cited via source commit — precision note).
- [S48] P — gerrit-review.googlesource.com/Documentation/concept-patch-sets.html — numbered patchsets, patchset-pair diffs.
- [S49] P — github.blog/changelog/2018-11-15-force-push-timeline-event/ — (old_head,new_head) lineage record.
- [S50] S — github.com/orgs/community/discussions/3478 — back-to-back force-push compression.
- [S51] P — we.phorge.it/w/differences_between_phabricator_and_phorge/ — live fork; Differential revision/diff model retained.
- [S52] S — en.wikipedia.org/wiki/Phabricator — Phabricator EOL June 2021.

**Identity, signing, credentials, GitHub plans/ToS**
- [S53] P — git-scm.com/docs/git-commit — identity resolution order; per-invocation overrides.
- [S54] P — docs.github.com …/troubleshooting-missing-contributions — "connected" email wording (precision-corrected); default-branch gating.
- [S55] P — docs.github.com …/setting-your-commit-email-address — ID-based noreply survives renames.
- [S56] P — docs.github.com …/about-commit-signature-verification — GPG/SSH/S-MIME only; web-flow precedent (author=user, signer=GitHub).
- [S57] P/S — github.com/sigstore/gitsign/issues/40 + docs.sigstore.dev/cosign/signing/gitsign — no Verified badge for gitsign; open since 2022.
- [S58] P — docs.github.com …/displaying-verification-statuses-for-all-of-your-commits — vigilant-mode all-or-nothing.
- [S59] P — github.blog …/commit-signing-support-for-bots-and-other-github-apps/ — auto-Verified voided by custom author/committer.
- [S60] B — jontsai.com/2025/10/17/ssh-commit-signing-part-2-… — SSH signing automation mechanics.
- [S61] B — httgp.com/signing-commits-in-github-actions/ — CI signing pain.
- [S62] B — blog.dbrgn.ch/2021/11/16/git-ssh-signatures/ — SSH signing mechanics (2021, flagged old; unchanged).
- [S63] S — github.com/orgs/community/discussions/164099 — Copilot agent signing gap.
- [S64] P — docs.github.com …/creating-a-commit-with-multiple-authors — strict trailer syntax; email-matched credit.
- [S65] P — code.claude.com/docs/en/settings — attribution setting (Co-Authored-By default, configurable).
- [S66] B — deployhq.com/blog/how-to-use-git-with-claude-code-… — Claude Code attribution behavior.
- [S67] P — docs.github.com …/responsible-use-of-copilot-coding-agent-on-githubcom — Copilot authors, human co-authors; no default-branch push.
- [S68] S — theregister.com/2026/05/04/microsoft_reverses_ai_credit_grab/ — VS Code co-author default reverted.
- [S69] P — github.com/microsoft/vscode/issues/314311 — mis-attribution bug.
- [S70] S — heise.de/en/news/WTF-Microsoft-forces-Co-Authored-by-Copilot-… — corroboration.
- [S71] S — github.com/openai/codex discussions/2807, /9449, issues/19799 — Codex: no default attribution; trailer support unshipped.
- [S72] B — christiantietze.de/posts/2026/03/identify-codex-cli-git-commits-… — Codex audit-trail configuration.
- [S73] B — allthingsopen.org/articles/open-source-ai-contributions-assisted-by-git-trailer-standard — Assisted-by movement.
- [S74] B — fabiorehm.com/blog/2026/03/02/our-coding-agent-commits-deserve-better-… — Co-authored-by critique.
- [S75] B — baristalabs.io/blog/ai-assisted-commits-need-provenance-trailer — provenance-trailer argument.
- [S76] P — github.blog/changelog/2025-03-18-fine-grained-pats-are-now-generally-available/ — FG-PAT GA.
- [S77] P — github.blog/changelog/2024-10-18-… — optional no-expiry FG-PATs.
- [S78] P — docs.github.com …/managing-your-personal-access-tokens — single-resource-owner limit; **collaborator-repo gap** (design-critical, held verbatim in verification); classic-PAT corralling.
- [S79] P — docs.github.com …/managing-deploy-keys — repo-scoped keys; no reuse; 1-h App installation tokens.
- [S80] P — docs.github.com …/authorizing-oauth-apps — device flow for headless enrollment.
- [S81] P — docs.github.com …/githubs-plans — Free: unlimited private repos/collaborators; feature availability lists (precision note: protection features listed public-only on Free).
- [S82] P — github.blog/changelog/2020-04-14-… — unlimited-collaborators change.
- [S83] P — docs.github.com …/permission-levels-for-a-personal-account-repository — owner/collaborator powers.
- [S84] P — docs.github.com …/about-rulesets — ruleset availability by plan; push rulesets Team+ (precision-corrected phrasing).
- [S85] S — github.com/orgs/community/discussions/184363 — non-enforcement error on Free private repos.
- [S86] P — docs.github.com …/transferring-a-repository — transfer semantics; collaborators preserved; redirects.
- [S87] B — ssojet.com/blog/ai-commit-message-tools — Conventional Commits as AI-generation default.
- [S88] B — agensi.io/learn/best-git-automation-skills-ai-agents-2026 — commit-skill conventions.
- [S89] B — dev.to/kenimo49/why-just-squash-merge-no-longer-works-… — squash-vs-preserve debate.
- [S90] B — zenvanriel.com/ai-engineer-blog/running-multiple-ai-coding-agents-parallel/ — worktree isolation norms.
- [S91] S — github.com/orgs/community/discussions/179983 — squash makes Copilot the author (single-source, flagged).
- [S92] P — docs.github.com …/about-pull-request-merges — silent on squash author; rebase updates committer.
- [S93] P — git-scm.com/docs/git-push — force-with-lease CAS semantics; --atomic.
- [S94] P — docs.github.com/en/repositories/creating-and-managing-repositories/repository-limits — 100 MB file (enforced) / 2 GB push / ≤6 pushes-min **[re-established]**.
- [S95] P — docs.github.com …/rate-limits-for-the-rest-api — API limits (broker rarely touches).
- [S96] P — docs.github.com/en/site-policy/github-terms/github-terms-of-service — one account/human; no shared logins; owner responsible.

**Worktrees, topology, safety nets, claims**
- [S97] P — git-scm.com/docs/git-worktree — shared store/refs; same-branch refusal; prune/lock; submodule BUGS note.
- [S98] P — code.claude.com/docs/en/worktrees — worktree-per-session; base = recently-refreshed origin/HEAD (>24 h staleness fetch, 5 s cap, cached fallback — **corrected by verification**); lock-while-running; never delete dirty.
- [S99] A — github.blog/open-source/git/highlights-from-git-2-50/ — maintenance worktree-prune task.
- [S100] P — man.archlinux.org/man/git-worktree.1.en — relative-paths extension.
- [S101] P — github.com/libgit2/libgit2/issues/7210 — relativeWorktrees breaks readers.
- [S102] B — joshtune.com/posts/git-worktree-pros-cons/ — per-worktree dep duplication; stale-metadata gotcha.
- [S103] P — cursor.com/docs/configuration/worktrees — 25-worktree cap; 6 h sweeps; /apply-worktree; conflict UX undocumented.
- [S104] P — github.com/BloopAI/vibe-kanban — worktree+branch per attempt; dev server per workspace; Apache-2.0; sunsetting banner.
- [S105] P — vibekanban.com/blog/shutdown — Bloop shutdown 2026-04-10; community continuation.
- [S106] P — conductor.build — worktree per workspace (Mac orchestrator).
- [S107] P — github.com/imbue-ai/sculptor + imbue.com/sculptor — container-per-agent + sync-back; closest analogue to Sinet's shape.
- [S108] P — docs.github.com/copilot/concepts/agents/coding-agent/about-coding-agent — branch+draft-PR; push-as-it-works; own-branches-only.
- [S109] A — github.blog …/github-copilot-meet-the-new-coding-agent/ — corroboration.
- [S110] P — docs.devin.ai/integrations/gh — PR deliverable; review comments → new commits (via search summaries — flagged).
- [S111] P — github.com/OpenHands/OpenHands/blob/main/AGENTS.md — branch/commit/PR instructions.
- [S112] B — trigger.dev/blog/parallel-agents-gitbutler — ditched worktrees for virtual branches.
- [S113] P — docs.gitbutler.com/features/coding-agents — but CLI; per-branch agent assignment.
- [S114] P — github.com/google-gemini/gemini-cli/blob/main/docs/cli/checkpointing.md — shadow repo at ~/.gemini/history; atomic restore trio; off by default.
- [S115] P — code.claude.com/docs/en/checkpointing — bash-blind; session-scoped; "complement, not replace VCS".
- [S116] P — aider.chat/docs/git.html — auto-commit per edit; dirty-commit protection; /undo.
- [S117] P — git-scm.com/docs/git-stash — untracked default-skip; conflicted-pop retention.
- [S118] B — databasesandlife.com/git-stash-loses-untracked-files/ — documented data-loss case.
- [S119] P — docs.jj-vcs.dev/latest/operation-log/ — op log; snapshot-on-command semantics (docs domain — precision note).
- [S120] B — panozzaj.com/blog/2025/11/22/avoid-losing-work-with-jujutsu-… — snapshot-trigger caveat; hook bolt-ons.
- [S121] P — docs.jj-vcs.dev/latest/git-compatibility/ — hooks/submodules/LFS/worktree unsupported; detached-HEAD colocation.
- [S122] P — github.com/netresearch/jujutsu-workflow-skill — "jj local, git canonical" as a skill only.
- [S123] P — github.com/2389-research/agentjj — hobby wrapper; non-adoption evidence (absence claim — flagged).
- [S124] P — vibekanban.com/docs/core-features/resolving-rebase-conflicts — rebase-conflict status/banner; agent-auto-resolve; abort-to-new-attempt.
- [S125] A — infoq.com/news/2026/04/github-stacked-prs/ — native stacked PRs preview.
- [S126] A — infoworld.com/article/4158575/… — corroboration.
- [S127] P — code.claude.com/docs/en/agent-teams — task-claim file-locking only; file-conflict avoidance advisory (design-critical negative finding).
- [S128] P — github.com/nxtg-ai/forge-orchestrator — niche OSS file-lock middleware (usage single-source, flagged).
- [S129] A — arxiv.org/abs/2606.15376 — CoAgent: rejects locks+OCC; launch-time serialization + plan repair.
- [S130] B — mikemason.ca — "Cursor tried and failed with locking" (single-source, unverified — flagged).
- [S131] B — neon.com/blog/ai-workflows-for-docs-devin — doc deliverables as ordinary PRs.
- [S132] P — git-lfs.com + docs.gitlab.com/topics/git/lfs/ — pointer files; content-addressed storage.
- [S133] B — medium.com/@pablojusue/git-lfs-and-dvc-… — binaries-out-of-git field practice.
- [S134] P — docs.github.com …/managing-a-merge-queue — merge_group = PR + latest base + queued PRs; evict on failure.
- [S135] B — gitkraken.com/learn/git/git-worktree — worktree-at-rev usage.

**Previews**
- [S136] R/S — virtuslab.com/blog/ai/vibe-kanban — port-pool daemon description (not in primary docs — flagged).
- [S137] P — claude.com/blog/preview-review-and-merge-with-claude-code — desktop dev-server preview + element-to-feedback.
- [S138] P — cursor.com/docs/agent/tools/browser — dev-server/port detection heuristic.
- [S139] P — github.com/zparnold/openhands-kubernetes-remote-runtime — per-port hostname model.
- [S140] P — github.com/OpenHands/OpenHands/issues/14905 — dynamic ports break behind reverse proxies.
- [S141] P — blog.replit.com/ports + /devtools — 0.0.0.0 bind requirement; multi-port picker.
- [S142] P — codesandbox.io/docs/sdk — microVM snapshot/resume; programmable ports (cloud pattern reference).
- [S143] R — northflank.com/blog/sandbox-providers — checkpoint-resume as cloud table stakes.
- [S144] P — blog.railway.com/p/introducing-railpack — nixpacks maintenance mode; successor rationale.
- [S145] P — github.com/coollabsio/coolify/issues/7983 — downstream confirmation.
- [S146] P — github.com/railwayapp/railpack — active (v0.31.1, 2026-07-15); BuildKit-daemon requirement.
- [S147] P — github.com/devcontainers/cli — maintained; Docker-based.
- [S148] P — mise.jdx.dev — pinned per-project toolchains.
- [S149] P — docs.astral.sh/uv/guides/projects/ — env verified before every `uv run`.
- [S150] P — caddyserver.com/docs/api — zero-downtime runtime route add/remove; @id addressing.
- [S151] P — vite.dev/config/server-options + github.com/vitejs/vite/discussions/6473 — HMR WebSocket proxy trap; allowedHosts.
- [S152] P/B — github.com/vitejs/vite/discussions/15842 + medium.com/@tushar3145/… — path-prefix routing breaks apps.
- [S153] P — tailscale.com/docs/reference/tailscale-cli/serve + tailscale.com/blog/services-ga — per-port serve; Services GA (Feb 2026) MagicDNS names + virtual IPs.
- [S154] P — man7.org …/systemd-socket-proxyd.8.html — exit-idle-time + StopWhenUnneeded on-demand/idle-stop.
- [S155] P — manpages.debian.org/testing/bubblewrap/bwrap.1.en.html — --overlay-src/--tmp-overlay unpersisted writes (kernel ≥4.0).
- [S156] P/R — npmjs.com/package/npkill + github.com/orgs/pnpm/discussions/4413 — reclamation tooling; store prune.
- [S157] P — github.com/spencerpauly/awesome-cursor-skills/blob/main/resources/comparing-branches-visually/SKILL.md — two-branch dual-server comparison recipe (absence of shipped product — flagged).
- [S158] P — vercel.com/docs/deployments/generated-urls — immutable commit URL + mutable branch URL.
- [S159] P — github.com/tsl0922/ttyd — terminal-over-web; --once; proxy-friendly.
- [S160] P — github.com/Xpra-org/xpra-html5 — desktop-GUI-in-browser (heavy; not v1).

**Encrypted snapshots (Angle 5 — citations re-established 2026-07-17)**
- [S161] P — github.com/FiloSottile/age — BSD-3; v1.3.1 (2025-12-28); recipients vs passphrase modes; no sender authentication (README-level; original exact sourcing lost — treated as verified-in-original-pass) **[re-established]**.
- [S162] P — age-encryption.org/v1 → github.com/C2SP/C2SP/blob/main/age.md — "An scrypt stanza, if present, MUST be the only stanza in the header"; MLKEM768-X25519 (X-Wing) hybrid PQ recipients (precision note) **[re-established]**.
- [S163] P — github.com/FiloSottile/typage — v0.3.0 (2025-12-29) "with post-quantum hybrid recipients" **[re-established]**.
- [S164] P — docs.github.com …/git-lfs (billing) — LFS Free = 10 GiB storage + 10 GiB bandwidth, metered; blocked on overage without payment method **[re-established]**.
- [S165] P — docs.github.com …/removing-sensitive-data-from-a-repository — unreachable objects persist (cached views, PR refs); Support-run GC required for true deletion **[re-established]**.
- [S166] P — docs.github.com …/about-releases — release assets <2 GiB/file, 1000/release, no total/bandwidth limit **[re-established]**.
- [S167] P — restic.readthedocs.io/en/stable/100_references.html — encrypted pack format; prune rewrites packs (restic-in-git rejection basis) **[re-established]**.
- [S168] P — github.com/facebook/zstd — dual BSD/GPLv2; v1.5.7 **[re-established]**.
- [S169] distilled, single-provenance — git-native transparent encryption (git-crypt class) wrong-shape argument: structure/filename/cadence leakage + delta defeat; conclusion verified in original pass, original citation not reconstructable.

**Prior campaign reports cited:** Research/03 (approval cards, frozen ACs), Research/04 (verification ladder, two-axis), Research/08 (§4.5 effect journal, §4.7 durable set + restore drill, §4.9 claims mechanics, R08-OQ5), Research/10 (§3.4 credential broker, safe-outputs pattern, C2 confinement), Research/14 (§6.2 N15/N19 pre-registered deference), GATE-1 decision record (ratified architecture + riders).
