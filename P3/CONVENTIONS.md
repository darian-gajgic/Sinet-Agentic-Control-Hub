# P3 build conventions

Derived from the frozen spec (S01, S16; build order S19.5) at packet P3-B0-1, 2026-07-19. Every packet session follows this file; on any conflict the spec wins and the conflict is reported to the coordinator. The file is append-mostly: later packets add the conventions their sections ground (each addition cites its spec section) and never silently rewrite earlier ones.

## 1. Repository layout (S01.5, S16.2, S19.5)

```
cmd/sinet/            the one release binary — mode dispatch only
internal/             all platform packages (module-private; nothing is importable from outside)
tools/                repo/CI tooling (e.g. tools/lockgate) — never part of the release artifact
.github/workflows/    CI (S01.11)
components.lock       adoption manifest (S16.2) — repo root
P3/                   build coordination (STATE.md is coordinator-owned; packets never edit it)
Spec/  Research/  Docs/   read-only for build sessions
```

- **One binary.** The release artifact is a single static Go binary, `sinet`; `sinet-control`, `sinet-broker`, and `sinet-portpool` are the same binary invoked in dedicated modes (S01.5 multi-call posture). Never add a second `cmd/` — a new daemon is a new mode plus a unit file, not a new artifact.
- Daemon modes live in the mode table in `internal/cli`; modes not yet built are reserved names that fail with "not implemented in this build" (exit 1) so invocation shape and future `ExecStart=` lines are stable from the first build.
- SPA assets embed into this same binary via `go:embed` at B6 (S01.5); nothing about that lands earlier.
- No `pkg/` directory: this module is never imported, so everything beyond `cmd/` and `tools/` is `internal/`.

## 2. Go conventions (S01.5)

- **Module path:** `github.com/dariannixda-eng/Sinet-Agentic-Control-Hub` (the real repo; private, never fetched by proxy — the uppercase path is accepted).
- **Toolchain pin:** the `go` directive in `go.mod` (`1.26.5`) is the single source of truth; the `Go toolchain` entry in `components.lock` mirrors it and `tools/lockgate` fails on any mismatch. Toolchain bumps are adoption-rail events (dated lock edit + go.mod change together).
- **stdlib-first.** Any third-party Go module is an S16 adoption: S16.4 checklist plus a `components.lock` entry (with the module path in the entry's `modules` list) land in the same commit as the first import. CI enforces this (§4).
- **Formatting/linting:** `gofmt`-clean and `go vet`-clean, both enforced in CI. No additional linters at B0 (a linter is a dependency; adopt it through the rail or not at all).
- **Package naming:** short, lowercase, no underscores. The S01.1 module seams map to fixed package names, created by their owning packets (B0-2+): `internal/storage`, `internal/scheduler`, `internal/gates`, `internal/ledger`, `internal/eventlog`, `internal/adapters`. Names are fixed here so packets converge; do not invent variants.
- **Doc comments:** standard Go style on exported identifiers; cite spec sections (e.g. "Spec S01.5") where they genuinely clarify a design constraint. No research narration, no changelog prose in code.
- **⚙ settings discipline (S01.10):** no ⚙ value is ever a hardcoded constant. Consumers read the settings registry (built in P3-B0-2) by dotted key. Until the registry exists, leave a named seam: `// TODO(settings): ⚙ <key> per <section>` — never an inline number.
- **Errors:** wrap with `%w`; introduce sentinel/typed errors only where a caller branches on them.
- Open (deferred to owning packets): logging/structured-log shape → B0-3 (journald posture, S01.11); SQLite driver + storage conventions → B0-2 (S16.3 driver row); systemd unit generation → B0-3 (generated, not installed — host changes wait for the B0 gate); frontend conventions → B6 (FC-v1).

## 3. Test conventions

- stdlib `testing` only — assertion/mocking libraries are dependencies and none are adopted.
- Tests are colocated `_test.go` files; table-driven where natural; test binaries and fixtures never write outside `t.TempDir()`.
- `go build ./...` and `go test ./...` green is a precondition for every commit — packets never commit red.
- The real `components.lock` and workflow files are validated inside the test suite (`internal/lockfile`) as well as by the standalone gate, so plain `go test ./...` trips on manifest breakage.
- Bring-up measurements (S19.6) are not Go tests: they are coordinator/operator-run probes recording to `P3/measurements/`.

## 4. Adoption rail mechanics (S16, S01.11)

- **Serialization (resolves S16.2 TBD-P3(lock file serialization), P3 choice dated 2026-07-19):** `components.lock` is strict, pretty-printed JSON — one field per line, so review diffs are clean (S16.2's only constraint), and the Go stdlib parses it, so the gate itself requires zero adoptions. Unknown fields are rejected (`internal/lockfile`); a format migration, if ever wanted, is an ordinary dated lock edit.
- **Fields:** the nine S16.2 normative fields, with `license` structured as `{spdx, scope, checked}` (path scope + check date, P-T16-2) and `role` as `{summary, section}`. Documented extension fields, additive only: `modules` (Go module paths an entry covers — the mechanical hook for the bundled-dependency rule) and `notes`. An npm-side equivalent of `modules` is defined at B6, not before.
- **Entry materialization:** the S16.3 table is the binding v0 commitment; a row's lock entry materializes when its component is first consumed (running unit, bundled dependency, toolchain, CI mechanism). TBD-P3 pins resolve per their row text (Go: "pin at scaffold" → resolved at P3-B0-1) or at the first quarterly pass (S16.7, S19.5). Anything outside S16.3 enters only through the S16.4 onboarding checklist; checklist #10 (operator approval) means packet sessions propose and phase gates approve.
- **CI actions are on the rail.** GitHub Actions workflow actions are constituents of the ratified S01.11 CI mechanism, recorded as `kind: toolchain` lock entries and pinned to full 40-hex commit SHAs (the human tag goes in `notes`), so "no new dependency without a lock entry" holds everywhere. Runner images are pinned to exact stable labels (`ubuntu-24.04`); `-latest` and preview labels are floating and never used.
- **Pin discipline:** exact versions only — never `latest`, `*`, or ranges (`^ ~ > < =` are rejected by validation). Where vendored content exists, the pin carries a content hash and upstream LICENSE/NOTICE files are kept in-tree (S16.2); nothing is vendored yet.
- **The gate:** `go run ./tools/lockgate` — locally before commit and as the CI `lock-gate` step. It validates the manifest fields (S16.2), asserts every `go.mod` require is covered by an entry's `modules`, asserts the `go` directive matches the Go-toolchain pin, and asserts every workflow `uses:` is SHA-pinned and entry-covered. The workflow scanner is a line-level tripwire for this repo's own files, not a general YAML parser — keep workflow `uses:` lines simple. The running-unit half of the S16.2 CI rule activates when unit generation lands (B0-3+).
- **Removal discipline:** an entry leaves the lock only by executing its funeral plan, as a dated lock edit with the reason — never silent deletion (S16.2).

## 5. Commit & process conventions (P3)

- **Subject:** `P3-<phase>-<n>: <summary> (<spec sections>)` — e.g. `P3-B0-1: repo scaffold + adoption rail + CONVENTIONS (S01, S16, S19.5)`. Body optional; trailer on every packet commit: `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Stage files explicitly — never `git add -A`. Packet sessions never push (the coordinator pushes after validation), never force-push, never edit `Docs/`, `Spec/`, `Research/`, or `P3/STATE.md`, and never read or stage `*-api-key.txt`.
- **CI:** `.github/workflows/ci.yml` runs gofmt, vet, build, test, lock-gate on push to `main` and on PRs. The tag-triggered release pipeline (artifact + `SHA256SUMS` + signed tag, S01.11) is a later packet.
