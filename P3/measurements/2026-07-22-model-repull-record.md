# Model re-pull leg (S12.9 sanctioned, R19) — 2026-07-22

- Disk checked first: 117 GB free (well above the ~30 GB need). User cache `~/.sinet-b45/models/`. sha256 on arrival → manifest.
- Context: the three B4-5 rate-limited seats sat as partials (Gemma ~4.7/7 GB, Granite-3.3 ~1.4/5.7 GB, Arctic ~4.2/5.4 GB); resumed with `curl -C -` — the HF per-IP rate limit had cleared.

## Observation

| seat | file | size | sha256 | status |
|---|---|---|---|---|
| workhorse-alternate (Gemma) | gemma-4-12b-it-qat-q4_0.gguf | 6,975,879,296 | `93567e57a8fe10b23569b9d9ec38cd005deedf71e29477c421a4b83f418a538b` | **COMPLETE** (resumed rc=0) |
| entailment default (Granite Guardian) | granite-guardian-3.3-8b-Q5_K_M.gguf | 4,203,593,728 | `7dee8bb7b74cb8e8c780be93b62c86eec0dc6baff953431df64ba0c672e3fa4f` | **COMPLETE** (resumed rc=0) |
| layer-2 SQL (Arctic) | Arctic-Text2SQL-R1-7B.Q5_K_M.gguf | 5,444,831,840 | `157cc3e1caafb02a7e4c7abc6e23fbe1bb1b75ef1623fc54885bd17b1a8c0c5d` | **COMPLETE** (resumed rc=0) |
| Granite Guardian **4.1** (S12.9 #5 comparison) | granite-guardian-4.1-tiny-Q5_K_M.gguf | 29 bytes | — | **BLOCKED — no such GGUF** (the `ibm-granite/granite-guardian-4.1-tiny-GGUF` repo/file returns a 29-byte error page; the 4.1 GGUF does not exist at the guessed path). Operator/bring-up item: locate the real 4.1 GGUF (or accept the honest two-way 3.3-only comparison). Not on the v0 default path. |
| Bespoke-MiniCheck-7B | — | — | — | **BLOCKED — HF 401** (CC-BY-NC gated; operator credential + license acceptance — never pulled unauthenticated beyond confirming the 401, per the discipline). |

## Verdict — PASS (the three sanctioned re-pulls COMPLETE with real sha256; two honest blockers recorded)

Gemma, Granite Guardian 3.3, and Arctic re-pulled to completion with verified sha256 — the manifest can re-hash `Pulled:true` for these seats (Gemma's hash consumed by the bakeoff; Granite's by the entailment leg; Arctic's by S14/B5). Granite Guardian 4.1's GGUF does not exist at the guessed repo (honest blocker — the 3.3-vs-4.1 comparison stands two-way / 3.3-only until the operator locates a real 4.1 GGUF). Bespoke-MiniCheck stays the operator 401 act. None of the blocked seats is the v0 default path or the smoke. The completed hashes are recorded here for the manifest re-hash (a `manifest.go` data edit gate-reviewed, not silently applied mid-packet). Host/network gate clean (exactly the sanctioned legs).
