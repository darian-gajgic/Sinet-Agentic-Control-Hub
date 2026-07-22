# Flan-T5-0.8B serving-path + MiniCheck-task verdict (S12.9, R24, drain C5/F6) — 2026-07-22

- Seat: MiniCheck-Flan-T5-Large (~0.78B, the S12.3 entailment CPU floor / sampled-checks fallback), Q6_K GGUF (`d332117f…`, pulled). Seq2seq (encoder-decoder) with a specialized fact-check task shape, NOT general /v1 chat. b10085. $0 local; on AC.

## Pre-registered expectation

MECHANICAL serving path: does the seq2seq model LOAD and RUN on llama-server at the pin? SEMANTIC leg (mine): with the MiniCheck task format (`predict: <document> claim: <claim>` → `1`/`0`), does it emit a meaningful supported/unsupported verdict? Verdict decides the CPU-floor sampled-checks posture (G3 Def.4).

## Observation (`probe_flant5.sh`, b10085, :8895, -ngl 99)

- **MECHANICAL: serves.** The seq2seq (encoder-decoder) GGUF LOADS and RUNS on `llama-server` at the pin — `/health` returned `ok` ~3 s after launch, no crash. Blackwell offload clean.
- **SEMANTIC: the generic `/completion` `predict:` path does NOT discriminate.** All 4 MiniCheck-format pairs (2 supported, 2 unsupported) returned the SAME degenerate output `'on on on on'` — zero signal, identical for supported and unsupported claims:

| document | claim | expected | Flan-T5 `/completion` output |
|---|---|---|---|
| Apache-2.0 license text | "…is Apache-2.0 licensed" | SUPPORTED (1) | `on on on on` |
| Apache-2.0 license text | "…is MIT licensed" | UNSUPPORTED (0) | `on on on on` |
| "100 requests/minute" | "…allows 100 rpm" | SUPPORTED (1) | `on on on on` |
| "100 requests/minute" | "…allows 1000 rpm" | UNSUPPORTED (0) | `on on on on` |

## Verdict — mechanically-servable YES; generic-endpoint discrimination NO; CPU-floor fallback rests on Guardian, not Flan-T5

- **Mechanically servable: YES** (loads + runs on the pinned llama-server, seq2seq supported).
- **Semantic discrimination via the generic path: NO.** The MiniCheck-Flan-T5 fine-tune emits its supported/unsupported verdict through a **specialized seq2seq classification inference path** (the `minicheck` library's encoder→decoder scoring of the label token), NOT through llama-server's generic `/completion` decoder-style generation — which produces degenerate filler (`'on on…'`) because the task head isn't being invoked. Making Flan-T5-MiniCheck a real entailment seat would require that dedicated inference wrapper (out of scope for the v0 generic-stack path; a STUDY item, not a drop-in seat).
- **CPU-floor sampled-checks posture (the actionable finding):** the entailment fallback rests on **Granite Guardian** (measured **97.4%**, clears the bar — `2026-07-22-entailment-thresholds.md`) as BOTH the mandatory-coverage seat and the sampled-checks seat; **Flan-T5-MiniCheck is NOT a usable generic-stack fallback** at v0. This closes C5 honestly: the model was pulled + probed on the real serving path, and the negative semantic result is recorded rather than assumed. $0 local; host gate clean.
