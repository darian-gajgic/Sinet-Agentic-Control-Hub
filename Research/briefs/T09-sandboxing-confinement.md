# T09 — Sandboxing & the confinement ladder

**Wave:** B1 (consumes G1 addendum) · **Depth:** FULL · **Report slug:** `sandboxing-confinement`

## Scope
S5 complete (C0 connectors / C1 trusted reasoning / C2 workspace-write / C3 web-reading / C4 web-acting-future; the honest caveat), 4.1 (isolation with enumerated exceptions), 4.4 (minimal powers), 4.7 (untrusted content hostile — blast-radius guarantee), D2 (credentials never inside task sandboxes; sandbox is THE isolation boundary). v0 needs C0–C2; v0.1 adds C3.

## Why this gates the spec
D2 deliberately omits OS-user separation — the sandbox carries the entire isolation load for a multi-user household platform. The confinement ladder is also the prompt-injection answer (blast radius, not immunity). Wrong tech choice here is expensive to reverse and constrains the adapter (T01), local models (T15), and every worker definition (4.4's minimal powers).

## Core question
What is the mid-2026 state of the art for sandboxing agent tool execution on a single Ubuntu host — filesystem isolation, per-class network egress control, and strict credential exclusion — suitable for implementing confinement classes C0–C3 as cheap, per-run, low-friction sandboxes on one laptop, without OS-user separation?

## Sub-questions
1. Technology comparison for THIS shape (per-run, seconds-lifetime to hours-lifetime, one host, no cluster): Linux namespaces/bubblewrap-class, Landlock, seccomp, nsjail, firejail, rootless containers (podman-class), gVisor, microVMs (Firecracker/Cloud-Hypervisor-class) — startup overhead, isolation strength, operational burden for one maintainer, and maturity mid-2026.
2. How agent products sandbox NOW: coding-harness sandboxes (Anthropic's, OpenAI's, opencode's permission/sandbox model, cloud sandbox services as pattern-reference only — self-hosting is fixed), what threat models they claim, and post-incident lessons published.
3. Egress control per class: allowlist mechanics (proxy-based vs netns+firewall vs eBPF), practical per-class policies (C1 none; C2 package registries only; C3 fetch/search allowlist), DNS handling, and TLS interception trade-offs (likely What-NOT-to-use).
4. Credential exclusion architecture (D2): provider sessions living OUTSIDE tool sandboxes — broker/proxy process patterns (sandbox asks, broker executes with credentials, response returns), agent-credential-broker prior art; how wrapped first-party CLIs authenticate without their session material entering the task sandbox (interaction with T01's substrate).
5. Prompt-injection blast-radius design mid-2026: the current discourse (lethal-trifecta-class analyses and successors), what combination of egress control + credential exclusion + proposal gating + tighter verification of C3 output is considered adequate containment; honest statement of what remains uncontained (S5.6).
6. Workspace mechanics: workspace-clone from the registered project store (S1.6), copy-on-write options (overlayfs, reflinks), read-only shared caches (packages, models) mounted into sandboxes safely (4.1's enumerated exceptions).
7. C0 connectors (no model in the loop): minimal-scope deterministic API calls — credential scoping per service, egress to that one service only.
8. GPU access from sandboxes (local-model tool calls, future 12.1 image gen): what isolation levels permit GPU use, at what risk — or whether GPU work stays outside task sandboxes as a broker service (bridge to T15).
9. Sandbox-policy definition per worker (4.4): expressing minimal powers as data (mounts, egress, tools) so worker templates (D8) carry their confinement class declaratively.

## Constraints that bind this topic
D2 (absolute: no credentials in task sandboxes, no OS-user separation), 4.2 (outward effects only as proposals — sandboxes never hold send/publish/push capability), D1 (everything on one host — no remote-sandbox dependencies), adopt-don't-fork (engine's own sandbox/permission features used via config only).

## Harvest-map items to verdict
K2 (default-deny permission service pattern), G1/gh-aw (safe-outputs: read-only default, buffered writes, threat-detection pass before externalizing), C3-row/OpenClaw per-agent auth-profile stores (credential isolation reference).

## Sources to prioritize
Sandbox tech docs/benchmarks (current), security researchers on agent isolation + prompt injection (2025–2026), coding-harness sandbox documentation and incident reports, Linux isolation engineering posts.

## Decisions this feeds
G2: sandbox technology per class, credential-broker architecture, egress design. Spec: confinement implementation section, worker permission schema (with T14).
