# T12 — Deliverables, review & git integration

**Wave:** B2 · **Depth:** FULL (medium breadth — mechanics over UI polish) · **Report slug:** `deliverables-review-git`

## Scope
§6 (everything arrives as a reviewable change; comment-anchored review; accept = one action → attributed commit; instant try-out), S1.8–S1.9 (native review, disposable preview), S1.11 (parallel same-project coordination — artifact level), S3.5 (review surfaces — mechanics here, stack in T13), D9 (per-project git, per-user GitHub remotes), 11.3 (encrypted platform-state snapshots to GitHub), 5.5 (feedback anchored to exact places, carried into retries).

## Why this gates the spec
The deliverable pipeline is the platform's product surface: everything a user ever sees arrives through it, and acceptance is the moment work becomes real (attributed commits, D9). Diff-first review across formats and the accepted-change flow shape the schema (artifacts, revisions, comments) and the git topology.

## Core question
How should Sinet produce, diff, review-with-anchored-comments, and accept deliverables of every type (code, documents, PDFs, images) into per-project git repositories with correct per-user attribution in mid-2026 — and how should platform state snapshot to GitHub client-side-encrypted, with a tested restore path?

## Sub-questions
1. Diff/compare mechanics per format: code/text (solved — pick idioms), documents (structured-text diffing, office formats?), PDFs (extracted-text diffs — current tooling quality), images (side-by-side/overlay) — what's genuinely available vs custom work.
2. Anchored comments that feed retries (6.2, 5.5): representing position-anchored feedback across revisions (anchor drift when content changes); numbered-findings handoff into the next attempt (harvest N15's findings→comments→retry loop — validate the pattern, the Review-v2 port itself is post-gate).
3. Revision lineage (6.1): round-over-round comparison models; deliverable versioning schema.
4. Accept → commit mechanics (6.3, D9): committing as the accepting user (author/committer identity, per-user credentials from their store, signing?), commit hygiene for agent work, push-on-accept; multiple users collaborating on one owner's repo (invited-collaborator flows).
5. Worktree/branch strategy for agent work: branch-per-pipeline (harvest N9's reasoning: stages share a branch; parallel pipelines isolated) vs per-run isolation (A7) vs current best practice for parallel agent writers on one repo; snapshot-commit safety nets (never lose uncommitted agent work).
6. Artifact-collision coordination (S1.11): detecting same-artifact writes at plan time (claim registries), sequencing vs explicit branching, accept-time merge surfacing — prior art in multi-agent coding platforms.
7. Disposable try-out (6.4, S1.9): one-click launch of built apps in throwaway environments (auto-install, port handling, auto-stop), whole-project before-vs-after previews from branches — current tooling (bridge to T09 for confinement of previews).
8. Platform-state snapshots (11.3): client-side-encrypted archives into a git repo — current encryption tooling fit (age-class file encryption, restic-class repos-in-git?, git-native encryption limits), bounded snapshot history in a remote, text-first exports, the 100 MB blob reality, and restore-testing practice (backup that isn't restore-tested doesn't exist).
9. Document/non-code project storage (D9's "document store"): git for documents vs alternatives that still satisfy attribution + history — keep it simple for one maintainer.

## Constraints that bind this topic
D9 (fixed topology: per-user GitHub remotes, operator-owned snapshot repo), D7 (accept is the gate that releases the outward push — proposals until then), D2 (pushes authenticate as the accepting user from their credential store), 11.3 (secrets never leave the host unencrypted; traces excluded from snapshots).

## Harvest-map items to verdict
N9 (worktree.py branch-per-pipeline design), A7 (Archon worktree manager — cleaner details?), N15 (Review v2 — pattern validation now, port later), N19 (app_runner + time-travel preview — nice-to-have check).

## Sources to prioritize
Git tooling docs (worktrees, committer identity, signing) — current; agent-platform writeups on parallel-work isolation; document/PDF diff tooling status; encryption-to-git tooling docs + independent reviews; GitHub limits documentation (primary).

## Decisions this feeds
G2: deliverable schema (artifacts/revisions/comments), git topology + worktree strategy, snapshot design. Spec: deliverable pipeline, accept flow, backup/restore section.
