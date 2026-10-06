# Sinet Agentic Control Hub

Sinet is a self-hosted platform for supervised AI work. A request passes through intake, a written specification with numbered acceptance criteria, and a plan the requester approves. A coding agent then executes it inside a sandbox, an independent check verifies the result, and a human reviews it. Accepted work lands as an attributed, signed git commit. Sinet drives existing agent CLIs (Claude Code, opencode, Kimi Code) through one adapter contract instead of modifying them, and it records every model call, so each run ends with a cost receipt.

It replaces [Nexus](https://github.com/darian-gajgic/Nexus-Agentic-Coding-Setup), which missed its own benchmark. The [post-mortem](Docs/nexus-post-mortem.md) explains what was kept and what changed.

## Status

Active development, one maintainer. The architecture spec was frozen on 2026-07-19 (git tag `spec-v1`), and all seven build phases it defines are implemented. Software development is the first supported task domain. The platform is operated single-user, but every record already carries its owner.

Not done yet:

- **Benchmark gate.** The protocol is pre-registered in a signed commit ([`Spec/benchmark-preregistration-v1.md`](Spec/benchmark-preregistration-v1.md)). Sinet has not yet been measured against using a frontier model directly, and new domains wait on that result.
- **Live app previews.** Preview environments are prepared and composed through the sandbox. Serving them to the browser is not wired up.

## What works today

- **Intake and planning.** Triage, a clarifying interview scaled to the stakes of the task, a specification with numbered acceptance criteria, and a plan that must be approved before anything runs.
- **Execution through unmodified engines.** Adapters for the Claude Code CLI, opencode (Z.AI GLM and local models) and the Kimi Code CLI. Each engine is pinned to a version and covered by a conformance suite.
- **Sandboxing.** Runs are confined with bubblewrap, user and network namespaces, and seccomp. Credentials live in a separate broker process and never enter the sandbox.
- **Verification.** Every deliverable goes through deterministic checks, domain checks and a judge pass that scores spec compliance and outcome sanity separately. Findings feed a retry loop.
- **Review and accept.** Deliverables are immutable revisions with diffs and anchored comments. Accepting one produces exactly one signed commit on the project's protected branch.
- **Scheduling and metering.** A SQLite-backed scheduler with priorities, budgets and per-lane concurrency. A provider rate limit parks the run until it can resume. Every paid call is recorded.
- **Durable state and recovery.** An append-only event log and a run state machine in SQLite. After a crash, restart or suspend, a recovery pass reattaches, harvests or flags each unfinished run.
- **Memory.** Scoped knowledge with provenance. Agents can search it; only an authenticated human can write to it.
- **Web UI.** A React and TypeScript app embedded in the binary: task board, chat, approval inbox, deliverable review, history and settings, plus web push notifications.
- **Operations.** Generated systemd units, scheduled state snapshots with a restore drill, and self-health watchdogs.

## Architecture

```
browser --> tailscale serve --> Caddy --> sinet control (127.0.0.1 only)
                                            |  HTTP API + one SSE stream
                                            |  pipeline, scheduler, metering
                                            |  platform.db (SQLite, WAL)
                                            |
                   sinet broker <-----------+-----------> per-run sandbox
              (credentials, signing,                   (bwrap + engine CLI)
               pushes)
```

- One static Go binary, `sinet`, runs in separate modes as systemd services: `control` (API, pipeline, scheduler, embedded UI), `broker` (secrets, commit signing, pushes) and `portpool` (preview ports).
- Every surface, including the web UI and chat, is a client of the same HTTP API. Nothing renders from private access.
- Engines run as child processes inside the sandbox. All platform logic stays on the adapter side.

| Path | Contents |
|---|---|
| `cmd/sinet`, `internal/` | Go backend, about 130k lines in 49 packages |
| `web/` | React 19 + TypeScript UI (Vite, Tailwind) |
| `Spec/` | The frozen architecture spec and the benchmark pre-registration |
| `Research/`, `Docs/` | The 18 research reports and the source documents behind the spec |
| `P3/` | Build records: state, task briefs, review reports, gate decisions, measurements, UI screenshots |
| `components.lock` | Every adopted dependency, pinned, with its licence and role |

## Build and run

Requirements: Linux, Go 1.26.5 (see `go.mod`), Node.js 22.

```bash
(cd web && npm ci --ignore-scripts && npm run build)   # UI assets, embedded by go build
go build -o sinet ./cmd/sinet
./sinet version
./sinet control --state-dir ./.state --http-addr 127.0.0.1:8482
# then open http://127.0.0.1:8482
```

Started outside systemd, `sinet control` uses a development posture: default settings, a development identity, and engines that run unconfined. Running real tasks needs at least one supported engine CLI installed and signed in. `./sinet units` renders the systemd unit set for a production install.

## Tests

- **Go:** 585 test files with 2,745 test functions across 48 packages.
- **Web:** 39 Vitest files with 860 tests.
- **Build harness:** shell tests in `P3/run/test-*.sh`.

CI (`.github/workflows/ci.yml`) runs the web typecheck, tests and build, then `gofmt`, `go vet`, `go build`, `go test ./...` and a lock gate (`tools/lockgate`). The lock gate fails if a Go module, npm package or GitHub Action is not pinned and listed in `components.lock`. Tests that need host capabilities (sandbox composition, a GPU) skip with a stated reason when the host lacks them. Tests that call paid model APIs only run when enabled by an environment variable.

```bash
go test ./...
(cd web && npm test)
go run ./tools/lockgate
```

## How this is built

The code is written by AI coding agents under a fixed process. The maintainer sets direction, answers decision gates and reviews the results. The repository records each step:

1. **Research, then a frozen spec.** Eighteen research reports led to a spec that was frozen and tagged before implementation began. Later changes are dated amendments that the maintainer approves.
2. **Failing tests first.** Each unit of work starts with a brief that traces every requirement to a spec section. The same stage writes acceptance tests from the spec and commits them failing, before any implementation exists (for example `374015b`). A second agent spot-checks the brief.
3. **Implementation.** A fresh agent implements the packet in its own git worktree until the tests pass.
4. **Independent review.** A separate agent, on a model at least as strong as the implementer, evaluates the diff against the spec. It looks for weakened assertions, silent skips and invented behaviour. A failing verdict returns numbered findings, and each one must be reproduced before it is fixed (for example `14ec042`).
5. **Human gates.** Decisions that belong to a person are written to `P3/gates/` and answered there.

The stage prompts are in `P3/prompts/` and the coordinator runbook is in `.claude/skills/p3-implementation/SKILL.md`.
