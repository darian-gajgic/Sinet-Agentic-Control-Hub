# T10 — Memory & knowledge architecture

**Wave:** B2 · **Depth:** FULL · **Report slug:** `memory-and-knowledge-architecture`

## Scope
§8 complete (permissioned learning, traceable/reversible lessons, distillation into playbooks, house knowledge flows into jobs, personal memory, worker-memory hygiene, one shared project truth, sharing by choice, declared precedence), S4.1–S4.11 complete, 15.6 (multi-user data model day one), D8 interplay (overlays ARE per-user memory), D10 (house-knowledge gating).

## Why this gates the spec
Memory shapes the day-one schema (15.6) even though rich learning ships post-v0. The spec must fix: layers with defined writers and lifetimes (S4.2/S4.3), scopes (person/project/house/worker) with declared precedence (8.9), and the retrieval philosophy. The harvest map's anti-item ("deterministic-retrieval-first; vector memory only as post-gate evaluated addition") is a *stance* — this topic tests it against mid-2026 evidence.

## Core question
What memory and knowledge architecture fits Sinet in mid-2026 — layered lifetimes with defined writers, human-gated learning with provenance and clean removal, per-user/project/house scopes with declared precedence — and is deterministic-retrieval-first (files + structured stores) still the right foundation versus the current generation of memory systems (vector, graph, hybrid)?

## Sub-questions
1. Memory-system landscape mid-2026 with production evidence: mem0, Zep/Graphiti, Letta-line, LangMem-class, engine-native memory features (e.g. file-based memories in coding harnesses) — what do real deployments report (retrieval precision, garbage accumulation, maintenance cost), beyond vendor demos?
2. Deterministic-first vs vector-first for an agent *platform* (not a chatbot): evidence on file/structured retrieval (grep-class, structured stores, curated injection) vs embedding retrieval for task-work quality; hybrid patterns; what the strongest 2026 practitioners actually run.
3. Layered lifetimes with defined writers (S4.2/S4.3): scratch (expires with run) / experience (compact summaries, medium-term) / permanent rules (human-written only) — prior art for write-permissioned memory layers; enforcement mechanics (schema-level, not vibes).
4. Human-gated learning loops (8.1–8.3): propose-from-outcomes (edits, comments, accept/reject signals) → human approval → versioned knowledge; who implements gated learning today; proposal quality economics (harvest N13 reference).
5. Provenance + reversible influence (8.2, S4.8): attributing each lesson to origin and *removing* it cleanly — what does removal mean once a lesson influenced later work; practical influence-tracing designs.
6. Scoping and precedence (8.8, 8.9/S4.11): per-user vs project vs house knowledge; adopt-by-choice sharing with visible origin; conflict surfacing (explicit spec > project truth > personal prefs > house defaults) — surfaced as questions, never silent; schema implications.
7. One shared project truth (8.7/S4.4): consistency mechanics when concurrent workers update project knowledge (single store + freshness checks vs eventing); interaction with S1.11 collision handling.
8. Injection selection (bridge from T04): how stored knowledge becomes the per-task slice — indexing/tagging knowledge for rule-based selection; trace visibility of what was injected (S4.6).
9. Forgetting by design (S4.10): TTLs, pruning policies, distill-then-delete patterns; keeping memory curated at household scale (small data, long horizon).
10. Multi-user isolation (10.1): memory leakage risks between users' runs and the schema/query patterns that make leaks structurally impossible (15.6).

## Constraints that bind this topic
D8 (overlays are the per-user worker-memory home), D10 (house knowledge = operator-gated), 8.1 (nothing learned without approval — automation proposes only), 11.3 (memory stores are snapshot-encrypted household data — schema should keep them text-first/exportable), D6 (helpers receive sliced knowledge, never store handles).

## Harvest-map items to verdict
N17 (memory-scope taxonomy: stm/experience/lts/longterm), N13 (gated lessons + KB distillation — later port), N14 (WINS/LESSONS ledger + golden exemplars), N20 (knowledge base as day-one content), anti-harvest "mem0/qdrant-first" row (the stance under test).

## Sources to prioritize
Memory-system project docs + independent evaluations (2025–2026), practitioner essays on agent memory in production, engine memory-feature docs, framework blogs (langchain.com/blog memory line and peers), academic memory-architecture surveys (recent only).

## Decisions this feeds
G2: retrieval philosophy, memory schema (layers/scopes/writers), gated-learning pipeline shape. Spec: memory & knowledge section, day-one schema fields (15.6).
