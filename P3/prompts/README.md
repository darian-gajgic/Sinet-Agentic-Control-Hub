# Stage prompt templates (rule R4 — launch prompts carry only packet deltas)

One file per pipeline stage. The coordinator never pastes these into a launch prompt: the launch prompt names the template and supplies the packet deltas (≤2,000 chars total), and the agent reads the template itself as its first act. Templates are the stable half of every stage prompt; edits to them are runbook amendments (log in STATE).

| Template | Stage | Model |
|---|---|---|
| `grounding.md` | 1 — brief + red acceptance tests | inherit (Fable); `opus` when the read-first sections are S10/S11-dense |
| `spotcheck.md` | 1b — brief-vs-spec spot-check (rule R3) | inherit (Fable) |
| `execute.md` | 2 — executor | `opus` always |
| `evaluate.md` | 3 — adversarial evaluation | inherit (Fable); `opus` on S10/S11-dense packets or any classifier trip |
| `finalize.md` | 4 — drain round 2 / dead-executor finalizer | `opus` |

Launch-prompt shape (the only thing the coordinator writes per stage):

```
Read `P3/prompts/<stage>.md` first and follow it exactly.
Packet: P3-<phase>-<n> — <title>.
Brief: P3/briefs/P3-<phase>-<n>.md.   Worktree: <path>   Branch: <name>
Range: <base>..<head>   (evaluation/finalize only)
Deltas: <anything packet-specific: sanctioned test edits by name, skips, ports, model routing>
Report: ≤1,200 chars in chat; full report to P3/reports/P3-<phase>-<n>-<stage>.md (rule R5).
```
