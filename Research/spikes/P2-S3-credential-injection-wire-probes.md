# Spike P2-S3 — Engine model-egress credential-injection wire probes

**Date:** 2026-07-18 · **Type:** measurement spike (defensive-security, operator's own machine + own subscriptions) · **Status:** COMPLETE
**Closes:** R10-OQ1 (Report 10 §3.4 credential-injection viability per lane) · **Folds in:** R09-OQ6 (`anthropic-ratelimit-unified-*` header lead, Report 09 §2.9 / §7-6)
**Engines under test (pinned):** `claude` 2.1.214 (operator Anthropic subscription) · `opencode` 1.18.3 (Z.AI GLM Coding plan) · **Proxy:** mitmproxy 12.2.3 (user-space venv)

---

## 1. Scope

Report 10 §3.4 keeps the engine's own model-endpoint subscription credential **out of the task sandbox** via **pattern-1**: a host-side **TLS-terminating proxy injects the real token** only on the pinned model-egress request; the sandbox holds a sentinel. §3.4 flagged this load-bearing open risk: *"whether each engine tolerates MITM on its model endpoint (cert-pinning) needs a live per-engine spike."* If an engine cert-pins, Report 10 §4 "what would change the decision #2" forces **pattern-2** (subscription credential sits in the sandbox, egress pinned to the model host only) for that lane.

This spike probes **per lane**: (a) does a local TLS-terminating proxy with a per-process-trusted CA succeed (no cert-pinning), and (b) can the proxy read and rewrite the auth header to inject the real token (demonstrated end-to-end). It also folds in R09-OQ6: confirm/refute the single-sourced `anthropic-ratelimit-unified-*` headers on the real subscription wire and inventory the full `anthropic-ratelimit-*` set (names only).

---

## 2. Method

- **User-space only, no system changes.** mitmproxy ran from the existing scratchpad venv. **The system trust store was never touched.** The mitmproxy CA was trusted **per-process only** via `NODE_EXTRA_CA_CERTS=<confdir>/mitmproxy-ca-cert.pem`, with `HTTPS_PROXY`/`HTTP_PROXY` (upper+lower case) pointed at `http://127.0.0.1:8899`. mitmproxy CA/key material lived in an ephemeral per-spike `confdir` (deleted at teardown).
- **`mitmdump` (headless)** on `127.0.0.1:8899` in default (regular forward-proxy) mode — the faithful shape for pattern-1 (HTTPS CONNECT + TLS termination on the real endpoint), not reverse-proxy.
- **Secret-safe interceptor addon.** A custom addon logged **only header NAMES, booleans, status codes, host, and path** — **never a header value**. No raw flow file was written (`mitmdump` started without `-w`). The real Z.AI key was held in proxy memory solely to demonstrate injection and was never logged.
- **Anthropic lane:** `claude -p "…one word: pong" --max-turns 1 --max-budget-usd 0.05 --output-format json`, using the operator's **existing** subscription auth (default config dir — isolating it would remove the credential and invalidate the test), with the proxy+CA env set per-process.
- **Z.AI lane:** `opencode serve` (fully isolated XDG per G1-S3) launched with proxy+CA env and a **sentinel** `ZHIPU_API_KEY`. One trivial model call (`zai-coding-plan/glm-4.7`) was driven through its REST API (`POST /session` → `POST /session/{id}/prompt_async`). Two runs: **control** (proxy passes the sentinel through) and **injection** (proxy swaps the sentinel for the real key). A no-proxy real-key warmup first populated provider deps and established a baseline.

---

## 3. Findings per lane

### 3.1 Anthropic lane (`claude -p` 2.1.214) — **pattern-1 VIABLE; no cert-pinning**

- **Proxy tolerated: YES, fully.** claude routed **all** its egress through the proxy and every TLS handshake succeeded — **12 requests, 12 × HTTP 200, zero TLS-handshake failures.** The load-bearing `POST api.anthropic.com/v1/messages` returned **200** through the terminating proxy. The turn completed (`num_turns: 1`) and then tripped the engine budget gate (`subtype: error_max_budget_usd`, est. $0.216 > $0.05 cap) — a Class-5 engine ceiling (Report 09 §2.2), **not** a transport failure.
- **claude tolerates a per-process-trusted CA** (`NODE_EXTRA_CA_CERTS`) + ambient `HTTPS_PROXY`. No pinning on `api.anthropic.com`.
- **Ancillary egress observed (design-relevant):** beyond `/v1/messages`, claude also called (all 200 through the proxy) `/api/claude_cli/bootstrap`, `/api/oauth/account/settings`, `/v1/mcp_servers`, `/mcp-registry/v0/servers`, `/api/claude_code_grove`, `/api/claude_code_penguin_mode`, `/api/eval/sdk-*`, `/api/event_logging/v2/batch`. All on `api.anthropic.com`. The credential-injection proxy will see these too → **injection must be scoped to the model path (`/v1/messages`), not applied blindly to every `api.anthropic.com` request.**
- **Token-injection demonstration:** not performed as a live sentinel-swap on this lane (claude used the operator's real OAuth store, not a sentinel). What is proven here is the load-bearing half — **TLS termination is tolerated and the request auth header is fully visible/rewritable at the proxy** (the `/v1/messages` request carried the client auth header and was intercepted cleanly). The actual substitution is a one-line header rewrite, demonstrated end-to-end on the Z.AI lane (§3.2).
- **Verdict: pattern-1-viable.** Report 10 §4 decision-changer #2 does **not** trigger for the Anthropic lane.

### 3.2 Z.AI lane (`opencode serve` 1.18.3) — **pattern-1 VIABLE; injection demonstrated end-to-end**

- **Proxy tolerated: YES.** The model call to `api.z.ai/api/coding/paas/v4/chat/completions` was intercepted; TLS handshake succeeded (zero TLS failures). Note opencode 1.18.3 ships as a **Bun-compiled** binary (not plain Node as the brief assumed) — nonetheless it **honors `NODE_EXTRA_CA_CERTS` and ambient `HTTPS_PROXY`** (Bun's Node-compat), so the per-process trust approach works unchanged. Auth is carried as `Authorization: Bearer <key>` on the **OpenAI-compatible** `/chat/completions` path.
- **Token injection demonstrated (clean A/B):**
  - **Control** (client holds sentinel, proxy pass-through): proxy read the sentinel in the `authorization` header; Z.AI returned **HTTP 401** (×2, incl. one opencode retry). Proves the sentinel alone is rejected.
  - **Injection** (client holds sentinel, proxy strips it and injects the real key): `proxy_injected_real_key=True`; Z.AI returned **HTTP 200** (×2). Proves the proxy can read+rewrite the auth header and that the injected token makes the call succeed.
  - **401 → 200 purely from proxy-side substitution** is the direct proof of pattern-1: the sandbox-side credential can be a sentinel; the host-side proxy supplies the real subscription token at the pinned egress.
- **Verdict: pattern-1-viable.** No cert-pinning; scoped-egress fallback not forced.

### 3.3 Verbatim (REDACTED) evidence

All values redacted at capture time — only names/booleans/statuses were ever written to disk.

```
# Z.AI CONTROL (INJECT off) — sentinel passed through
REQ POST api.z.ai /api/coding/paas/v4/chat/completions | sentinel_in=authorization | proxy_injected=False
RESP status=401  (x2)

# Z.AI INJECTION (INJECT on) — proxy swaps sentinel -> real key
REQ POST api.z.ai /api/coding/paas/v4/chat/completions | sentinel_in=authorization | proxy_injected_real_key=True
     req_header_names=[Accept, Accept-Encoding, Authorization, Connection, Content-Length,
                       Content-Type, Host, User-Agent, x-session-affinity, x-session-id]
RESP status=200  (x2)
Z.AI 200 resp_header_names=[Connection, Content-Type, Date, Server, Strict-Transport-Security,
                            Transfer-Encoding, Vary, X-LOG-ID]   # no rate-limit headers exposed

# Anthropic — all through the terminating proxy, no pinning
REQ POST api.anthropic.com /v1/messages | client_auth_present=True
RESP status=200   (and 11 further 200s across bootstrap/oauth/mcp/event_logging/eval)
TLS handshake failures across ALL runs (both lanes): 0
```

Auth-header **values** (operator OAuth token; Z.AI key) were **never captured**. Redacted to `<REDACTED>` by construction.

---

## 4. `anthropic-ratelimit-*` header inventory (R09-OQ6) — **CONFIRMED, names only, values REDACTED**

The teamclaude single-source lead is **confirmed on the real subscription wire.** `POST api.anthropic.com/v1/messages` (subscription OAuth) returned the **`anthropic-ratelimit-unified-*` family (16 headers)**:

```
anthropic-ratelimit-unified-5h-reset
anthropic-ratelimit-unified-5h-status
anthropic-ratelimit-unified-5h-utilization
anthropic-ratelimit-unified-7d-reset
anthropic-ratelimit-unified-7d-status
anthropic-ratelimit-unified-7d-utilization
anthropic-ratelimit-unified-7d_oi-reset
anthropic-ratelimit-unified-7d_oi-status
anthropic-ratelimit-unified-7d_oi-surpassed-threshold
anthropic-ratelimit-unified-7d_oi-utilization
anthropic-ratelimit-unified-fallback-percentage
anthropic-ratelimit-unified-overage-disabled-reason
anthropic-ratelimit-unified-overage-status
anthropic-ratelimit-unified-representative-claim
anthropic-ratelimit-unified-reset
anthropic-ratelimit-unified-status
```

Plus other Anthropic-specific response headers (names only): `anthropic-organization-id`, `request-id`, `traceresponse`.

**Key correction to Report 09 for the subscription lane:** the **classic** `anthropic-ratelimit-requests-*` / `anthropic-ratelimit-tokens-*` / `-input-*` / `-output-*` buckets documented for API-key auth (Report 09 source [1]) were **ABSENT** on this subscription `/v1/messages` response. The subscription surface exposes the **unified 5h / 7d family** instead (matching the plan's 5-hour + weekly windows; `7d_oi` appears to be the weekly Opus-input sub-limit, with a `surpassed-threshold` flag). These are RFC-style reset + `utilization` + `status` triples — **provider-signaled observed state, D4-clean** (no client-side window modeling).

**Z.AI** exposes **no** rate-limit response headers on `/chat/completions`; consistent with Report 09 §2.2 — Z.AI carries limit state in **error-code bodies** (1302/1305/1308/1310…), not headers.

---

## 5. Implications for the sandbox credential-broker design (D2 / Report 10 §3.4)

1. **R10-OQ1 resolved: pattern-1 is viable on BOTH v0 lanes.** Neither `claude -p` 2.1.214 nor `opencode serve` 1.18.3 cert-pins its model endpoint against a per-process-trusted CA. The engine's own subscription credential can be kept **fully out of the task sandbox** (sentinel inside; real token injected at the pinned model-egress proxy). Report 10 §4 decision-changer #2 (cert-pinning → pattern-2) does **not** fire for either lane; the "definitely sound" scoped-egress fallback (pattern-2) is **not required** at v0.
2. **Per-process CA trust is sufficient — no system trust store change.** `NODE_EXTRA_CA_CERTS` works for both the Node engine (claude) and the Bun engine (opencode). This matches Sinet's launch model (the control plane owns every `exec`, so it sets per-process env). The mitmproxy CA **private key** lives in `confdir` (`mitmproxy-ca.pem`) — the real broker's signing key is equivalently sensitive and must be `0700`, host-only, never in any sandbox.
3. **Scope injection to the model path, not the host.** claude fans out to ~8 ancillary `api.anthropic.com` paths (bootstrap, oauth/account/settings, mcp-registry, event_logging, eval, …). The broker must inject the real token **only** on the model-egress request (`/v1/messages`; Z.AI `/api/coding/paas/v4/chat/completions`) and pass the rest untouched — injecting on telemetry/oauth endpoints is both unnecessary and a needless secret-exposure surface. Injection target is the `Authorization` header on both lanes (Bearer).
4. **The injection proxy doubles as the D4 rate-limit observation point (folds in R09-OQ6).** Because claude tolerates termination, the same host-side proxy that injects the token can harvest the `anthropic-ratelimit-unified-*` headers as provider-signaled observed state — enriching Report 09's Class-2 park-timing (`-reset`) and the C4 consumption meters (`-utilization`, `-status`, `-overage-status`) with **zero** window-modeling (D4-clean). One choke point, two purposes (mirrors the D7 "one write, two purposes" ledger shape).
5. **Add a "model-egress MITM tolerance" canary to the per-substrate conformance suite (P-T01-2).** Both engines are pinned, fast-moving deps; a future release could introduce pinning and silently break pattern-1. The conformance suite should assert, per engine version, that a trusted-CA terminating proxy on the model path still yields a 200 — so a pin-regression is caught at upgrade time, not in production. Fallback if it ever regresses on one lane: pattern-2 (scoped-egress-only) for that lane, exactly as Report 10 §3.4-2 specifies.
6. **Trust-model caveat (honest scope).** This proves the engines accept a **Sinet-controlled, per-process-trusted** CA — it does **not** weaken their TLS against an *untrusted* MITM (they correctly reject that). Pattern-1's security rests on the proxy CA being Sinet-owned and per-process-trusted, which is the design. The residual "exfil-via-model-endpoint with an attacker key" risk (Report 10 §7-6) is *reduced* by injection (an attacker in the sandbox holds only a sentinel, so cannot ride the channel with a real key) but not eliminated (the legitimate injected channel still exists) — unchanged from Report 10's analysis.

---

## 6. Cost

- **Real out-of-pocket: $0.00.** Both lanes are flat-rate subscriptions (no marginal dollar cost); Z.AI 401s consume nothing, the Z.AI 200s consume ~2 subscription prompt-units.
- **Anthropic API-equivalent estimate:** ~**$0.22** for the single probe turn (`total_cost_usd` 0.216125 — a client-side estimate on subscription auth, per Report 09 §2.1; the budget gate fired at the $0.05 cap *after* the turn completed, so the turn's estimate exceeded the cap but no real charge occurred).

---

## 7. Teardown

See final-message confirmation. Proxy killed by PID and verified dead; port 8899 verified free; `opencode serve` (14096) killed and port freed; all flow/evidence/log files, the mitmproxy `confdir` (CA cert **and private key**), and the isolated opencode XDG (incl. any `auth.json`) deleted; no raw flow capture was ever written. No secret value was written to this report or any committed file.
