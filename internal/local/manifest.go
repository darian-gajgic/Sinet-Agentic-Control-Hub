package local

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// manifest.go — the Spec S12.3 v0 model set as committed platform data (brief
// R9; §8 reading 6: "Local models are not manifest entries" in components.lock
// — S16.1; the model set is S12's OWN registry). Model WEIGHTS are never
// committed; they live in the shared read-only model cache (Spec S12.1 feature
// 4.1), a structural-config root, operator-relocatable, flagged to the gate.
// This table is the concrete v0 record the S12.9/S12.10 battery+swap machinery
// keys off (duty, model hash, engine build) — the model hash lives here (R18
// D7 rows + B4-7 keying consume the sha256 field).
//
// Licenses are verified ON THE MODEL CARD ITSELF at pull time (Spec S12.3:
// "aggregator roundups hallucinate models and licenses"), with the verify date
// recorded. Apache-2.0/MIT is the default; the sole pre-ratified exception is
// Bespoke-MiniCheck-7B (CC-BY-NC-4.0), carried per-seat WITH its reason. A card
// contradicting a spec-table license cell is recorded as the card's ACTUAL
// license and FLAGGED as a conflict, never silently adopted (R8) — the Gemma 4
// row below.
//
// The model set is PULLED to the model-cache root and the per-file sha256 is
// recorded here (drain F1: the pulls are executed, not deferred). PULLED +
// HASHED this session (Pulled:true, real SHA256): the workhorse-DEFAULT
// (Qwen3.5-9B), the fast/smoke seat (Qwen3.5-4B — loaded on the real GPU for
// the tier-L smoke), and the CPU floor (MiniCheck-Flan-T5). Honest upstream
// blockers (recorded precisely, NOT silently deferred — drain F1): the SQL
// (Arctic), the workhorse-alternate (Gemma 4) and the entailment default
// (Granite) hit HF's per-IP global rate-limit (~0.5 MB/s after ~15 GB of
// session downloads; Granite's own CDN is throttled harder) so their multi-GB
// files cannot complete in-session — sha256 fills at bring-up; Bespoke-
// MiniCheck-7B is 401-GATED (CC-BY-NC — needs an operator HF credential); the
// DeBERTa NLI cross-encoder has NO GGUF (encoder classification head — its
// B4-7 consumer pulls safetensors for a transformers runtime); the embedder
// is post-gate (not pulled). None of the incomplete seats is the v0 default
// path or the smoke — they are B4-7 measurement/bakeoff seats. The model-cache
// location + residency stay an operator ratification (item e).

// License is a seat's card-verified license record (Spec S12.3).
type License struct {
	// SPDX is the SPDX id or the card's declared license tag.
	SPDX string `json:"spdx"`
	// VerifyDate is when the card was read (RFC3339 date).
	VerifyDate string `json:"verify_date"`
	// Note carries a conflict/exception reason (Gemma's card-vs-URL conflict;
	// the MiniCheck CC-BY-NC per-seat exception reason). Empty = clean default.
	Note string `json:"note,omitempty"`
	// Conflict marks a spec-vs-card license conflict flagged to the gate,
	// never silently adopted (R8). Exception marks the ratified per-seat
	// non-default (Bespoke-MiniCheck CC-BY-NC-4.0).
	Conflict  bool `json:"conflict,omitempty"`
	Exception bool `json:"exception,omitempty"`
}

// GGUFFile is one downloadable file of a seat (repo + file name + sha256).
// SHA256 is empty until the bring-up pull records it; the file NAME is
// card-verified now so the pull is mechanical.
type GGUFFile struct {
	Repo   string `json:"repo"`   // HF repo id
	Name   string `json:"name"`   // GGUF file name at the named quant
	SHA256 string `json:"sha256"` // "" until pulled (bring-up); filled on download
}

// SeatRecord is one row of the S12.3 model set (R9): the seat key ⚙
// local.alias values resolve to, the model, the card, the license, the
// file(s), quant, pool, and context length.
type SeatRecord struct {
	// Seat is the alias-target key (a ⚙ local.alias value points here): a
	// seat abstraction ("workhorse", "fast") or a concrete model id.
	Seat string `json:"seat"`
	// Role is the human S12.3 seat role.
	Role string `json:"role"`
	// Model is the model id.
	Model string `json:"model"`
	// CardURL is the HF model card (where License was verified).
	CardURL string     `json:"card_url"`
	License License    `json:"license"`
	Files   []GGUFFile `json:"files"`
	Quant   string     `json:"quant"`
	Pool    string     `json:"pool"` // "pool12" | "cpu" | "" (post-gate)
	// ContextLen is the model's FULL context (a model fact). ServingContext is
	// the BOUNDED context config-gen emits as --ctx-size (drain F9): the full
	// context would OOM on the 12 GB pool with fp16 KV (never quantize KV,
	// S12.3), so each GPU seat carries a sane bounded default that loads;
	// B4-7's VRAM-ledger calibration (S12.7/S12.9) refines these per machine.
	ContextLen     int64 `json:"context_len"`
	ServingContext int64 `json:"serving_context"`
	// GPUSeated: a pool12 member config-gen puts in the swap group (R10).
	GPUSeated bool `json:"gpu_seated"`
	// Servable: llama-server serves this architecture over /v1 at the pin.
	// The CPU encoder/seq2seq seats (DeBERTa NLI, Flan-T5) are recorded
	// honestly false where non-servable, never faked into config (R10).
	Servable bool `json:"servable"`
	// Note carries provenance / post-gate / no-serving-path / coordinator-pick
	// flags (R7/R10).
	Note string `json:"note,omitempty"`
	// Pulled reports whether the weights are present in the cache (sha256
	// recorded). False = bring-up pull pending (the download is operator
	// item e; the alias still resolves — the client refuses at call time
	// with ErrStackAbsent if the model is not loadable).
	Pulled bool `json:"pulled"`
}

// ManifestVersion versions the committed model set (bumped on any swap via
// the S12.10 gate, B4-7).
const ManifestVersion = "s12.3-v0"

// Manifest returns the committed v0 model set (Spec S12.3). It is a code
// table — the simplest "committed, versioned manifest carrying per seat"
// realization (R9). The embedder is present as a POST-GATE record (G2 Def.8):
// NOT pulled, NOT configured; the ⚙ local.alias "embedder" row stays and the
// client refuses it (§8 reading 9).
func Manifest() []SeatRecord {
	const vd = "2026-07-22"
	return []SeatRecord{
		{
			Seat: "workhorse", Role: "Workhorse (default alias target unless the bakeoff flips it)",
			Model:   "Qwen3.5-9B",
			CardURL: "https://huggingface.co/unsloth/Qwen3.5-9B-GGUF",
			License: License{SPDX: "Apache-2.0", VerifyDate: vd},
			Files:   []GGUFFile{{Repo: "unsloth/Qwen3.5-9B-GGUF", Name: "Qwen3.5-9B-Q5_K_M.gguf", SHA256: "dc2a39aef291f91a9116ad214058da0d86eb648743a124bd8c333787c4b9c91c"}},
			Quant:   "Q5_K_M", Pool: "pool12", ContextLen: 32768, ServingContext: 8192, GPUSeated: true, Servable: true, Pulled: true,
		},
		{
			Seat: "fast", Role: "Fast tier (burst/classification; CPU-viable; same family/dialect as the workhorse)",
			Model:   "Qwen3.5-4B",
			CardURL: "https://huggingface.co/unsloth/Qwen3.5-4B-GGUF",
			License: License{SPDX: "Apache-2.0", VerifyDate: vd},
			Files:   []GGUFFile{{Repo: "unsloth/Qwen3.5-4B-GGUF", Name: "Qwen3.5-4B-Q4_K_M.gguf", SHA256: "00fe7986ff5f6b463e62455821146049db6f9313603938a70800d1fb69ef11a4"}},
			Quant:   "Q4_K_M", Pool: "pool12", ContextLen: 32768, ServingContext: 16384, GPUSeated: true, Servable: true, Pulled: true,
		},
		{
			Seat: "workhorse-alternate", Role: "Workhorse bakeoff alternate (B4-7 needs it)",
			Model:   "Gemma 4 12B QAT",
			CardURL: "https://huggingface.co/google/gemma-4-12B-it-qat-q4_0-gguf",
			License: License{
				SPDX: "Apache-2.0", VerifyDate: vd, Conflict: true,
				Note: "CARD-vs-SPEC CONFLICT (R8): the card's license FIELD reads \"apache-2.0\" (matching the S12.3 spec cell) BUT links the Gemma license doc (ai.google.dev/gemma/docs/gemma_4_license); Gemma-family cards historically carry the Gemma Terms. Recorded as the card states + FLAGGED: whether Gemma 4 is truly Apache-2.0 or the Gemma Terms apply under the URL must be operator-ratified before this alternate goes live (B4-7 bakeoff). Not silently adopted either way.",
			},
			Files: []GGUFFile{{Repo: "google/gemma-4-12B-it-qat-q4_0-gguf", Name: "gemma-4-12b-it-qat-q4_0.gguf"}},
			Quant: "first-party int4 QAT", Pool: "pool12", ContextLen: 131072, ServingContext: 4096, GPUSeated: true, Servable: true,
			Note: "PULL RATE-LIMITED (drain F1, honest upstream fact): NOT gated (the file streams, unlike Bespoke's 401) — the initial pull reached ~2.6 GB, then HF's per-IP global rate-limit (~0.5 MB/s, after ~15 GB of session downloads) prevents the ~7 GB file completing in-session; sha256 fills at bring-up. This is the bakeoff ALTERNATE (B4-7), not the v0 default path or the smoke.",
		},
		{
			Seat: "Granite Guardian 8B", Role: "Entailment default seat (AggreFact-frontier grounding screen)",
			Model:   "Granite Guardian 3.3-8B",
			CardURL: "https://huggingface.co/ibm-granite/granite-guardian-3.3-8b-GGUF",
			License: License{SPDX: "Apache-2.0", VerifyDate: vd},
			Files:   []GGUFFile{{Repo: "ibm-granite/granite-guardian-3.3-8b-GGUF", Name: "granite-guardian-3.3-8b-Q5_K_M.gguf"}},
			Quant:   "Q5_K_M", Pool: "pool12", ContextLen: 131072, ServingContext: 8192, GPUSeated: true, Servable: true,
			Note: "S12.3 names Guardian 8B; the current card is Granite Guardian 3.3-8B (evaluate 3.3 vs 4.1 at S12.9, R16 §7-OQ3). Quant policy: Q5/Q6 for ≤9B on pool12; KV cache stays fp16/q8_0 (never quantized). PULL THROTTLED (drain F1, honest upstream fact): the ibm-granite/granite-guardian-3.3-8b-GGUF CDN serves this file at ~0.5 MB/s (vs ~15–20 MB/s for the other repos) — the ~5.7 GB pull cannot complete in-session; sha256 fills when the throttled pull completes at bring-up (the file streams to the cache root). This is the entailment DEFAULT seat, consumed by B4-7's entailment measurement (S12.9), not the smoke or the v0 default path.",
		},
		{
			Seat: "Bespoke-MiniCheck-7B", Role: "Entailment accuracy-alternate (alternate only, never default)",
			Model:   "Bespoke-MiniCheck-7B",
			CardURL: "https://huggingface.co/bespokelabs/Bespoke-MiniCheck-7B",
			License: License{
				SPDX: "CC-BY-NC-4.0", VerifyDate: vd, Exception: true,
				Note: "THE sole ratified v0 license exception (S12.3): non-commercial. Acceptable for this household deployment, never the default; blocked if Sinet ever commercializes. Carried per-seat with this reason (R7/R8). Finetuned from internlm2_5-7b-chat → causal chat model, servable in principle.",
			},
			Files: []GGUFFile{{Repo: "mradermacher/Bespoke-MiniCheck-7B-GGUF", Name: "Bespoke-MiniCheck-7B.Q5_K_M.gguf"}},
			Quant: "Q5_K_M", Pool: "pool12", ContextLen: 32768, ServingContext: 8192, GPUSeated: true, Servable: true,
			Note: "PULL BLOCKED (drain F1, honest upstream fact): every candidate GGUF repo (mradermacher/tensorblock/QuantFactory) returns HTTP 401 — the Bespoke-MiniCheck-7B GGUFs are GATED, consistent with the CC-BY-NC license requiring acceptance. An unauthenticated pull cannot fetch it; the pull needs an operator HF credential + CC-BY-NC acceptance (a per-seat gate step). This is an ALTERNATE (B4-7 bakeoff), not the default path and not the smoke target — flagged, not silently deferred. sha256 fills when the operator provides the credential.",
		},
		{
			Seat: "MiniCheck-Flan-T5-0.8B", Role: "Entailment CPU floor (sampled checks pending G3 Def.4 / S12.9)",
			Model:   "MiniCheck-Flan-T5-Large",
			CardURL: "https://huggingface.co/lytang/MiniCheck-Flan-T5-Large",
			License: License{SPDX: "MIT", VerifyDate: vd},
			Files:   []GGUFFile{{Repo: "nvhf/MiniCheck-Flan-T5-Large-Q6_K-GGUF", Name: "minicheck-flan-t5-large-q6_k.gguf", SHA256: "d332117fff35b692f1006944ab8f11fd254b22fa0ef5a0a6e6ca9765d01c5e75"}},
			Quant:   "Q6_K", Pool: "cpu", ContextLen: 2048, GPUSeated: false, Servable: false, Pulled: true,
			Note: "Flan-T5-Large (~0.78B) is the S12.3 \"0.8B\" CPU floor. Seq2seq (encoder-decoder) with a specialized fact-check task shape, NOT general /v1 chat — its serving path at the v0 pin is B4-7's to validate (S12.9). Recorded honestly Servable=false; NOT configured into llama-swap (R10), never faked.",
		},
		{
			Seat: "DeBERTa-class NLI cross-encoder", Role: "Contradiction pre-screen (high-precision advisory)",
			Model:   "cross-encoder/nli-deberta-v3-base",
			CardURL: "https://huggingface.co/cross-encoder/nli-deberta-v3-base",
			License: License{SPDX: "Apache-2.0", VerifyDate: vd},
			Files:   nil, // no GGUF serving path — see Note
			Quant:   "n/a (encoder classification head)", Pool: "cpu", ContextLen: 512, GPUSeated: false, Servable: false,
			Note: "COORDINATOR-FLAGGED PICK (R7): the spec names a DeBERTa-CLASS NLI cross-encoder (~100 MB), not a card; cross-encoder/nli-deberta-v3-base (Apache-2.0, ~100 MB, entailment/neutral/contradiction head) is the concrete choice for B4-7's contradiction-screen consumer to ratify. NON-SERVABLE via llama-server /v1: an encoder sequence-classification head is not a chat/completions model — no GGUF serving path at the v0 pin. Recorded honestly, NOT configured into llama-swap (R10); its consumer is B4-7 (memory.ContradictionScreen stays nil).",
		},
		{
			Seat: "Arctic-Text2SQL-R1-7B", Role: "Layer-2 open SQL (enabled at v0, flagged lower-confidence; G3 D3.5)",
			Model:   "Arctic-Text2SQL-R1-7B",
			CardURL: "https://huggingface.co/Snowflake/Arctic-Text2SQL-R1-7B",
			License: License{SPDX: "Apache-2.0", VerifyDate: vd},
			Files:   []GGUFFile{{Repo: "mradermacher/Arctic-Text2SQL-R1-7B-GGUF", Name: "Arctic-Text2SQL-R1-7B.Q5_K_M.gguf"}},
			Quant:   "Q5_K_M", Pool: "pool12", ContextLen: 32768, ServingContext: 8192, GPUSeated: true, Servable: true,
			Note: "Enabled at v0; its query-surface consumer (the S12.3 guardrail stack: read-only conn, allowlisted views, single-statement parse, LIMIT+timeout, audit-logged, flagged lower-confidence) is S14/B5 — not built here. PULL RATE-LIMITED (drain F1, honest upstream fact): the initial pull reached ~4.0 GB at ~15 MB/s, then HF applied a per-IP global rate-limit (~0.5 MB/s) after ~15 GB of session downloads — the ~5.4 GB file cannot complete in-session; sha256 fills at bring-up. Its consumer is S14/B5, not the v0 default path or the smoke.",
		},
		{
			Seat: "Qwen3-Embedding-0.6B", Role: "Embedder — POST-GATE ONLY (G2 Def.8 vector gate not opened)",
			Model:   "Qwen3-Embedding-0.6B",
			CardURL: "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF",
			License: License{SPDX: "Apache-2.0", VerifyDate: vd},
			Files:   nil, // NOT pulled
			Quant:   "Q8_0", Pool: "", ContextLen: 0, GPUSeated: false, Servable: false,
			Note: "NOT PULLED, NOT CONFIGURED (§8 reading 9): the G2 Def.8 vector post-gate (embeddings only at ≥5% lexical-miss rate or >5,000 entries) has NOT opened. The ⚙ local.alias \"embedder\" row stays as S12.4 registry data; the duty client refuses it with the post-gate reason (ErrEmbedderPostGate). Recorded here for provenance only.",
		},
	}
}

// seatByKey returns the manifest seat whose Seat key matches, and whether
// found. The alias resolver looks up ⚙ local.alias values here (R15).
func seatByKey(key string) (SeatRecord, bool) {
	for _, s := range Manifest() {
		if s.Seat == key {
			return s, true
		}
	}
	return SeatRecord{}, false
}

// ModelHash returns the seat's model hash — the sha256 of its first GGUF
// file (the "model hash" the D7 marker and the S12.9/S12.10 battery key on,
// R9/R18). Empty until the bring-up pull records the sha256. A DOWNLOADED
// manifest carries the real hash.
func (s SeatRecord) ModelHash() string {
	if len(s.Files) == 0 {
		return ""
	}
	return s.Files[0].SHA256
}

// FileSHA256 computes the sha256 hex of a pulled GGUF file's bytes — the
// bring-up pull uses it to fill the manifest (kept here so the hashing
// authority is one place; R7 "every pulled file's sha256 recorded").
func FileSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ManifestJSON returns the manifest as pretty strict JSON — the committed
// data form (R9 "code table or strict-JSON data"), also what the operator
// bring-up tooling reads to drive the pull. Deterministic (seats already in
// a fixed order).
func ManifestJSON() ([]byte, error) {
	m := Manifest()
	sort.SliceStable(m, func(i, j int) bool { return m[i].Seat < m[j].Seat })
	return json.MarshalIndent(struct {
		Version string       `json:"version"`
		Seats   []SeatRecord `json:"seats"`
	}{ManifestVersion, m}, "", "  ")
}
