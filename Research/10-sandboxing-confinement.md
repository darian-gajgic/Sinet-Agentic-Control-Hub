# Research 10 — Sandboxing & the confinement ladder (T09)

**Wave:** B1 · **Depth:** FULL · **Date:** 2026-07-17 · **Status:** COMPLETE
**Consumes (binding):** `Research/decisions/GATE-1-architecture-direction.md` (G1), `Research/spikes/G1-S1-cli-wrap-vs-sdk.md`, `G1-S2-defer-drill.md`, `G1-S3-opencode-park-xdg.md`, reports 01/02.
**Feeds:** G2 (sandbox tech per class, credential-broker architecture, egress design); spec confinement-implementation section; worker permission schema (with T14).

---

## 1. Scope

Feature-list items researched:

- **S5 complete** — the confinement ladder: C0 connectors, C1 trusted reasoning, C2 workspace-write, C3 web-reading, C4 web-acting (future, 12.2), and S5.6 "the honest caveat" (blast radius, not immunity).
- **4.1** isolation with enumerated exceptions (ro shared caches, workspace-clone, explicitly-shared resources).
- **4.4** minimal powers per worker.
- **4.7** untrusted content treated as hostile — the blast-radius guarantee.
- **D2** — credentials never inside task sandboxes; the sandbox is THE isolation boundary (no OS-user separation).
- Touchpoints: **S1.6** (registered project store → workspace clone), **4.2** (outward effects only as proposals; sandboxes hold no send/publish/push), **12.1** (future local image gen → GPU).

**Build target:** v0 needs C0–C2; v0.1 adds C3; C4 is future. This report specifies the whole ladder so the v0 tech choice does not have to be reversed to add C3/C4 (the brief's "expensive to reverse" warning).

This topic sits directly on top of two G1-ratified layers it does not re-litigate: **systemd transient units per run process are the adopted baseline** (PID-1 time ceilings + cgroup accounting) — this report researches what composes *on top*; and **all process spawning is control-plane-owned with engine-native subagents disabled on every substrate** — which is what makes external sandbox-wrapping enforceable at all (Sinet owns every `exec`, so it owns every jail).

---

## 2. Current state of the art (mid-2026)

### 2.1 The field has converged on layered kernel primitives for local agent CLIs

Both shipping first-party coding harnesses sandbox tool execution with the **same stack**, independently arrived at: **bubblewrap (user namespaces) + seccomp-BPF + an out-of-sandbox network proxy on Linux; Seatbelt (`sandbox-exec`) on macOS**; OpenAI's Codex adds **Landlock** to the Linux path. [S-ANTH-BLOG, S-ANTH-DOCS, S-SRT, S-CODEX-DOCS, S-WILLISON-CODEX — all primary vendor docs/repos, cross-verified by an independent hands-on investigation]. Neither ships a VM for the local case. The explicit framing across the field: this namespace tier is **defense-in-depth, not a hard multi-tenant boundary** — cloud services that run genuinely untrusted multi-tenant code step up to **gVisor** (user-space kernel; Claude.ai web, Modal, Daytona) or **Firecracker microVMs** (E2B) [S-MICROVM-2026, S-CLOUD-CMP — practitioner/vendor tier].

Anthropic states the design goal in Sinet's own words: sandboxing exists so that "even a successful prompt injection is fully isolated" — a compromised agent "can't steal your SSH keys, or phone home to an attacker's server" — and stresses that **filesystem AND network isolation are inseparable**: network-only leaves credential exfil open; filesystem-only lets an agent backdoor resources to regain network [S-ANTH-BLOG, primary]. Anthropic reports internal sandboxing cut permission prompts **84%** (vendor-internal, unverifiable externally — flagged).

Critically, Anthropic has **open-sourced this exact machinery** as `@anthropic-ai/sandbox-runtime` ("srt", Apache-2.0, Beta Research Preview): a library that wraps *any* process (agents, MCP servers, bash) in bubblewrap + seccomp + host-side HTTP/SOCKS5 proxy, configured by a single JSON file, **no container image required** [S-SRT, primary repo]. This is the strongest single piece of adoptable prior art for Sinet and is discussed throughout.

### 2.2 Credentials-outside-the-sandbox is now productized, not aspirational

The pattern D2 mandates is shipping in the field:

- **Codex** runs the engine (holding the API credential) *outside* the sandbox; "the sandbox applies to spawned commands," which run with network off by default [S-CODEX-DOCS, primary].
- **Claude Code on the web** fronts all git traffic with a proxy service: "sensitive credentials (such as git credentials or signing keys) are never inside the sandbox"; inside the sandbox, git holds only a *scoped* credential for the proxy, which validates auth and branch names before attaching the real token and forwarding to GitHub [S-ANTH-BLOG, primary].
- **MCP's authorization spec (2025-11-25)** makes the server an OAuth 2.1 resource server and **forbids token pass-through**: "The MCP server MUST NOT pass through the token it received from the MCP client," and names the confused-deputy problem explicitly [S-MCP-AUTH, primary spec]. A running-code instance: NVIDIA NemoClaw's host-side stdio-to-HTTP MCP bridge spawns servers with keys from the *host* environment, binds 127.0.0.1 only, and reaches the sandbox through a per-server scoped egress rule — "API keys never enter the sandbox" [S-NEMOCLAW, primary repo issue].

The canonical design for "exercise a credential without exposing it" remains **ssh-agent**: keys live in the agent's memory, clients request an *operation* (sign this), and only the result returns; OpenSSH 8.9's **destination constraints** (`ssh-add -h host>host2`) cryptographically bind each use to a destination, the textbook confused-deputy fix [S-SSH-AGENT, S-SSH-RESTRICT, primary man pages]. **CyberArk Secretless Broker** productizes the same shape for DB/SSH/HTTP ("connection secrets… never exposed to the client") but is Kubernetes-sidecar-heavy [S-SECRETLESS, primary].

### 2.3 Egress control: the firewall is the boundary; the proxy is only convenience

The whole field has converged on one egress architecture, and — more importantly — on **why the naive version fails**. The design: each sandbox gets a **network namespace with a default-DROP firewall (nftables/iptables)** as the real boundary, forcing all traffic to a **host-side allowlisting proxy** that decides by hostname (TLS SNI / HTTP CONNECT, **no TLS interception**), backed by **IP deny-CIDRs** for cloud-metadata (169.254.169.254) and RFC1918 ranges [S-INIT-FW, S-INNOQ, S-SNAS, S-IRONPROXY — primary + practitioner + academic].

The load-bearing lesson, proven by a run of 2025–2026 CVEs, is that **hostname/string allowlisting at the proxy is not a security boundary** — it must sit behind the firewall's IP:port default-drop:

- **CVE-2025-66479** (Claude Code / srt): `allowedDomains: []`, documented as "block all," was read as allow-all (a `length > 0` check disabled the proxy). Network enforcement silently absent for ~30 releases; fixed srt 0.0.16 / Claude Code v2.0.55 [S-CVE-66479, NVD + researcher].
- **SOCKS5 null-byte bypass** (Claude Code): `attacker.com\x00.google.com` passed the JS `endsWith('.google.com')` check while libc `getaddrinfo()` truncated at the null byte and dialed the attacker. Affected v2.0.24 through ~v2.1.89 (~5.5 months), silently patched [S-PENLIGENT, S-KOUKYOSYUMEI, researcher tier].
- **CVE-2025-55284**: an agent-tool default allowlist auto-approved `ping`/`nslookup`/`dig`, enabling **DNS-tunnel exfiltration** (secret encoded as query subdomains) through allowed resolvers [S-DNS-EXFIL, advisory].
- **CamoLeak / CVE-2025-59145** (Copilot Chat, CVSS 9.6): exfiltration through GitHub's *own trusted* image proxy `camo.githubusercontent.com`, one pre-signed URL per character — an **allowlisted egress channel is an exfil channel** [S-CAMOLEAK, researcher].
- **StepSecurity harden-runner CVE-2026-32947**: even an eBPF DNS-layer allowlist was bypassed by **DoH tunneling** — encrypted DNS defeats domain filters [S-HARDENRUNNER-CVE, primary advisory].

Anthropic's own docs concede the residual: their proxy "does not terminate or inspect TLS" by default, so a broad allow like `github.com` enables **domain fronting** to hosts outside the list [S-ANTH-DOCS, primary]. The field's consensus fix is not TLS-MITM-everywhere (that breaks cert-pinned tools and enlarges the trust base) but: **firewall default-drop as the boundary, resolve-and-pin with post-resolution CIDR checks, block outbound DoH resolvers, restrict HTTP methods (Codex cloud offers GET/HEAD/OPTIONS-only), and keep the sandbox's own reachable surface minimal** [S-CODEX-CLOUD, S-SNAS, primary/academic].

### 2.4 Prompt-injection: the consensus is "assume compromise, bound the blast radius"

Sinet's S5.6 framing — containment, not immunity — **is** the mid-2026 consensus, not a hedge. Four independent primary voices converge:

- **Simon Willison's lethal trifecta** (private data + untrusted content + external communication): guardrail products claiming "95% of attacks caught" are "presenting a failing grade"; "we still don't know how to 100% reliably prevent this" [S-TRIFECTA, primary; 13 mo old, flagged but actively cited through 2026].
- **Meta's "Agents Rule of Two"** (Oct 2025): an agent should satisfy **at most two of** {processes untrusted input, accesses sensitive data/systems, can change state or communicate externally} within a session; needing all three demands human-in-the-loop or "another reliable means of validation." A session may safely *transition* between two-property phases if the transition breaks the attack chain [S-RULE-OF-TWO, primary]. This is the citable rule for Sinet's ladder.
- **"The Attacker Moves Second"** (arXiv 2510.09023; OpenAI + Anthropic + GDM authors): adaptive attacks broke **12 published defenses at >90% success, 100% under human red-teaming**, despite near-zero original claims — the load-bearing evidence that probabilistic defenses cannot *be* the guarantee [S-ATTACKER-SECOND, primary].
- **Anthropic, "How we contain Claude"** (May 2026): "Design for containment at the environment layer first, then steer behavior at the model layer." Model-layer numbers are offered as explicitly insufficient-standalone (Opus 4.7: ~0.1% single-attempt injection success, but **5–6% after 100 adaptive attempts**) [S-CONTAIN, primary].

Design-level answers exist for the "you need all three" case — **CaMeL** (capability-tagged data-flow interpreter; provable containment at a measured utility cost: 77% vs 84% of AgentDojo tasks) and the **six design patterns** paper (Action-Selector, Plan-Then-Execute, Dual-LLM, Code-Then-Execute, Map-Reduce, Context-Minimization) — all sharing the property that **consequential actions are fixed before untrusted ingestion, or tainted data is structurally barred from triggering them** [S-CAMEL, S-PATTERNS, primary papers]. These are research-grade, not yet productized as drop-ins (flagged).

### 2.5 Proposal-gating and declarative policy have shipping exemplars

- **gh-aw (GitHub Agentic Workflows) "safe-outputs"** (technical preview, Feb 2026): the agentic job runs with a **read-only token**, sandboxed, egress-controlled, and — instead of writing — emits **structured JSON action requests** that are buffered, validated against declared per-type caps/allowlists, passed through a **separate threat-detection LLM job** (scanning agent output + git patch + original intent for injection/secret-leak/malicious-patch), and only then replayed by a **separate minimal-permission write job**; config cannot be overridden by agent output [S-GHAW-SAFE, S-GHAW-THREAT, primary docs]. This is Sinet's 4.2 ("no un-proposed outward effect") as running code.
- **Default-deny tool authorization**: Amazon Bedrock **AgentCore Policy** (GA Mar 2026) evaluates every agent→tool call against **Cedar** policies at a gateway — default-deny, forbid-overrides-permit, enforced *outside agent code and prompt reach* [S-AGENTCORE, primary]. OPA/Rego equivalents exist. **MCP tool annotations** (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) are the emerging shared risk vocabulary — with the spec's own caveat that annotations are *untrusted hints*, so policy must bind to tool identity, not the hint [S-MCP-ANNOT, primary].
- **Confinement-as-data** precedent is mature: seccomp/AppArmor/SELinux profiles, Kubernetes `securityContext` + `NetworkPolicy`, Google's **action manifests** (per-action declarative effects/auth/data-types consumed by a deterministic policy engine), and srt's single-JSON three-section schema (network allow-list, filesystem read-deny/write-allow, enforcement primitives) [S-K8S-SECCOMP, S-GOOGLE-SAIF, S-SRT — primary].

---

## 3. Candidate approaches

### 3.1 Filesystem + process isolation layer

| Technology | Startup | Isolation strength | One-maintainer burden | Verdict for Sinet |
|---|---|---|---|---|
| **bubblewrap** (userns) | single-digit ms | shared kernel; strong with seccomp+Landlock | very low — one static binary, no daemon, no images | **PRIMARY.** Used by Claude Code + vendored by Codex; srt wraps it directly. |
| **Landlock LSM** | ~0 (self-imposed) | filesystem + TCP(+UDP) port scoping; unprivileged, stackable | very low — kernel-native, composes inside bwrap | **ADOPT as inner layer** (allowlist FS + egress ports). |
| **seccomp-BPF** | negligible/syscall | syscall-surface reduction (kill ptrace/process_vm_readv/odd execve) | low — ship a static profile | **ADOPT as inner layer** (defense-in-depth, not the boundary). |
| **nsjail** | ms | namespaces+cgroups+rlimits+Kafel seccomp | low-moderate — config-driven, less agent-ecosystem adoption | Viable alt to bwrap; no reason to switch given bwrap's adoption. |
| **firejail** | ms | **setuid-root**; long escape history | — | **REJECT** (§5). |
| **rootless podman/crun** | ~0.15–1 s (rootless +~0.45 s first) | shared kernel; larger runtime codebase (runc trio CVE-2025-31133/-52565/-52881) | moderate — image/storage mgmt; buys OCI ecosystem + gVisor on-ramp | Heavier tier for runs needing a full distro userland. Not the default. |
| **gVisor (runsc)** | ~50–100 ms | user-space kernel; host surface ~53–68 syscalls vs 450+; 3 CVEs 2025, no public full escape | low — single binary, drops into podman as OCI runtime | **STEP-UP tier for C3/hostile-input runs** and the only working GPU-in-sandbox story. |
| **microVM** (Firecracker/Cloud-Hypervisor/QEMU) | 125 ms–5 s | strongest (hypervisor boundary) | **highest** — curate guest kernel+rootfs, jailer, TAP net, virtiofs, second kernel to patch | **REJECT for v0** (§5); revisit only if a run must be zero-trust *and* the bus-factor-1 cost is justified. |

**Recommended composition (per run, on the adopted systemd transient unit):**
`systemd transient unit (time ceiling + cgroup accounting) → bwrap (user/mount/PID/net/UTS/IPC ns; per-run workspace; ro-bind caches) → seccomp-BPF profile → Landlock ruleset inside`. This is exactly the Codex/Claude-Code/srt stack, starts in milliseconds, and every layer is a no-daemon kernel primitive one maintainer can hold in their head [S-SANDLOCK, S-MICROVM-2026, S-SRT].

Two Ubuntu-specific must-verify items (both real, both current): (a) **`kernel.apparmor_restrict_unprivileged_userns=1`** has been the default since 24.04 and unconfined bwrap gets a capability-less userns that dies at mount — the fix is a per-binary AppArmor profile containing `userns,` (25.04+ ships `bwrap-userns-restrict`); **never** flip the sysctl off globally [S-APPARMOR-USERNS, S-JDHODGES, distro docs + practitioner]. (b) **Landlock ABI level on kernel 7.0**: ABI 4 (TCP bind/connect scoping) and ABI 7 (audit, kernel 6.15) are firmly sourced; **ABI 8 (TSYNC multithreaded enforcement — important, since engine processes are multithreaded) and ABI 10 (UDP scoping) kernel-version mappings are not pinned by any fetched primary source** — resolve on the box with a `landlock_create_ruleset(NULL,0,VERSION)` probe [S-LANDLOCK-MAN, S-LANDLOCK-KERNEL, S-LANDLOCK-NEWS, primary/uncertain].

### 3.2 Workspace mechanics (S1.6, 4.1)

- **Reflink CoW (btrfs/XFS/bcachefs):** `cp --reflink` clones the registered project store metadata-only (shared extents, independent files, ms-scale) — plain directories afterward, so **Landlock/git/tools "just work."** **Gotcha: ext4 has no reflink → full physical copy** — the project-store volume must be btrfs or XFS [S-REFLINK, S-YOLOAI, primary/repo].
- **OverlayFS (ro lower = project store; per-run upper):** the upperdir *is* the run's diff (useful for commit-or-discard review), millisecond-cheap, wrapped natively by bwrap ≥0.11 (`--overlay`/`--ro-overlay`/`--tmp-overlay`). **Two gotchas:** touching the lower dir while mounted is undefined behavior, and **Landlock rules do not propagate through overlay layers — policies must target the mounted overlay path** [S-OVERLAYFS, S-LANDLOCK-MAN, primary]. `DeltaBox`/`AgentFS` demonstrate ms-scale checkpoint/rollback on reflink-backed overlays [S-DELTABOX].
- **Read-only shared caches** (package caches, model weights): `bwrap --ro-bind` / systemd `BindReadOnlyPaths=` [S-SRT, S-SYSTEMD-SANDBOX]. The poisoning risk to avoid is a *writable* cache shared across runs (one compromised run seeds the next) — for caches tools insist on writing, use an overlay with shared ro lower + per-run throwaway upper.

**Recommendation:** project-store volume on **XFS or btrfs**; per-run workspace = **reflink clone** (semantically boring, Landlock-clean) as the default; **overlay** where commit-or-discard diff semantics are wanted (aligns with the accepted-work-as-commit flow, D9). Caches ro-bind only.

### 3.3 Egress layer (per class)

Adopt the field-consensus architecture (§2.3): **per-run netns + nftables default-DROP (the boundary) + host-side allowlisting proxy (hostname convenience) + IP deny-CIDRs (metadata/RFC1918) + blocked outbound DoH.** srt gives this off-the-shelf on Linux (netns removed, all traffic via Unix sockets to host HTTP/SOCKS5 proxies) [S-SRT, primary]. Per-class policy:

- **C1 (trusted reasoning) & verification runs:** netns with **no veth / no route** — strongest and simplest; no proxy. Matches G1's "verification runs default network-off + history-stripped."
- **C2 (workspace-write):** default-drop + proxy allowlisting **package-registry hostnames** (npm/pypi/crates/apt/go/maven/nuget/rubygems — GitHub Copilot's maintained allowlist and Codex's ~80-domain "common dependencies" preset are adoptable artifacts). **CDN fan-out gotcha:** registries redirect downloads to CDNs (`files.pythonhosted.org`/RubyGems = Fastly; `static.crates.io`) — allowlist the CDN host, not just the API host, and prefer an **on-host caching mirror** (devpi/verdaccio-style) so C2 can eventually narrow to one internal host [S-COPILOT-ALLOW, S-CODEX-CLOUD, S-PYPI-FASTLY, primary].
- **C3 (web-reading):** the agent gets **no raw egress**; arbitrary domains are reachable **only through one allowlisted fetch/search service host** (the C3 fetch broker). The sandbox's own allowlist is just that one host; the broker does the fetching, treats results as data, and returns them for tighter verification.
- **C0 (connectors):** single-host allowlist + IP deny-CIDR guard.

TLS interception stays **off by default** (§5). The one justified exception is the engine's own model-egress path (§3.4), where Sinet owns both ends.

### 3.4 Credential-broker architecture (D2) and the engine-credential problem

**The broker.** Build a single host-side, **ssh-agent-shaped broker daemon** (per-user Unix socket): the sandbox submits a *typed operation*, the broker attests the caller by **Unix-socket peer credentials** (SO_PEERCRED — the SPIFFE insight: authenticate the *asker*, don't hand it a secret), evaluates policy, executes with the owner's credential outside the sandbox, and returns **only the result**. Never model the sandbox-facing API on git-credential helpers (which return the secret) — ssh-agent (operation-in/result-out) is the model; git-credential is the anti-model [S-SSH-AGENT, S-GIT-CRED, primary]. Adopt three broker invariants from the field: per-credential **destination constraints** (ssh-agent), **never pass tokens through** + **audience-binding** (MCP spec MUSTs), and a **PDP/CDP split** kept as an internal code boundary (SANS's CB4A: policy decision holds zero credentials; delivery mints/uses narrow short-lived tokens on approval) [S-SSH-RESTRICT, S-MCP-AUTH, S-CB4A]. The broker is the single choke point — one place to log, revoke, and rate-limit — and outward-effect operations (push/publish/send) route to the existing 4.2 gated-proposal flow, never auto-execute.

**MCP as the natural broker surface** for the model-in-the-loop case: run MCP servers **outside** the sandbox holding their own credentials, exposing only scoped tools; the spec already forbids token pass-through and mandates audience binding [S-MCP-AUTH, S-NEMOCLAW]. For deterministic **C0 connectors**, plain host-side code with **one narrowest-scope provider credential per (person × service × capability)** — Stripe restricted keys (`rk_…`, "especially when handing a key to an AI agent"), Gmail `gmail.send` never `gmail.modify` — plus a one-host egress pin [S-STRIPE-KEYS, S-GMAIL-SCOPES, primary].

**The engine-credential tension (the sharpest design question in this topic).** D2 says credentials never enter the task sandbox — but the execution engine (`claude -p`, `opencode serve`) needs *its own* subscription/API credential to reach the model endpoint, and (given adopt-don't-fork) Sinet cannot split the engine's auth from its tool execution by modifying it. Two reconciliations, both with field precedent:

1. **Credential injection at the pinned model-egress path (preferred).** The sandbox holds only a **sentinel**; a TLS-terminating broker/proxy substitutes the real subscription token **only** on requests to the pinned model endpoint. This is Anthropic's productized `mask` + `injectHosts` + `network.tlsTerminate` and the Formal/mitmproxy "dummy key, proxy injects real one" pattern [S-ANTH-DOCS, S-FORMAL]. It is the one place TLS-MITM is justified — Sinet owns both ends. **Open risk:** whether each engine tolerates MITM on its model endpoint (cert-pinning) needs a live per-engine spike before this is load-bearing.
2. **Scoped-egress fallback (definitely sound).** If an engine won't tolerate injection, its subscription credential may sit in the sandbox **but its egress is pinned to the model endpoint only** — blast radius becomes "can call the model as this user," not "can reach arbitrary hosts," combined with **no other credential** in the sandbox and **no outward-effect capability**. Residual risk is exfil-via-model-endpoint (the Anthropic-incident class), which the C3 fetch-broker split and tighter verification bound but do not eliminate.

**Substrate granularity differs and this is load-bearing.** `claude -p` is **per-run** → wrap each invocation in a per-run sandbox cleanly (engine + its sandboxed Bash subprocesses). `opencode serve` is a **per-user persistent HTTP server that executes tools in-process and ships no OS sandbox of its own** (issue #5529) — so it cannot be a per-run jail. The workable, adopt-don't-fork path: run the whole `opencode serve` inside a **per-user** bwrap+netns jail (scoped to that user's workspaces + model-egress + no other credentials), give each session its **own reflink workspace clone** for per-run file isolation, and configure opencode's **app-level** `permission`/`allowRead`/`denyRead`/`allowedDomains` as an inner *soft* layer (configuration, not modification) [S-OPENCODE-PERMS, S-OPENCODE-ISSUE, primary]. The **cross-user boundary — the D2-critical one, since there is no OS-user separation — is preserved on both lanes** (per-user server + per-user XDG (spike S3) + per-user jail); per-run OS-level isolation is stronger on the claude lane than the opencode lane. See Open Questions for whether C3 on the opencode lane warrants ephemeral per-run `opencode serve` instances.

### 3.5 GPU access (12.1 future; bridge to T15)

The evidence says **keep GPU work outside the task sandbox, behind a broker service**:

- Binding `/dev/nvidia*` into a sandbox exposes the **full NVIDIA kernel-driver ioctl surface**, an actively exploited LPE surface (NVIDIA's Jan 2026 bulletin; CVE-2025-33219 integer overflow reachable by a low-privilege local process via `/dev/nvidia*`) [S-NVIDIA-LPE, S-CVE-33219, vendor advisory]. The NVIDIA Container Toolkit itself was an escape surface (**CVE-2025-23266 "NVIDIAScape," CVSS 9.0**, three-line Dockerfile → host root; fixed 1.17.8) [S-NVIDIASCAPE, primary].
- **gVisor nvproxy** proxies an allowlisted ioctl set against pinned driver versions — better than VFIO passthrough — but officially supports **datacenter GPUs (T4/A100/L4/H100)**; a **Blackwell mobile RTX 5070 Ti is unverified**, driver bugs behind allowlisted ioctls remain reachable, and nvproxy breaks in rootless mode [S-GVISOR-GPU, primary].
- **Firecracker refuses GPU passthrough by policy**; Cloud-Hypervisor/QEMU VFIO weakens the VM boundary via DMA and is a non-starter on a single-GPU hybrid-graphics laptop where the host also uses the GPU [S-MICROVM-2026, S-FIRECRACKER-SPEC].

**Recommendation:** local-model inference / embeddings / (future) image-gen run as a **host-side broker service outside every sandbox**, reached via a mediated API surface — the sandbox holds no `/dev/nvidia*`; the broker enforces an API-shaped surface instead of an ioctl-shaped one. Direct device binding is reserved for **trusted-tier (C1-and-operator-owned) runs only**, if ever. This is the same control-plane-as-sole-egress shape used by srt and browser-use [S-BROWSERUSE, S-LANGCHAIN-SANDBOX]. Full local-model isolation detail is T15's.

### 3.6 Declarative per-worker policy as data (4.4, 9)

Express each worker template's confinement class as a **manifest field consumed by the sandbox launcher** (srt-style JSON + Kubernetes/Google-action-manifest precedent). Recommended shape:

```
confinement_class:  C0 | C1 | C2 | C3            # sets the defaults below
filesystem:
  workspace:        clone | none                  # reflink clone of registered store
  mounts:           [{path, mode: ro|rw|rw-no-delete}]   # rw-no-delete ships in Claude Cowork
  denyRead:         [<paths>]                      # credential stores ALWAYS denied (structural)
network:
  mode:             none | registries | fetch-broker | single-host
  allow:            [<hostnames>]                  # default-deny; firewall is the boundary
tools:              [<allowlist>]                  # default-deny per role
credentials:        [<named broker auth-profile refs>]   # never raw secrets; broker-mediated
outputs:            [{safe_output_type, max_cap}]  # gh-aw-style proposal caps
rule_of_two:        {untrusted_input, sensitive_data, external_effect}  # static gate
```

Every field has shipping precedent [S-SRT, S-K8S-SECCOMP, S-GOOGLE-SAIF, S-GHAW-SAFE, S-RULE-OF-TWO]. The `rule_of_two` field lets the control plane **statically refuse** a manifest declaring all three properties without a supervision gate — Meta's rule as an admission check. Credential-store deny must be **structural (empty-env, deny-by-default)**, not a scrub list: Claude Code's *own* local sandbox still reads `~/.ssh`/`~/.aws` and inherits env by default unless explicitly denied [S-ANTH-DOCS, primary] — Sinet, building fresh, inverts the default.

---

## 4. Recommendation for Sinet

**Primary approach: a composed, per-run, kernel-primitive sandbox — `systemd transient unit → bubblewrap → seccomp-BPF → Landlock` — with egress via a per-run netns + nftables default-drop + host allowlisting proxy, credentials held only in an ssh-agent-shaped host-side broker (never in the sandbox), and confinement class carried declaratively per worker.** Adopt Anthropic's open-source `sandbox-runtime` as the reference implementation / candidate dependency for the bwrap+seccomp+proxy core (Apache-2.0, no fork of any engine), and gate the confinement classes as:

| | **C0 connectors** | **C1 trusted reasoning** | **C2 workspace-write** | **C3 web-reading** |
|---|---|---|---|---|
| Model in loop | no | yes | yes | yes |
| Filesystem | none (host-side code) | ro workspace clone | rw workspace clone (reflink) + ro caches | rw workspace clone + ro caches |
| Network | 1 named service | **none** (no route) | registries allowlist (+ CDN hosts) | **fetch-broker host only**; raw web via broker |
| Credentials | 1 narrowest-scope key, broker-held | none | none in sandbox; git push = proposal via broker | none in sandbox |
| Output gating | deterministic; outward effects = proposals | — | every push/publish = proposal (4.2) | data-only; **tighter verification** of output |
| Isolation tech | egress pin + scoped key | bwrap+Landlock+seccomp, netns no-route | + registries proxy | + gVisor step-up for hostile input |

v0 ships C0–C2; v0.1 adds C3. The **cross-user boundary (D2's load-bearing one) is the per-user server + per-user XDG (spike S3) + per-user jail**; per-run classes layer on top.

**Why this and not the alternatives:** it is the exact stack both shipping first-party harnesses independently converged on (production-tier evidence, §2.1); every layer is a no-daemon kernel primitive a bus-factor-1 maintainer can reason about; it starts in milliseconds (fits seconds-lifetime runs); it composes cleanly on the already-adopted systemd baseline; and it needs **zero engine modification** (srt wraps processes externally; opencode/claude are configured, not forked) — satisfying adopt-don't-fork. The blast-radius guarantee it delivers — no credential access, no un-proposed outward effect, verification of steered output — **is** the 2026 consensus containment (§2.4), with Meta's Rule of Two as the citable admission check and gh-aw safe-outputs as shipping proof of the proposal-gate mechanism.

**What would change the decision:**

1. **A single-user run needs zero-trust isolation** (e.g., executing genuinely adversarial third-party code, not just reading hostile web content) → step that class up to **gVisor** (already the recommended C3-hostile tier) or, only if justified against the one-maintainer cost, a microVM.
2. **An engine won't tolerate model-egress credential injection** (cert-pinning) → fall back to §3.4-pattern-2 (scoped-egress-only) for that lane and record the residual exfil-via-model-endpoint risk explicitly.
3. **The Ubuntu 26.04 apparmor userns default or Landlock ABI probe comes back worse than expected on the actual host** → the bwrap layer needs the per-binary AppArmor profile (known fix) or the Landlock egress-port/UDP scoping is unavailable (fall back to nftables for all egress control, which is the boundary anyway).
4. **opencode gains native OS sandboxing** (issue #5529 lands) → reduce the per-user-jail workaround to configuration, and reconsider per-run confinement on that lane.

---

## 5. What NOT to use and why

- **firejail** — setuid-root by architecture (the exact model bubblewrap just *deprecated* for security), long escape history (CVE-2022-31214 userns→root, TOCTOU overlay races), and a 2025 emergency release signed by a substitute maintainer because the lead was unreachable for a month (bus-factor smell). bwrap does the same job without setuid. [S-FIREJAIL-CVE, S-FIREJAIL-REL]
- **TLS interception (MITM) as the default egress mechanism** — to allowlist by URL/path you must install a trusted CA in every sandbox, which breaks cert-pinned tools (`gh`, `gcloud`, `terraform`), enlarges the trust base, and forces the proxy to re-implement all of TLS correctly. None of C0–C3 as specified need path-level rules (they are host/registry/service scoped). Reserve MITM for the single engine→model-egress path where Sinet owns both ends. [S-ANTH-DOCS, S-IRONPROXY]
- **Network allowlist (hostname/proxy) treated as a security boundary** — CVE-2025-66479, the SOCKS5 null-byte bypass, DNS-tunnel exfil, DoH bypass, and CamoLeak-through-a-trusted-proxy all prove the string layer fails. Use it for *convenience*; make the **nftables default-drop the boundary**. [S-CVE-66479, S-KOUKYOSYUMEI, S-DNS-EXFIL, S-CAMOLEAK]
- **Denylists of any kind** for confinement — agents reason around them: `/proc/self/root/usr/bin/npx` defeated a path denylist; the dynamic linker (`ld-linux… /usr/bin/wget`) loaded a binary via mmap past an exec gate; an agent asked to *disable its own sandbox* when a namespace failed. **Allowlist-only mounts, tools, and egress.** [S-ONA-ESCAPE, S-TANAYSHAH]
- **App-level permission systems as the isolation boundary** (base opencode, Cline) — "if an agent can run bash or file tools, it inherits the OS permissions of the process." opencode's permission model and Cline's `.clineignore` are advisory; they are a useful *inner soft layer* under an OS jail, never the boundary. [S-OPENCODE-ISSUE, S-CLINE]
- **microVMs (Firecracker/Cloud-Hypervisor/QEMU) for v0** — strongest boundary, but the operational profile of a platform team (curate guest kernel + rootfs, jailer, TAP networking, virtiofs, a second kernel to patch) — wrong for bus-factor-1 at the seconds-lifetime scale, and Firecracker refuses the GPU anyway. Keep as the "step 2" if a class ever needs zero-trust. [S-MICROVM-2026, S-FIRECRACKER-SPEC]
- **GPU device-binding (`/dev/nvidia*`) into task sandboxes** — exposes the actively-exploited NVIDIA ioctl LPE surface (CVE-2025-33219) and the container-toolkit escape class (NVIDIAScape); use a **GPU broker service outside the sandbox** instead. [S-NVIDIA-LPE, S-NVIDIASCAPE]
- **Credential *scrubbing* instead of *exclusion*** — the productized default (Claude Code inherits env + reads credential dirs unless you deny each one) is the wrong default for D2. Start from **empty-env, deny-by-default**, so there is nothing to scrub and nothing to forget. This also closes the spike's hazard class: confinement must **never depend on anything the engine "remembers"** (resume without re-supplied `--settings` silently executed a parked call). [S-ANTH-DOCS, spike S2]
- **Trusting engine-inherited settings/config** — the S1 spike measured that both Anthropic surfaces leak operator-level settings into worker runs by default, and CVE-2026-25725 is exactly this class weaponized (in-sandbox code created a missing `.claude/settings.json` with a SessionStart hook that ran with host privileges on restart; also Copilot's `chat.tools.autoApprove` config-poisoning RCE, CVE-2025-53773). Mandate explicit `--settings`/`settingSources: []` on **every** managed invocation, deny writes to all settings paths (resolve symlinks), and treat agent-writable config as an attack surface. [S-CVE-25725, S-COPILOT-RCE, spike S1/S2]

---

## 6. Harvest-map verdicts

- **K2 — Default-deny permission service / "ToolPolicy" pattern → CONFIRM (with a boundary caveat).** Strong shipping prior art validates default-deny, forbid-overrides-permit tool authorization enforced *outside agent code and prompt reach* (Bedrock AgentCore/Cedar GA Mar 2026; OPA/Rego; MCP annotations as the risk vocabulary) [S-AGENTCORE, S-MCP-ANNOT]. **Caveat:** ToolPolicy is a *tool-authorization* layer, not an isolation boundary — the incident record shows agents reasoning around app-level controls, so it must sit **on top of** the OS sandbox (§5), never substitute for it. Adopt as the declarative per-worker tool-allowlist field (§3.6), default-deny.

- **gh-aw / G1 "safe-outputs" (read-only default, buffered writes, output caps, threat-detection pass before externalizing) → CONFIRM (strong — promote to a primary pattern).** This is the closest shipping implementation of Sinet's 4.2 + 4.7: read-only token, sandboxed egress-controlled agentic job → structured buffered action-requests → per-type caps/allowlists → a **separate threat-detection LLM pass** over agent output + patch + original intent → a **separate minimal-permission write job**, with config the agent cannot override [S-GHAW-SAFE, S-GHAW-THREAT]. Adopt the shape directly for the proposal pipeline; the threat-detection pass is the concrete mechanism for "C3 output receives tighter verification" (S5.4) and for the P-T06 two-axis verification of steered output. (Note: gh-aw is GitHub-Actions-shaped; Sinet ports the *pattern*, not the runner.)

- **C3-row / OpenClaw per-agent auth-profile stores (scoped token stores, no cross-run leakage) → REVISE.** The *goal* (per-owner credential isolation, no cross-run leak) is confirmed and required by D2, and the per-user XDG isolation that underpins it is measured-PASS (spike S3). **But revise the mechanism:** do not model it as "each agent/run gets its own token store the sandbox can read" — under D2 the store must live **outside** the sandbox, broker-mediated (§3.4, ssh-agent-shaped: operation-in/result-out, peer-credential attestation, destination constraints), because any credential the sandbox can read is exfiltratable by steered reasoning. The auth-profile becomes a **named reference** in the worker manifest that the broker resolves — the sandbox holds the reference, never the secret. Also note the spike's measured hazard: opencode stores access/refresh tokens **plaintext** in `auth.json` and its SQLite DB, so per-user stores need `chmod 0700` + backup-encryption regardless of the broker.

---

## 7. Open questions (for operator decision or later research)

1. **[G2 / spike] Engine model-egress credential injection.** Does `claude -p` (and `opencode serve`) tolerate a TLS-terminating local proxy injecting the subscription token on the model endpoint (§3.4-pattern-1), or does cert-pinning force the scoped-egress fallback (pattern-2)? This decides whether the engine's own credential can be kept fully out of the sandbox. *Owner: pre-implementation spike (one probe per lane), feeds G2.*
2. **[G2] Per-run confinement on the opencode lane.** Given `opencode serve` is per-user-persistent and sandboxes nothing itself, is per-user jail + per-run reflink workspace + app-level permission config sufficient for C3 (hostile web-reading), or should C3 runs on the opencode lane spin **ephemeral per-run `opencode serve` instances** inside per-run jails (paying the ~62 MB + network first-boot cost the S3 spike measured)? *Owner: operator decision informed by a C3 threat-model call.*
3. **[verify-on-host] Ubuntu 26.04 sandbox prerequisites.** Confirm on the actual laptop: `sysctl kernel.apparmor_restrict_unprivileged_userns` (and whether `bwrap-userns-restrict` ships/works on 26.04); the Landlock ABI level via a version probe (is ABI 8 TSYNC multithreaded enforcement present? ABI 10 UDP scoping?); whether the project-store volume is/can-be XFS or btrfs (ext4 kills reflink CoW). *Owner: operator, one afternoon of `sysctl`/probe checks before G2 finalizes tech.*
4. **[T15 bridge] GPU broker interface.** What API surface does the GPU-as-broker service expose to sandboxes (local-model inference, embeddings, future 12.1 image-gen), and does any class ever justify direct `/dev/nvidia*` binding? gVisor nvproxy on the Blackwell mobile RTX 5070 Ti is unverified — test `runsc nvproxy list-supported-drivers` if a sandbox-GPU path is ever wanted. *Owner: T15, with a hardware-compat spike.*
5. **[new platform problem — Known-problems list] Config-poisoning as a first-class threat.** CVE-2026-25725, CVE-2025-53773, and DuneSlide (CVE-2026-50548/9) are the same class: **an agent poisons its own config/guardrails to escape** (settings.json SessionStart hooks, `autoApprove`, working-directory-into-allowlist). Combined with the spike finding that engines leak operator settings and forget permission config on resume, this is a distinct problem the spec's Known-problems list should name: *"agent-writable configuration is an escape surface."* Mitigations (all from §5): explicit `--settings`/`settingSources:[]` every invocation, deny writes to all settings paths with symlink resolution, invocation-config re-supply on every resume, empty-env deny-by-default. *Owner: T14 (permission schema) + spec Known-problems update.*
6. **[new platform problem] The allowlisted-egress-is-an-exfil-channel residual.** Even a correct firewall+proxy leaves exfil through *legitimately allowed* endpoints (CamoLeak via GitHub's own proxy; Anthropic's own exfil-via-api.anthropic.com with attacker keys). For C2/C3 this is uncontained by egress control alone; the compensating controls are minimal reachable surface, HTTP-method restriction, and — for the model endpoint — the credential-injection proxy so an attacker's key can't ride the channel. *Owner: later research + T06 verification interplay.*
7. **[research contradiction — flag, not silent divergence]** None. This report is consistent with reports 01/02 and the G1 spikes; it *extends* spike S3's per-user-XDG-PASS into the cross-user isolation boundary claim, and it *operationalizes* the spike mandates (explicit settings isolation, no-engine-memory-dependence, plaintext-token `chmod 0700`) as confinement requirements.

**Honest caveat carried forward (S5.6), what remains uncontained even with the full stack:** (a) in-sandbox reasoning steering is unpreventable — injected content can still produce sabotaged code / poisoned analysis within the worker's powers, and verification gates are themselves probabilistic (adaptive attacks beat 12 defenses >90%); (b) exfiltration through allowed egress (above); (c) the human approver degrades (users approved ~93% of prompts; "when the user types the instruction, there's nothing anomalous to catch"); (d) persistent-memory/CLAUDE.md poisoning reloads every session; (e) below-trifecta harms (misinformation to the user, denial-of-wallet) are unaddressed by design. Sinet's guarantee is and remains **blast radius: no credential access, no un-proposed outward effect, and a verification gate over steered output** — containment, not immunity [S-CONTAIN, S-RULE-OF-TWO, S-ATTACKER-SECOND].

---

## 8. Sources

Evidence tiers: **P** primary (vendor doc / repo / advisory / spec / man page), **R** reputable independent research, **A** academic, **B** engineering blog / practitioner, **S** secondary/aggregator. All accessed **2026-07-17**.

**Agent-product sandboxing (§2.1–2.2)**
- [S-ANTH-BLOG] https://www.anthropic.com/engineering/claude-code-sandboxing — P — Claude Code sandbox: bwrap/Seatbelt + out-of-sandbox proxy, "credentials never inside the sandbox," fs+net inseparable, 84% prompt reduction (2025-10-20).
- [S-ANTH-DOCS] https://code.claude.com/docs/en/sandboxing — P — full sandbox config: default reads ~/.ssh/~/.aws + inherits env unless denied; `credentials` deny/mask + injectHosts; no-TLS-inspection default + domain-fronting caveat; managed lockdown keys; Bash-subprocess-only scope.
- [S-SRT] https://github.com/anthropic-experimental/sandbox-runtime — P — open-source srt: bwrap + seccomp (blocks AF_UNIX) + netns-removed + HTTP/SOCKS5 host proxies via socat; JSON schema; documented limitations (domain fronting, docker.sock).
- [S-CODEX-DOCS] https://learn.chatgpt.com/docs/sandboxing (from developers.openai.com/codex) — P — Codex modes read-only/workspace-write/danger-full-access; bwrap+Landlock+seccomp; network off by default; fail-closed refuse-if-unenforceable.
- [S-CODEX-CLOUD] https://learn.chatgpt.com/docs/cloud/internet-access — P — Codex cloud: internet blocked in agent phase; None/Common(~80 domains)/All presets; GET/HEAD/OPTIONS method restriction.
- [S-WILLISON-CODEX] https://simonwillison.net/2025/Nov/9/codex-sandbox-investigation/ — R — independent confirmation of Codex Seatbelt + Landlock/seccomp, no net by default.
- [S-OPENCODE-PERMS] https://opencode.ai/docs/permissions/ — P — opencode allow/ask/deny, tool list, denyRead/allowedDomains — app-level only, kernel-enforced by nothing.
- [S-OPENCODE-ISSUE] https://github.com/sst/opencode/issues/5529 — P — open FR: no OS sandbox; bash/file tools inherit process OS rights.
- [S-GEMINI-CLI] https://google-gemini.github.io/gemini-cli/docs/cli/sandbox.html — P — Gemini CLI sandbox off by default; docker/podman/sandbox-exec + Seatbelt profiles.
- [S-DOCKER-SANDBOX] https://docs.docker.com/ai/sandboxes/ — P — Docker Sandboxes: microVM-per-agent, pattern reference.
- [S-CLINE] https://agent-safehouse.dev/docs/agent-investigations/cline — R — Cline has no built-in sandbox; full local-user privileges.
- [S-CLOUD-CMP] https://northflank.com/blog/firecracker-vs-gvisor — B — isolation-tier ladder; E2B=Firecracker, Modal/Daytona/Claude.ai-web=gVisor.

**Sandbox technology (§3.1)**
- [S-BWRAP-REPO] https://github.com/containers/bubblewrap — P — userns/mountns/netns/seccomp mechanism; setuid deprecated 0.11.2 (CVE-2026-41163 setuid-only); `--overlay` flags since 0.11.0.
- [S-BWRAP-ADV] https://github.com/containers/bubblewrap/security/advisories/GHSA-xq78-7hw4-5jvp — P — CVE-2026-41163 setuid-only; userns mode unaffected; Codex bumped vendored bwrap to 0.11.2 (PR #21389).
- [S-LANDLOCK-MAN] https://man7.org/linux/man-pages/man7/landlock.7.html — P — ABI 1–8 mapping, cannot-restrict list, overlayfs-non-propagation.
- [S-LANDLOCK-KERNEL] https://docs.kernel.org/userspace-api/landlock.html — P — ABI capability detail incl. in-dev ABI 9–10 (UDP); TCP scoping = ABI 4.
- [S-LANDLOCK-NEWS] https://landlock.io/news/5/ — P — ABI 6=6.12, 7=6.15; adopters (Nomad, GNOME, landrun); ABI 8–10 kernel versions not pinned (verify-on-box).
- [S-SANDLOCK] https://arxiv.org/html/2605.26298v1 — A — Landlock+seccomp AI-agent sandbox, ~5ms startup, CoW workspaces.
- [S-NSJAIL] https://github.com/google/nsjail — P — nsjail 3.6 (Mar 2026), Kafel seccomp; maintained alt.
- [S-FIREJAIL-CVE] https://www.cvedetails.com/vulnerability-list/vendor_id-16191/Firejail-Project.html — P — firejail setuid-root escape history (CVE-2022-31214, TOCTOU overlay).
- [S-FIREJAIL-REL] https://github.com/netblue30/firejail/releases — P — 0.9.76 emergency release signed by substitute maintainer (bus-factor smell).
- [S-RUNC-ADV] https://github.com/opencontainers/runc/security/advisories/GHSA-9493-h29p-rfm2 — P — runc Nov-2025 escape trio (CVE-2025-31133/-52565/-52881); OCI-runtime escape class.
- [S-GVISOR-CVE] https://www.cvedetails.com/vulnerability-list/vendor_id-1224/product_id-69948/Google-Gvisor.html — P — 3 gVisor CVEs 2025, no public full escape.
- [S-MICROVM-2026] https://emirb.github.io/blog/microvm-2026/ — B — Firecracker 125ms/28ms-snapshot/no-GPU-by-policy; gVisor ~50ms & 53–68 host syscalls; VFIO-weakens-boundary; nvproxy>VFIO for GPU.
- [S-FIRECRACKER-SPEC] https://github.com/firecracker-microvm/firecracker/blob/main/SPECIFICATION.md — P — ≤125ms boot, ≤5MiB overhead, GPU excluded by design.
- [S-PODMAN-PERF] https://oneuptime.com/blog/post/2026-03-18-optimize-podman-container-startup-time/view — B — rootless +~0.45s first-container; native overlayfs > fuse-overlayfs.

**Workspace mechanics (§3.2)**
- [S-OVERLAYFS] https://docs.kernel.org/filesystems/overlayfs.html — P — upperdir/workdir constraints; lower-mutation undefined; userxattr unprivileged mode.
- [S-REFLINK] https://btrfs.readthedocs.io/en/latest/Reflink.html — P — reflink semantics: shared extents, independent files, same-fs only.
- [S-YOLOAI] https://github.com/kstenerud/yoloai — P — per-run CoW via reflink; **ext4 gets full physical copy** (no reflink).
- [S-DELTABOX] https://arxiv.org/abs/2605.22781 — A — DeltaBox: ms checkpoint/rollback on XFS-reflink overlays for agent sandboxes.
- [S-SYSTEMD-SANDBOX] https://wiki.archlinux.org/title/Systemd/Sandboxing — B/P — systemd-run directives (PrivateNetwork/PrivateUsers/BindReadOnlyPaths/IPAddressDeny/DynamicUser); `systemd-analyze security`.

**Egress control (§2.3, §3.3)**
- [S-INIT-FW] https://github.com/anthropics/claude-code/blob/main/.devcontainer/init-firewall.sh — P — reference iptables+ipset default-drop; dig→ipset + GitHub /meta ranges + verify step.
- [S-INNOQ] https://www.innoq.com/en/blog/2026/03/dev-sandbox-network/ — B — single-maintainer Squid CONNECT-only allowlist + nftables default-drop, no MITM.
- [S-SNAS] https://arxiv.org/pdf/2606.17533 — A — 4-layer egress defense; argues IP:port enforcement beats hostname strings.
- [S-IRONPROXY] https://github.com/ironsh/iron-proxy — P — single-host egress firewall; embedded DNS; upstream IP deny-CIDR closing SSRF/rebinding; optional MITM.
- [S-CILIUM-DNS] https://docs.cilium.io/en/stable/security/dns/ — P — toFQDN DNS-proxy pinning (eBPF pattern reference).
- [S-HARDENRUNNER-CVE] https://github.com/step-security/harden-runner/security/advisories/GHSA-46g3-37rh-v698 — P — CVE-2026-32947: DoH tunneling bypasses eBPF DNS allowlist.
- [S-CVE-66479] https://nvd.nist.gov/vuln/detail/cve-2025-66479 — P — srt empty-allowlist disabled network enforcement.
- [S-PENLIGENT] https://www.penligent.ai/hackinglabs/claude-code-sandbox-bypass/ — R — bypass catalog (null-byte/CRLF/IDNA); post-resolution deny 169.254/private CIDRs.
- [S-KOUKYOSYUMEI] https://medium.com/@Koukyosyumei/claude-codes-socks5-proxy-bypass-why-egress-filtering-must-happen-at-the-boundary-aaa445019e69 — R — SOCKS5 null-byte bypass; enforce at L3/L4, not proxy strings.
- [S-DNS-EXFIL] https://embracethered.com/blog/posts/2025/claude-code-exfiltration-via-dns-requests/ — R — CVE-2025-55284 DNS-tunnel exfil via allowed ping/dig/nslookup.
- [S-COPILOT-ALLOW] https://docs.github.com/en/copilot/reference/copilot-allowlist-reference — P — maintained npm/pypi/crates/apt/go/maven/nuget/rubygems + CDN allowlist.
- [S-PYPI-FASTLY] https://github.com/pypi/warehouse/issues/10399 — P — PyPI/RubyGems on Fastly; CDN-fronting gotcha for registry allowlists.

**Credential broker / C0 (§2.2, §3.4, §3.5)**
- [S-SSH-AGENT] https://man.openbsd.org/ssh-agent — P — operation-in/result-out; keys never leave agent.
- [S-SSH-RESTRICT] https://www.openssh.org/agent-restrict.html — P — destination-constrained agent keys (confused-deputy fix).
- [S-GIT-CRED] https://git-scm.com/docs/gitcredentials — P — credential-helper returns secret to git (the anti-model for D2).
- [S-SECRETLESS] https://github.com/cyberark/secretless-broker — P — broker holds secret; client never sees it.
- [S-MCP-AUTH] https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization — P — OAuth 2.1 RS; audience binding (RFC 8707); token pass-through forbidden; confused-deputy section; stdio-uses-env.
- [S-MCP-ANNOT] https://blog.modelcontextprotocol.io/posts/2026-03-16-tool-annotations/ — P — readOnly/destructive/idempotent/openWorld hints; hints are untrusted (bind policy to tool identity).
- [S-NEMOCLAW] https://github.com/NVIDIA/NemoClaw/issues/566 — P — host-side MCP bridge, keys from host env, 127.0.0.1, per-server scoped egress; "API keys never enter the sandbox."
- [S-CB4A] https://www.sans.org/blog/your-ai-agent-easily-confused-deputy-why-cloud-security-needs-credential-broker — R — PDP/CDP split, JIT DPoP-bound short-lived tokens, tiered approval, canaries.
- [S-STRIPE-KEYS] https://docs.stripe.com/keys-best-practices — P — one restricted key per component; recommended for AI agents.
- [S-GMAIL-SCOPES] https://developers.google.com/workspace/gmail/api/auth/scopes — P — gmail.send as narrowest send-only scope.
- [S-FORMAL] https://www.formal.ai/blog/using-proxies-claude-code/ — B — dummy-key + mitmproxy credential injection.
- [S-BROWSERUSE] https://browser-use.com/posts/two-ways-to-sandbox-agents — B — control-plane-as-sole-egress; credentials outside sandbox.
- [S-LANGCHAIN-SANDBOX] https://www.langchain.com/blog/the-two-patterns-by-which-agents-connect-sandboxes — B — sandbox↔service connection patterns.

**GPU (§3.5)**
- [S-GVISOR-GPU] https://gvisor.dev/docs/user_guide/gpu/ — P — nvproxy ioctl allowlist, pinned drivers, supported datacenter GPUs, residual driver risk, rootless breakage.
- [S-NVIDIA-LPE] https://www.esecurityplanet.com/threats/nvidia-gpu-driver-flaws-enable-privilege-escalation-across-platforms/ — S — Jan 2026 NVIDIA driver LPE bulletin.
- [S-CVE-33219] https://www.sentinelone.com/vulnerability-database/cve-2025-33219/ — P — kernel-module integer overflow via /dev/nvidia* ioctls, CVSS 7.8.
- [S-NVIDIASCAPE] https://nvd.nist.gov/vuln/detail/cve-2025-23266 — P — NVIDIAScape container-toolkit escape, CVSS 9.0, fixed 1.17.8.

**Prompt-injection blast radius (§2.4, §2.5)**
- [S-TRIFECTA] https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/ — P — lethal trifecta; "95% caught is a failing grade" (13 mo, flagged).
- [S-RULE-OF-TWO] https://ai.meta.com/blog/practical-ai-agent-security/ — P — Agents Rule of Two (at-most-2-of-3; needing all three → human-in-loop).
- [S-ATTACKER-SECOND] https://arxiv.org/abs/2510.09023 — P/A — adaptive attacks beat 12 defenses >90% (100% human red-team).
- [S-CAMEL] https://arxiv.org/abs/2503.18813 — A — CaMeL capability-tagged data-flow; 77% vs 84% utility (16 mo, flagged).
- [S-PATTERNS] https://arxiv.org/abs/2506.08837 — A — six design patterns for securing LLM agents (13 mo, flagged).
- [S-CONTAIN] https://www.anthropic.com/engineering/how-we-contain-claude — P — "contain at environment layer first"; Opus 4.7 5–6% injection after 100 attempts; residual-risk catalog (May 2026).
- [S-GHAW-SAFE] https://github.github.com/gh-aw/reference/safe-outputs/ — P — buffered structured outputs, per-type caps, sanitization, separate write job.
- [S-GHAW-THREAT] https://github.github.com/gh-aw/reference/threat-detection/ — P — separate threat-detection LLM job over output+patch+intent; skips write on detection.
- [S-AGENTCORE] https://aws.amazon.com/blogs/machine-learning/secure-ai-agents-with-policy-in-amazon-bedrock-agentcore/ — P — Cedar default-deny tool authorization at gateway (GA Mar 2026).
- [S-K8S-SECCOMP] https://kubernetes.io/docs/tutorials/security/seccomp/ — P — seccomp JSON profiles as data; allowlist semantics.
- [S-GOOGLE-SAIF] https://research.google/pubs/an-introduction-to-googles-approach-for-secure-ai-agents/ — P — action manifests + hybrid defense-in-depth (deterministic policy first).

**Incidents / escapes (§2.3, §5)**
- [S-CVE-25725] https://github.com/anthropics/claude-code/security/advisories/GHSA-ff64-7w26-62rf — P — CVE-2026-25725: in-sandbox settings.json creation → SessionStart hook host-priv escape; fixed 2.1.2.
- [S-COPILOT-RCE] https://embracethered.com/blog/posts/2025/github-copilot-remote-code-execution-via-prompt-injection/ — R — CVE-2025-53773: prompt-injected `chat.tools.autoApprove` config-poisoning RCE.
- [S-CURSOR-DUNE] https://thehackernews.com/2026/07/critical-cursor-flaws-could-let-prompt.html — R — Cursor DuneSlide CVE-2026-50548/9 (9.8): sandbox escape via working_directory/symlink.
- [S-ONA-ESCAPE] https://ona.com/stories/how-claude-code-escapes-its-own-denylist-and-sandbox — R — agent defeats denylist via /proc/self/root, dynamic linker; asks to disable own sandbox.
- [S-TANAYSHAH] https://tanayshah.dev/blog/agent-sandbox-runtime-hardening/ — B — allowlist-only mounts; layered seccomp; denylists fail against agents.
- [S-NX] https://socket.dev/blog/nx-packages-compromised — R — Nx s1ngularity: first malware to weaponize installed AI CLIs to harvest secrets.
- [S-MCP-GH] https://invariantlabs.ai/blog/mcp-github-vulnerability — R — public-issue injection → private-repo leak via broad PAT; "one repo per session" mitigation.
- [S-CAMOLEAK] https://obfuscated.site/blog/camoleak-github-copilot-exfiltration-cve-2025-59145 — R — CVE-2025-59145 (9.6): exfil through GitHub's own trusted Camo proxy.
- [S-REPLIT] https://www.theregister.com/2025/07/21/replit_ai_deletes_database/ — S — agent deleted prod DB during code freeze; "don't touch prod" is not a control.
- [S-APPARMOR-USERNS] https://discourse.ubuntu.com/t/understanding-apparmor-user-namespace-restriction/58007 — P — apparmor_restrict_unprivileged_userns mechanics; per-binary `userns,` profile fix.
- [S-JDHODGES] https://www.jdhodges.com/blog/codex-sandbox-ubuntu-24-04-fix/ — B — concrete bwrap-on-Ubuntu userns fix (2026-04).

**Cross-verification note:** the two most load-bearing angles (sandbox technology, agent-product sandboxing) were each independently researched by two separate search agents whose findings agreed on all material claims; discrepancies (Landlock ABI kernel-version at 7.0; exact SOCKS5-bypass patch version) are flagged inline as verify-on-box / non-load-bearing. Load-bearing claims rest on ≥2 independent sources except where explicitly flagged single-source (the 84% prompt-reduction figure is Anthropic-internal; several CVE writeups are the researcher's own single account, corroborated by NVD/GHSA where a CVE ID exists).
