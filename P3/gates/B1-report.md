# B1 phase gate — report

**Status: OPEN — awaiting operator.** Written 2026-07-20 at B1 queue completion (4/4 packets done, coordinator-validated, CI green on every push). Contract: `Spec/core-architecture-v1.md` v1 (tag `spec-v1`) + amendment A1. Build record: `P3/STATE.md`; per-packet readings in commit bodies; spike results in `P3/measurements/`.

## 1. What shipped (execution substrate, S19.5 B1)

| Packet | Commit | Delivered |
|---|---|---|
| P3-B1-1 | `c87035b` | D3 adapter contract (`internal/adapters`); Anthropic/claude-CLI lane (`internal/adapters/claudecli`) — 7 S03.1 verbs, events→fenced event-log, S02.4 checkpoint per paid call, engine_sessions + transcript copy-aside, `sinet engine-hook` PreToolUse gate with serialize-by-deny, defer/park→ask-record, resume, cancel; conformance suite v1 (3 tiers); claude CLI lock entry (pin 2.1.214). **Live smoke passed** on the real engine ($0.016). |
| P3-B1-2 | `bbd5847` | Metering ledger (`internal/metering`) — checkpoint-fed consumption read-model, effective-dated price-table seam, receipts, consumption-pressure gauge, Anthropic unified-header overlay; scheduler (`internal/scheduler`) — real admission: Enqueue→CAS-claim under per-(user,lane) slots→Dispatcher seam, five-class limit taxonomy, priority ladder + aging. Zero new deps. |
| P3-B1-3 | `fcc29c0` | Sandbox (`internal/sandbox`) — C0–C2 compose, probes, seccomp-BPF, Landlock seam, Rule-of-Two/P-T05-1, `sinet run-launch` (S11.8); credential broker (`internal/broker`) — `sinet broker` process, per-person AES-256-GCM stores, SO_PEERCRED, resolve/sign split, injection at spawn. +1 os-mechanism lock entry (probe cleared 2026-07-20). Zero new deps. |
| P3-B1-4 | `f8977cd` | B1 spike battery — 3 spikes pre-registered→measured→verdicted (`P3/measurements/2026-07-20-*`). Live spend ~$0.27 of a $5 cap. Instrumentation only, no feature scope. |

## 2. Evidence

- **Tests:** `go test -count=1 ./...` green (19 `ok` packages; sanctioned engine/host-absent skips print a notice — the only skip class, verified via PATH-stripped run); `-race` green on every substantive package; coordinator re-ran the full battery independently after each packet; lock-gate green (7 entries, 11 modules).
- **CI:** green on GitHub for every push (through the B1-3 push, run on `41dd7f0`).
- **Live confinement (B1-3, shipped binary):** `class=C1 seccomp=72 bytes landlock-abi=8`; spawn showed host secret invisible, netns default-drop (no route), uid 1000, exit 0; `PTRACE_TRACEME denied EPERM` inside the sandbox.
- **Live broker round-trip (B1-3, two real processes):** store → resolve (secret matches), signing-key resolve refused, fixture secret absent from broker logs and the encrypted blob (125 bytes, no plaintext).
- **End-to-end dev-mode (B1-2):** run created → queued → CAS-claimed under lane cap → driven through the fake-engine path → checkpoint usage → receipt materialized (labeled UNPRICED, §13-cited), 10-frame `/events` trail.
- **Spikes (B1-4):** serialize-by-deny **PASS-PROVISIONAL**; PreCompact/injection **PASS, TBD resolved**; auto-memory containment **PASS**.

## 3. Demo — run it yourself (literal)

```
cd ~/Sinet-Agentic-Control-Hub
go build -o /tmp/sinet ./cmd/sinet
/tmp/sinet sandbox-probe            # (or the shipped probe verb) → class + seccomp/landlock facts
go test ./internal/sandbox/ -run TestCompose -v   # confinement assertions, skips-with-notice if a tool is absent
go test ./internal/adapters/claudecli/ -run Spike -v   # serialize-by-deny reconfirm harness (fake engine; no spend)
# Real-engine smoke (spends ~$0.02, only if `claude` is installed + authed):
go test ./internal/adapters/claudecli/ -run E2E -tags engine -v
```

## 4. Decisions the operator owns at this gate

1. **Engine pin reconciliation (S03.3):** binding pin is **claude CLI 2.1.214**; the host has **2.1.215**. Conformance passed on 2.1.215, and B1-4 found 2.1.215 actually *fixes* the S03.4 parallel-gate silent-fallback trap (it honors first-defer). Options: **(a)** bump the pin to 2.1.215 via the S03.3 gated procedure (I run the conformance suite as the gate, record the bump as a dated lock edit — this is a worker-revalidation trigger, but no workers exist yet, so it's cheap now); **(b)** hold at 2.1.214 and treat 2.1.215 as an untested-drift observation. Recommendation: **(a)** — bumping while there's nothing downstream to revalidate is the cheapest it will ever be, and 2.1.215 is strictly better on the gate behavior.
2. **`sandbox-runtime` S16.3 row — deviation to ratify:** the spec lists a `sandbox-runtime` library adoption (Apache-2.0, TBD-P3 pin). B1-3 did **not** adopt it — it built the row's *own named funeral-plan alternative* (direct composition of the same kernel primitives), so the composed stack has zero third-party sandbox dependency. This is spec-permitted (it's literally the row's replacement path) but it's a deviation from the row's default, so it's yours to ratify. Recommendation: **ratify the direct-composition posture** — fewer dependencies, and the primitives are the ones the spec already ratifies.
3. **At-rest broker crypto — age/sops adoption:** dev-mode broker stores use stdlib AES-256-GCM. The spec's S16.3 `age`/`sops` rows are for the host-integrated at-rest posture (systemd-creds + encrypted stores). Decide: **(a)** adopt `age` now (+ decide `sops`) through the rail as a B2 item — coheres with **operator item 6** (age identity escrow, due B4); or **(b)** keep stdlib AES-GCM through B1 and adopt at B2 alongside the host installs. Recommendation: **(b)** — it pairs naturally with the B2 host-change batch and the escrow step.
4. **Non-⚙ structural constants (B1-2):** lane-concurrency default (1), scheduler poll interval, and priority-aging biases ship as documented code constants — S18 ratifies no ⚙ keys for them. Same shape as the B0 auth-numbers decision. Recommendation: **keep as constants** (amend S18 later only if you want them tunable).
5. **Price-table data (B1-2):** the price table ships **empty** → every receipt is honestly UNPRICED. The spec's plan is to vendor `genai-prices data.json` (hash-pinned, S16.3). Adopt-at-B2 with the other data/host work, or now? Recommendation: **B2** — receipts stay honest meanwhile, and vendoring is a rail entry best batched.
6. **Packet readings en bloc:** ~24 further clearly-implied readings across B1-1..B1-4 (section-cited in commit bodies; CONVENTIONS §10/§12 hold the durable ones). Ratify en bloc or name any to reopen.

## 5. Operator hands-on items live at this gate

- **Item 1 — Z.AI dashboard prompt-unit calibration** (the 5-step recipe in the P2-S1 report). Due "at B1 for metering trust" — but the Z.AI lane does not exist yet (B1 shipped the Anthropic lane only), so the calibration has nothing to calibrate against until a Z.AI lane is built (a later adapter packet). **Recommendation: defer to when the Z.AI lane lands**; no action needed now. Flag it here so it isn't lost.
- **Item 3 — the probe batch** (`P3/measurements/PROBES.md`): reboot-survival, timer catch-up, freeze/thaw. Due before B1's measurements are *relied on*; the recovery ladder already treats suspend as crash-equivalent, so nothing is blocked, but the reboot-survival answer in particular confirms an assumption B2+ leans on. Run when convenient; results drop into `P3/measurements/`.

## 6. Known-open, tracked (no action needed to close the gate)

- `TBD-BRINGUP(parallel-gate fallback rate)` stays OPEN until S08/B3 fixes the default worker model — the serialize-by-deny reconfirm re-runs against it then (harness committed).
- Auto-memory config-root `memory/` channel: B1 lowering doesn't yet wipe/redirect it — refinement queued for B3/S09.
- Anthropic unified-header park-timing trust: `TrustParkTiming()` hard-false until the S19.6 utilization-scale + `7d_oi` observation window runs (opens now that the overlay reads real headers; coordinator+operator).
- Egress substrate (nftables+proxy), credential-injection proxy, Landlock live enforcement, `run@`/`broker` unit install + polkit: documented host-change seams, deferred to B2 with the install batch.
- Deferred at B2 from the B0 gate: all host installs (units, cert runbook, Caddy) + logind sleep-inhibitor wiring.

**To close:** answer §4 (free text is fine); the coordinator records answers here + STATE, then opens B2. B2 is the walking-skeleton phase — its gate is the live intake→execute→checkpoint→receipt demo on this machine, which is also the natural moment the §4/§5 host-side items come due.
