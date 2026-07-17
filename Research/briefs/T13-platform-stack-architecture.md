# T13 — Platform stack & process architecture

**Wave:** C · **Depth:** LIGHT (well-trodden territory — currency check + fit reasoning, not a frontier survey; single focused session, still fully cited) · **Report slug:** `platform-stack-architecture`

## Scope
S3.1–S3.10 (surfaces: mission control, approval inbox, board, workforce map, chat, review UIs, notifications, live-by-default, multi-user-aware — the *stack* that carries them; UX details stay with the spec), §13 (settings surface), 15.6 (multi-user data model), anti-monolith lesson (Nexus: 11.5k-line server.py — no seams), D1 (LAN/Tailscale service).

## Why this gates the spec
Every other topic assumes a platform shell: processes, storage, API, real-time updates, auth. The Nexus monolith is the named anti-pattern; the spec needs a decomposition with seams a solo maintainer can hold. This is deliberately LIGHT: mature engineering territory where the risk is picking stale fashion, not missing frontier research.

## Core question
What application stack and process decomposition best fits a one-maintainer, self-hosted, multi-user agent platform on a single Ubuntu laptop in mid-2026 — API-first backend, real-time web UI usable from phones, SQLite-class storage, LAN/Tailscale-only exposure — optimizing for bus-factor-1 maintainability over a decade, not for scale?

## Sub-questions
1. Process decomposition: control plane / execution lanes / web surface as separate processes (supervision via systemd units?) — what boundaries prevent the monolith failure while staying operable by one person; IPC choices (HTTP/unix sockets/queues via SQLite).
2. Backend: current mainstream choices for typed, API-first services a solo dev maintains for years (Python's current best practice vs Go vs TypeScript) — judged on longevity, typing, ecosystem fit with agent tooling, and the operator's enterprise-architect background.
3. Frontend approach for live dashboards + phone approvals: SPA framework vs server-driven (HTMX-class) trade-offs at bus-factor-1; PWA for phone approvals — web-push notification constraints over Tailscale (S3.7: notified → glance → decide on a phone), current push options for self-hosted apps without cloud relays (or honest fallbacks).
4. Real-time: SSE vs WebSockets for live boards/streams (S3.9) — current consensus for this scale.
5. Storage: SQLite as system-of-record (WAL, backup via litestream-class tools, single-writer discipline) — when it stops sufficing; whether anything in the feature list forces more (likely not at household scale — verify against T07/T11 schema loads).
6. AuthN/AuthZ for a household over LAN/Tailscale: Tailscale identity integration vs passkeys vs sessions — friction vs security for non-technical members; per-user identity threading through every query (15.6).
7. Settings machinery (13.4): schema-driven settings with validation — patterns that keep every-single-setting organized without bespoke UI per option.
8. Packaging/deploy on the host: uv/venv vs containers for the platform itself (not sandboxes — that's T09); CI on GitHub (D9) building verified releases; config/data directory layout; log management.
9. What existing self-hosted platforms of similar shape run (reference points from T16's sweep if committed) — copy their boring, proven choices where possible.

## Constraints that bind this topic
D1 (one host, LAN/Tailscale exposure only), 15.6 (owner on every record from day one), S3.9 (live by default), Operating reality (survives sleep/wake; unattended while up), anti-monolith (decomposition is a requirement, not taste).

## Harvest-map items to verdict
A1 (Archon dashboard components as PORT/ADAPT candidates — stack compatibility check), A5 (7-table schema sanity reference), anti-harvest "server.py monolith" row (the lesson this topic encodes).

## Sources to prioritize
Current framework/stack state-of-practice posts (2025–2026), SQLite-in-production guidance, Tailscale docs on serving apps + identity (primary), web-push/PWA current constraints (primary browser-vendor docs).

## Decisions this feeds
G3: stack choice, process decomposition, auth model. Spec: platform shell section, deployment/ops.

## G2 addendum (2026-07-17) — settled inputs, cite don't re-research

G2 is CLOSED (`Research/decisions/GATE-2-substrate-and-adoption.md`); reports 01–14 plus both gate files are committed inputs. The shell you are decomposing already contains ratified organs — design around them, never reopen them: SQLite-WAL `platform.db` with a sole-writer control plane is the system of record (report 08 §4.1); the D7 event log is the observability substrate and live updates are one SSE endpoint with `event_seq` cursor resume (report 12 §4.1/4.3 — your SQ4 starts from this settled answer and checks fit, not fashion); the sandbox stack (report 10 §4), credential broker, and the preview plumbing (report 13 §4.8: Caddy admin-API routes, `tailscale serve`, systemd-socket-proxyd idle-stop, port-pool daemon) are given services the decomposition must house. New binding rules for this topic: every tool adopted in this layer meets P-T11-4 criteria (permissive license, pinned, the record always lives in Sinet's DB — tools are runners/projectors, never stores); every ADOPT carries the P-T16-1 component-onboarding checklist (pin + replacement path + abandonment criteria — give this checklist a home in your stack section); license checks are path-scoped per P-T16-2. The frontend question consumes report 14 §3.3's shortlist (dnd-kit, react-diff-view, monaco-diff, assistant-ui) as candidates only — the binding pick happens at the spec-phase frontend workshop. The 13.4 settings machinery (SQ7) must implement the G1 settings-not-constants rider (operator-editable, audit-trailed, ceiling-bounded auto-adjustment). Report 14 §2.2's control-plane mortality (~14-month half-life) is your anti-monolith evidence: small owned core + adopted organs is the survivable shape.
