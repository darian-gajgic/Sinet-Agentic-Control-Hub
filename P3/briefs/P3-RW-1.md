# P3-RW-1 brief — intake project pin (the Projects-tab door)

**Packet origin:** `P3/design/product-map.md` §5 (operator-approved 2026-08-05) — the one backend packet the rework queues. §7 build-order step 2 (give-work journey) consumes it.
**Spec sections read in full at grounding (2026-08-05):** S06.1–S06.10, S13.7 (+S13.8/S13.9 skim), S15.2–S15.3, S00.9. Drafts canonical.
**Code read at grounding:** `internal/stage/surface.go`, `internal/intake/{intake,pipeline,state}.go`, `internal/project/{registry,project,onboard}.go`, `internal/shell/project_seams.go`, tests `internal/project/registry_test.go`, `internal/stage/{e2e,round1_e2e}_test.go`, `internal/shell/project_seams_test.go`, `internal/api/intake_handlers.go`.

## BINDING carried facts (verified at grounding — do not relitigate)

- The registry match at intake is `project.Store.MatchForIntake` (`internal/project/registry.go:300`): a deterministic name-token scan of title+text over ACTIVE entries visible to the requester (`visibleTo` = owner OR invited member, line 327; `Entry.Active()`), longest name wins, ok=false otherwise.
- The production `intake.Registry` seam is `registrySeam.Match` in `internal/shell/project_seams.go:57` — it builds `project.MatchHint{UserID, Title, Text}` from the `intake.Request` and projects the entry via `toRegistrySlice` (ProjectID, `<id>@capture-v<n>` Ref, conventions, commands, danger zones). The pipeline consumes it at `Pipeline.Start` (`internal/intake/pipeline.go:150-157`) — note: **seam errors are currently swallowed** (`err == nil && ok`), a no-match and an error both degrade to no slice.
- `Surface.Submit` (`internal/stage/surface.go:94`) decodes `submitBody{Title, Text, Inputs}` and builds `intake.Request{UserID, Title, Text, Inputs}`; `userID` arrives from the authenticated session (`internal/api/intake_handlers.go` `handleIntakeSubmit`: `id.UserID`). Unknown JSON body fields are silently ignored today.
- `intake.State` marshals its `Req Request` whole onto every `intake.state` event (`state.go:95`), so a new Request field IS durably recorded with no schema change — **the additive precedent is the `Inputs` field, commit `56b93c4` (P3-B6-7), recorded in CONVENTIONS §44**: `submitBody` + `intake.Request` gained `inputs` additively; a body without it submits exactly as before.
- Downstream run→project resolution reads the DURABLE intake match `st.Registry.Project` (CONVENTIONS §23 F3/F4; `projectSeams.projectForTask`) — never a re-match. The pin therefore propagates to workspace/fingerprint/base-content with zero downstream change.
- S15.2: "evolution is additive-first"; "full endpoint schemas are P3 work within these bounds"; "the browser is a display, never an authority — authorization and visibility are enforced server-side". S06.2 step 2 says the "matched" entry is injected — it does not pin token-scanning as the only match mechanism; S13.7's "'in the shop backend' needs no path-explaining" is an example, not an exhaustive contract. **Hence NO S00.9 amendment is needed: no ratified sentence changes.** No spec conflict was found.

## 1. Numbered requirements

- **R1** — `submitBody` (`internal/stage/surface.go`) gains an optional `project` field (string, `omitempty`). Additive: a body without it submits byte-identically to today. [S15.2 additive-first; precedent `56b93c4`; product map §5]
- **R2** — `intake.Request` gains `Project string `json:"project,omitempty"``; `Submit` passes it through. Because `State` marshals `Req` whole, the pin lands on every `intake.state` event and thus the durable intake record — no migration, no schema change. [CONVENTIONS §14 event-sourced state; S06.1 Stage-0 record; precedent §44]
- **R3** — When `project` names an ACTIVE registry entry the requester owns or belongs to, the registry seam resolves that entry DIRECTLY into the `RegistrySlice` — the request text is not consulted for the pin. [product map §5 "pins the RegistrySlice directly"; S06.2 step 2; S13.7 "the registry feeds intake resolution"]
- **R4** — **The security edge:** owner/member + ACTIVE validation is enforced server-side at pin resolution, using the SAME visibility predicate `MatchForIntake` embodies (owner OR member; `Active()`), kept in ONE place — never a second hand-rolled copy of the predicate in another package. [S15.2 authority rule; S13.7; CONVENTIONS §23 F3 discipline; placement = OQ3]
- **R5** — Unpinned submissions (no `project` field) keep the deterministic name-token scan exactly as today. [product map §5 "the text match stays for unpinned submissions"; S06.2]
- **R6** — A pinned slice is the SAME projection a matched slice gets (`toRegistrySlice`: ProjectID, capture-versioned Ref, conventions, commands, danger zones), so danger zones feed the stakes floor, registry slots resolve interview slots, and approval pins ledger-§2 constraints identically. [S06.2; S06.9; S05.1; CONVENTIONS §14]
- **R7** — Nothing downstream changes: workspace creation, freshness fingerprint, base content all read the durable `st.Registry.Project` as today. [CONVENTIONS §23 F3/F4; S02.6/S02.10]
- **R8** — NO new ⚙ key; `internal/settings/index.go` stays byte-unchanged (118 keys / 33 domains — the §14/§23 posture). State this in the executor report with the tally as proof. [S18; CONVENTIONS §6]
- **R9** — Import walls hold: `internal/intake` never imports `internal/project` (seam only, CONVENTIONS §14); `internal/project` imports storage+eventlog only (AST-pinned `conformance_test.go`, §23). Pin resolution crosses only the existing `intake.Registry` seam wired at the composition root.
- **R10** — Go-only paths. `web/` is untouched (a frontend builder works there concurrently); the SPA consumes the field at build-order step 2, not in this packet. [product map §5/§7; packet constraint]

## 2. Seams to respect (and stubs)

- The packet RIDES the existing `intake.Registry` seam (`Match(ctx, req)` — the full `Request` already flows through it, so R2 needs **no interface change**). No new seam, no stub: every phase this packet touches is already built.
- If OQ1 resolves to "reject loudly", the error surfaces through the existing `mapIntakeErr` shape (`internal/stage/surface.go:39`) — a new branch-on sentinel in `internal/intake` (CONVENTIONS §2 errors rule). Note the swallow at `pipeline.go:151`: a loud disposition must route the pin-refusal around it (surface pre-check before `Start`, or a distinguishable seam return) — coordinator picks via OQ1; do not build both.
- Frontend consumption, and any Projects-tab UI, are NOT this packet.

## 3. ⚙ settings consumed (by registry name)

**None — explicitly.** The packet introduces no new ⚙ key and consumes no existing key it does not already transit. The settings index tally (118/33) must be unchanged after landing.

## 4. Files expected to change

| File | Change |
|---|---|
| `internal/intake/intake.go` | `Request.Project` (R2) |
| `internal/stage/surface.go` | `submitBody.Project` + pass-through (R1); error mapping only if OQ1 = loud |
| `internal/shell/project_seams.go` | `registrySeam.Match` honors the pin (R3/R4) |
| `internal/project/registry.go` | validated pin lookup (R4; shape per OQ3) |
| `internal/project/registry_test.go` | store-level pin validation tests incl. the property-based visibility sweep (§8) |
| `internal/shell/project_seams_test.go` | production-seam pin test (typed `Request.Project` — could not compile at grounding) |
| `internal/stage/round1_e2e_test.go` | `registryOver` (the seam's inlined test double, line ~180) mirrors the pin pass-through |
| `internal/stage/projectpin_e2e_test.go` | committed RED at grounding — flips green (§7 item 1) |
| `internal/intake/pipeline.go` | only if OQ1 = loud (the swallow at line 151) |

## 5. Adopted components touched

**None.** No new dependency, `components.lock` and `go.mod`/`go.sum` unchanged.

## 6. CONVENTIONS constraints binding this packet

- **§2**: stdlib-first; gofmt/vet-clean; sentinel errors only where callers branch; no hardcoded ⚙.
- **§3**: stdlib `testing` only; colocated `_test.go`; `t.TempDir()` discipline. **Deviation, coordinator-sanctioned:** the grounding commit leaves `go test ./internal/stage` red on exactly `TestProjectPinAtSubmit/{owner_pin_without_name_in_text,member_pin}` until the executor lands (`go build ./...` stays green — the tests compile). Every other package stays green.
- **§5**: subject `P3-RW-1: … (S-refs)`; explicit pathspec staging, never `git add -A`; packet sessions never push, never touch `Docs/`, `Spec/`, `Research/`, `P3/STATE.md`.
- **§14**: intake imports storage/eventlog/run/ledger only; seams degrade as the spec directs; event-sourced state (no intake table).
- **§23**: project package import walls (AST-pinned); seams wired at the composition root only; F3 durable-match rule; no new ⚙ (§23's own posture extends to this packet).
- Process: the concurrent frontend builder owns `web/` — do not touch it; retry (never break) a locked git index.

## 7. Acceptance checklist (each item concretely checkable)

1. **Committed battery green:** `go test ./internal/stage -run TestProjectPinAtSubmit` fully green — AC-1..AC-7 (see §8), including the `registryOver` mirror in `round1_e2e_test.go`.
2. **Production seam proven:** a shell-level test drives `registrySeam.Match` with `Request{Project: …}` directly: owner pins, member pins, non-member/pending/unknown never pin.
3. **Store-level predicate proven:** the owner/member × ACTIVE sweep at `internal/project` level (the property-based candidate, §8) passes; the predicate exists in exactly one package (grep-checkable: no second `Members`-loop copy).
4. **Slice parity:** a pinned submission's slice carries the same conventions/commands/danger-zones/Ref projection as a text-matched one (assert on `Ref == "<id>@capture-v<n>"` and a danger-zone element).
5. **Durable record:** the born task's latest `intake.state` event JSON contains the submitted `project` under `request` (read the event row, not just `LoadState`).
6. **Additive proof:** a body without `project` behaves byte-identically — AC-7 plus the full existing intake/stage suites green, unmodified except `registryOver`.
7. **OQ1 disposition implemented exactly as ratified** — with its error mapping (and an api-level status assertion) if loud; with a fallback assertion if silent.
8. **Hygiene:** `gofmt` clean, `go vet ./...` clean, `go build ./...` green, full `go test ./...` green after landing; settings index 118/33 byte-unchanged; `components.lock` unchanged; conformance import-wall tests green; nothing under `web/`, `Spec/`, `Docs/`, `Research/`, `P3/STATE.md`.

## 8. Acceptance tests — committed at grounding vs specified for the executor

**Committed RED in this grounding commit — `internal/stage/projectpin_e2e_test.go` (`TestProjectPinAtSubmit`, evidence: run of 2026-08-05, two subtests fail on `registry = <nil>`):**

| Subtest | Status today | Asserts |
|---|---|---|
| `owner_pin_without_name_in_text` | **RED** | AC-1/AC-2: Submit body `{"title","text","project":"shop"}` as owner alice, text never names the project → `LoadState(task).Registry.Project == "shop"` |
| `member_pin` | **RED** | AC-3: same as member bob |
| `cross_user_pin_never_pins` | guard (green, OQ1-proof) | AC-4: alice pinning dana's ACTIVE entry → either Submit errors OR born state is not pinned to it |
| `pending_entry_never_pins` | guard (green, OQ1-proof) | AC-5: a captured-but-PENDING entry never pins |
| `unknown_project_never_pins` | guard (green, OQ1-proof) | AC-6: an unknown id never yields a slice |
| `unpinned_text_match_unchanged` | guard (green, regression pin) | AC-7: no `project` field + text naming "shop backend" → text match still pins |

The battery could be committed compiling because `Submit` takes a raw JSON body (today's decoder ignores the unknown field). Fixture: entries registered at the store layer (`Register`/`Capture`/`Activate` — `shop` owner alice member bob name "shop backend"; `attic` owner dana; `draft` pending).

**Specified here, NOT committable at grounding (they reference the typed field / methods that do not exist — would break `go build`):**

- `internal/shell/project_seams_test.go` — `TestRegistrySeamHonorsSubmittedPin`: build a store via `seamDB`+`project.New`, `Onboard`+`Approve` an entry, call `registrySeam{proj}.Match(ctx, intake.Request{UserID: owner, Text: "no name here", Project: id})` → slice with full `toRegistrySlice` projection; non-member/pending/unknown → not pinned (exact not-pinned shape per OQ1).
- `internal/project/registry_test.go` — the store-level validation sweep (shape per OQ3, e.g. on `MatchHint.Project` or the sibling lookup): table over {owner, member, stranger} × {active, pending} × {known, unknown} asserting resolve-iff (visible AND active). **Property-based candidate (spec-stated invariant):** for ALL requester/entry combinations, a pin NEVER resolves an entry where `!Active() || !(owner || member)` — S13.7 + S15.2 server-side authority; implement as a `testing/quick`-style or exhaustive-table sweep (stdlib only, §3).
- If OQ1 = loud: a surface-level test asserting the mapped status/code for each refusal class (the `mapIntakeErr` table), and that no task row is born from a refused Submit.

## 9. Open questions — coordinator disposition BEFORE the executor launches

- **OQ1 — semantics when a submitted `project` fails validation** (unknown id / not owner-or-member / not ACTIVE). The map pins only the valid case ("when present and the requester owns/belongs to the named ACTIVE entry, it pins…"). Candidates:
  - **(a) Reject loudly at Submit** (4xx per `mapIntakeErr` conventions — e.g. not_found/forbidden; task never born). Bearing: S15.2 "the browser is a display, never an authority" + the door sends ids from a picker, so an invalid pin is a bug or a revoked membership — silence hides it; S06.2's fail-closed instinct. Cost: `Pipeline.Start` swallows seam errors today (`pipeline.go:151`) — (a) needs the refusal routed around the swallow (surface pre-check before `Start`, or a distinguishable seam error), plus a sentinel + mapping.
  - **(b) Silently fall back to the text match.** Bearing: S06.2/§14 degrade posture ("optional duties skipped, never faked") and the existing swallow. Cost: the user believes the task is project-scoped when it is not — the exact failure product-map §5 exists to kill.
  - **(c) Fall back but record the refusal** durably (intake record/decision entry) — loud in the record, quiet in the response.
  - The committed guards hold under all three; item 7 of §7 binds the executor to the ratified one.
- **OQ2 — pin value shape:** registry `project_id` (the primary key; what the Projects tab holds; `Get` is by id) vs name/alias (what `MatchForIntake` scans). Map text "the named ACTIVE registry entry" bears both readings. The committed battery uses the **id** (`"shop"`, whose entry Name is deliberately "shop backend"); a name-based disposition changes fixture literals only. JSON field name `project` is pinned by the map itself.
- **OQ3 — placement of the validation predicate (R4):** (i) inside `internal/project` beside `MatchForIntake` — e.g. `MatchHint.Project` honored within `MatchForIntake`, or an exported sibling lookup — keeping the unexported `visibleTo` the single predicate (favored by §23's F3/single-predicate discipline; makes the store-level sweep natural); (ii) in the shell seam via `Get` + exported `Owner`/`Members`/`Active()` — a second copy of the predicate in a second package. No spec text bears directly; this is structure, but it decides which package holds the security edge and its tests.
- **OQ4 — record provenance:** should the durable record distinguish pinned-from-scanned (a source marker on the intake record / state), or is the recorded `request.project` + `registry_slice_ref` sufficient? S06.1's record fields name no source; the map asks for none. Grounding committed nothing either way.

## 10. SCOPE VERDICT

Small and additive; every touched mechanism already exists and is tested. The single design fork is OQ1 (it decides whether `pipeline.go`/error mapping change at all); OQ2/OQ3 are one-line/one-name forks; OQ4 is a record field. No spec conflict; no amendment; no ⚙; no dependency; no migration.
