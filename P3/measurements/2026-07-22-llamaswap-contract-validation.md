# Tier-R behavioral validation — the generated llama-swap config + endpoint contract at the pinned v241 (R26)

- **Packet:** P3-B4-5 (S12 local serving stack), 2026-07-22. A live tier-R conformance check (CONVENTIONS §10; S16.4 #5 "capability claims verified behaviorally … never docs") of the config-gen (R10) + the eager-unload endpoint routes (R12) against the REAL pinned llama-swap, done user-level, $0, no host changes.
- **Method:** downloaded the llama-swap v241 linux/amd64 release binary user-level to a scratch dir (the sanctioned R2 leg; `llama-swap_241_linux_amd64.tar.gz`, LICENSE.md = MIT / Benson Wong in the tarball, matching the lock entry). Generated a config with `sinet local config` — the live nvidia-smi GPU UUID, pool12 swap group, per-class TTLs. Started `llama-swap --config <generated> --listen 127.0.0.1:8799` on an ephemeral loopback port (never a production port) and probed the surface, then stopped it. No llama.cpp/CUDA build and no model files were needed (llama-swap lazy-loads).

## Raw observations (live, 2026-07-22)

- **Binary:** `llama-swap --version` → `version: v241 (8b61e3d), built at 2026-07-22T04:06:05Z` — matches `LlamaSwapPin` + the lock entry.
- **Config ACCEPTED at v241.** llama-swap started and stayed running against the generated config — it validates its YAML schema on startup, so acceptance proves the generated `models:` entries (`cmd`/`ttl`/`unloadTimeout`/`env` with `CUDA_VISIBLE_DEVICES=<UUID>`) and the `groups: pool12: {swap:true, exclusive:false, persistent:false, members:[…]}` shape are correct at the pinned build (R26 "the llama-swap YAML contract").
- **`GET /v1/models`** returned exactly the generated GPU-seated model set — `Arctic-Text2SQL-R1-7B`, `Bespoke-MiniCheck-7B`, `Gemma 4 12B QAT`, `Granite Guardian 3.3-8B`, `Qwen3.5-9B`, `Qwen3.5-4B` — all `status: unloaded`. The model-id keys (including the space-bearing `"Gemma 4 12B QAT"`) round-trip. NO embedder and NO CPU non-servable seat (DeBERTa/Flan-T5) appear — the R10 "never faked into config" rule holds against the real binary.
- **`POST /api/models/unload`** → **HTTP 200** (the eager-unload all-unload route, R12/S12.2 §4.5).
- **`POST /api/models/unload/Qwen3.5-4B`** → **HTTP 200** (the per-model unload route).

## Verdict

The config generator and the eager-unload endpoint routes are **behaviorally validated against the real pinned llama-swap v241** — the R26 llama-swap-contract assertion (and the S16.4 #5 behavior check for the lock entry) confirmed live, not from docs. The generated config is accepted, `/v1/models` matches the manifest's GPU-seated servable seats, and both unload routes respond 200.

## Not covered here (deferred to the B4 gate / hardening — the srt/ttyd host-install precedent)

- The llama.cpp `llama-server` CUDA build (R3 path (b): user-level CUDA toolkit + source build for the Blackwell sm_120 RTX 5070 Ti) — a multi-GB, long-running host-user install, DEFERRED; the `/v1` CONTRACT the conformance asserts is backend-independent, so it does not gate this validation.
- The ~30 GB model pull (operator item e — model-cache location + residency ratification) and therefore the full `/v1/chat/completions` generation with real json_schema-enforcement + logprobs (tier L on a served model) — SANCTIONED-SKIP until the stack is installed at the gate. The tier-F fake exercises the Sinet-side logic; this file adds the real llama-swap-side contract.

## Spend

$0 — a user-level binary download + an ephemeral loopback llama-swap process, no engine calls, no host changes, no production port touched (127.0.0.1:8799 only). The scratch binary lives outside the repo and is never committed.
