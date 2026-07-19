## S11 — Sandboxing & confinement

**Scope:** The per-run sandbox that is D2's load-bearing isolation boundary — the composed kernel-primitive stack, the host-verified prerequisites it runs on, how the workspace and sanctioned sharing mount inside it, the credential broker and engine-credential injection that keep every secret outside it, the declarative confinement classes C0–C4 with their admission check, the two escape-surface problems this section owns, the deliberate privileged surface the unprivileged control plane needs, sandbox teardown, and the honest blast-radius invariant that closes the section.
**Binding inputs:** R10 (primary) [G2 D2.3]; SPIKE P2-S2 (host sandbox prerequisites + systemd harvest matrix), SPIKE P2-S3 (engine model-egress credential-injection wire probes); G2 D2.2 (`sandbox-runtime`, systemd-creds+sops/age adopted), D2.3 (sandbox stack ratified, host-contingent — now cleared); G3 Def.7 (privileged resume-remediation designed at spec time with T09 review); G1 Def.6 (systemd transient units), D1.3 (pause-and-flag, never auto-kill), rider 1 (settings-not-constants); feature list D2, 4.1, 4.2, 4.4, 4.7, S5.1–S5.6, S1.6, 12.1/12.2, 15.1, Operating reality; siblings `S01` (unit map, `sinet-broker`, run units, listener lint, sleep/wake reconcile) and `S02` §S02.10 (the DECIDED overlayfs+git-worktree workspace) + §S02.5 (recovery ladder / GC). Problems: P-T09-1, P-T09-2 (this section), P-T05-1, P-T13-1, P-T01-2.

*New terms coined here, defined on first use:* **per-run sandbox** (the composed jail wrapping one run's engine process); **credential-injection proxy** (the host-side TLS-terminating proxy on the pinned model-egress path); **auth-profile** (a named, broker-resolved credential reference held in a worker's control-plane record — never a secret); **run launcher** (`sinet-run@`'s fixed `ExecStart`).

### S11.1 The composed per-run sandbox stack

Every run executes inside a **per-run sandbox**: a composition of no-daemon kernel primitives layered on the adopted systemd baseline [R10 §4; G2 D2.3; G1 Def.6]. Composed outward-to-inward:

```
sinet-run@<run_id>.service   PID-1 time ceiling + cgroup accounting (the S10 cost backstop) [XREF:S01]
  └─ bubblewrap               user/mount/PID/UTS/IPC + empty per-run netns; workspace + ro-bind caches
       └─ seccomp-BPF          static allowlist profile (kills ptrace/process_vm_readv/odd execve)
            └─ Landlock         filesystem allowlist + TCP bind/connect scoping (ABI 8)
                 └─ engine      claude -p / opencode session — unprivileged, NoNewPrivileges
```

This is the exact stack both shipping first-party harnesses (Claude Code, Codex) independently converged on [R10 §2.1, §4]: it starts in single-digit milliseconds (fits seconds-lifetime runs), every layer is a kernel primitive a bus-factor-1 maintainer can hold in their head, and it needs **zero engine modification** (engines are configured, never forked). seccomp and Landlock are **defense-in-depth, not the boundary** — the boundary is the namespace isolation plus the network policy (S11.4) [R10 §2.1, §3.1].

**Adoption.** Anthropic's Apache-2.0 `sandbox-runtime` ("srt") is adopted for the **bwrap + seccomp + proxy core** [G2 D2.2; R10 §2.1]: it wraps any process (no container image, single-JSON config) and supplies the netns-removed + UDS-to-host-proxy egress shape used in S11.4. Adopt-don't-fork holds — srt wraps processes externally; the engines are configured. Its pin, replacement path, and abandonment criteria live in `components.lock` [XREF:S16].

**Two structural rules, from the escape record** [R10 §5]:

- **Allowlist-only, never denylist.** Agents reason around denylists (`/proc/self/root/usr/bin/npx`, the dynamic linker loading a binary past an exec gate). Mounts, tools, and egress are allowlist-only; there is no denylist to defeat.
- **Empty-env, deny-by-default — not scrub.** The productized default (Claude Code inherits the environment and reads `~/.ssh`/`~/.aws` unless each is denied) is the wrong default for D2. Sinet builds fresh from an empty environment with nothing bound, so there is nothing to scrub and nothing to forget [R10 §3.6, §5].

**Substrate granularity is load-bearing** [R10 §3.4]. `claude -p` is per-run → each invocation is wrapped in its own per-run sandbox. `opencode serve` is a per-user persistent HTTP server that sandboxes nothing itself → the whole server runs inside a **per-user** bwrap+netns jail scoped to that user's workspaces and model-egress, each session gets its own workspace (S11.3), and opencode's app-level `permission`/`allowRead`/`denyRead` config is an **inner soft layer only** — configured through S03's engine lowering [XREF:S03], never the boundary. The **cross-user boundary (D2's load-bearing one, since OS-user separation is deliberately omitted) is preserved on both lanes** by the per-user server + per-user XDG (spike G1-S3) + per-user jail; per-run OS-level isolation is strictly stronger on the claude lane.

### S11.2 Host prerequisites — verified on the v0 host

The D2.3 ratification contingency [G2 D2.3] is **discharged** for everything checkable without root, reboot, or a real suspend [SPIKE P2-S2]. The three blocking facts and what each settles:

| Prerequisite | Measured on the v0 host | What it enables / which fallback is now moot |
|---|---|---|
| **Unprivileged userns under AppArmor restriction** | `apparmor_restrict_unprivileged_userns=1` enforced (plain `unshare -U -r` blocked); **bwrap 0.11.1** at `/usr/bin/bwrap`; `/etc/apparmor.d/bwrap-userns-restrict` **ships and functions** (bwrap gets its userns, child capabilities stripped under label `bwrap//&unpriv_bwrap (enforce)`) | The full stack runs with **no fallback needed** — R10's "per-binary AppArmor profile" fix is shipped and proven, so that fallback is moot as a worry. **Rider (binding):** Sinet MUST invoke the *system* `/usr/bin/bwrap` (the profile attaches to that path), never a vendored or renamed copy, or ship its own `userns`-granting profile |
| **Landlock ABI level** | **ABI 8** — TSYNC multithreaded enforcement present (engine processes are multithreaded → the bar is met); **ABI 10 UDP scoping absent** | Landlock filesystem allowlist + TCP bind/connect scoping are available and used; Landlock cannot scope UDP at ABI 8 → **nftables remains the egress boundary anyway** (S11.4), which was always the design — the "nftables fallback" is simply the boundary |
| **Filesystem / reflink CoW** | Single ~420 GB **ext4** root, **no reflink**, 91% full (~39 GB free) | Reflink-CoW per-run workspaces are unavailable as-is → this is precisely why S02.10 **DECIDED overlayfs + git-worktree** rather than reflink; the sandbox mounts that choice (S11.3) [XREF:S02] |

**Remaining deferred host probes** (need root / reboot / a real suspend — operator-assisted, implementation phase): system-unit reboot survival of run-unit corpses; `ExitType=cgroup` exercised against the *real* bwrap+engine+children run wrapper (vs the synthetic probe); system-scope `systemd-run`/template-unit variants. `TBD-OPERATOR(sandbox host-probe close-out — system-scope run-unit + ExitType=cgroup-against-real-tree, batched with the S01/S02 suspend session)`. None changes the stack's shape; each only confirms a sub-behavior the S02 recovery ladder already tolerates.

### S11.3 Workspace composition inside the sandbox

S02.10 DECIDED the v0 workspace as **git-worktree + overlayfs on the existing ext4 volume** [XREF:S02]. This section specifies how that mounts *inside* the per-run sandbox; the workspace *lifecycle and GC policy* remain S02's.

- **Mount shape.** `lowerdir` = a **read-only shared project base** (a worktree of the registered project store, S1.6, shared across runs and never mutated during a run); `upperdir` = the **per-run writable diff**. The upperdir *is* the run's reviewable change set, aligning with the accept-work-as-attributed-commit flow (D9) [XREF:S13]. bwrap ≥ 0.11 wraps overlays natively (`--overlay`/`--ro-overlay`/`--tmp-overlay`); host bwrap 0.11.1 qualifies [R10 §3.2; SPIKE P2-S2].
- **Two overlay gotchas, both binding** [R10 §3.2]: (a) mutating the `lowerdir` while it is mounted is undefined behavior → the shared base is only ever updated **between** runs, by S02's GC/refresh, never by a live run; (b) **Landlock rules do not propagate through overlay layers** → every Landlock filesystem rule MUST target the **mounted overlay path**, not the lower or upper directories.
- **Sanctioned sharing → mounts** — the complete enumerated set of 4.1's exceptions, everything else isolated (D2):

  | Sanctioned share (4.1) | Mount |
  |---|---|
  | Read-only common caches (package caches, model weights) | `--ro-bind` (`BindReadOnlyPaths=`). A **writable** cross-run cache is the poisoning anti-pattern (one compromised run seeds the next); where a tool insists on writing, give it a shared ro lower + a per-run throwaway `--tmp-overlay` upper [R10 §3.2] |
  | The registered project store via workspace-clone (S1.6) | the overlay above — shared worktree base `lowerdir` + per-run `upperdir` [S02.10] |
  | Resources a project explicitly shares | mounted per the worker's declarative `mounts` list, `ro` or `rw-no-delete` [R10 §3.6; XREF:S08] |
  | Everything else | **isolated** — not bound; the credential stores are **structurally denied** (empty-env, deny-by-default), never reachable [R10 §3.6; D2] |

### S11.4 Egress: the firewall is the boundary, the proxy is convenience

The real egress boundary is a **per-run empty network namespace backed by a host-level nftables default-DROP**; the allowlisting proxy is a **convenience layer**, not a security boundary [R10 §2.3, §3.3, §4]. Composition:

1. **Per-run empty netns** (created unprivileged inside bwrap; loopback only, no route) — the primary default-drop: with no route, nothing egresses. Classes that need egress reach out **only through a bind-mounted Unix-domain socket** to a host-side proxy (srt's netns-removed shape) — the sandbox holds no IP route to exfil through [R10 §3.3].
2. **Host-level nftables default-DROP + IP deny-CIDRs** — the standing boundary that backs the host proxy and any residual path: cloud-metadata (`169.254.169.254`) and RFC1918 ranges are dropped, closing SSRF/rebinding to the host and metadata service. Installed once at boot by a privileged unit (S11.8), never per-run [R10 §2.3].
3. **Host-side allowlisting proxy** — hostname allowlisting by SNI / HTTP CONNECT with **no TLS interception**. This is **convenience only**; it decides *which* allowed host, it does not *contain*.
4. **Block outbound DoH resolvers; restrict HTTP methods** where the class allows (GET/HEAD/OPTIONS for read-only fetch) [R10 §2.3].

**Why the proxy is not the boundary — the CVE record that is the basis for this posture** [R10 §2.3, §5]: CVE-2025-66479 (an empty allowlist read as allow-all disabled srt's proxy for ~30 releases); the SOCKS5 null-byte bypass (`attacker.com\x00.google.com` passed the string check, ~5.5 months); CVE-2025-55284 (DNS-tunnel exfil via auto-approved `ping`/`dig`); CamoLeak / CVE-2025-59145 (exfil through GitHub's *own trusted* proxy); harden-runner CVE-2026-32947 (DoH tunneling defeated an eBPF DNS allowlist). Every one proves hostname/string allowlisting fails as a boundary → it must sit **behind** the firewall's IP:port default-drop.

**TLS interception is OFF by default** — installing a trusted CA in every sandbox breaks cert-pinned tools, enlarges the trust base, and forces the proxy to re-implement TLS [R10 §5]. The **one** justified MITM is the engine→model-egress path, where Sinet owns both ends (S11.5).

Per-class egress (feeds the S11.6 table): **C0** = one named service host + IP deny-CIDR guard; **C1 / verification runs** = empty netns, **no proxy at all** (strongest and simplest); **C2** = default-drop + proxy allowlisting package-registry hostnames **and their CDN hosts** (the Fastly fan-out gotcha — `files.pythonhosted.org`, `static.crates.io`; an on-host caching mirror can later narrow C2 to one internal host); **C3** (v0.1) = **no raw egress**, only one fetch-broker host that fetches on the sandbox's behalf and returns results as data [R10 §3.3].

### S11.5 Credentials outside the sandbox — the broker and engine injection

**D2 invariant (MUST):** no credential ever enters a task sandbox; the sandbox is *the* isolation boundary (there is no OS-user separation) [D2; R10 §2.2]. Two credential kinds are handled distinctly: **owner secrets** (git, provider connectors, outward-effect channels) via the broker, and **the engine's own model-endpoint credential** via injection.

**The broker** (`sinet-broker.service`; S01 owns the unit, S11 the internals) is an **ssh-agent-shaped** host-side daemon on a per-user UDS: the sandbox submits a *typed operation*, the broker attests the caller by **UDS peer credentials (SO_PEERCRED)** — authenticate the asker, never hand it the secret — evaluates policy, executes with the owner's credential **outside** the sandbox, and returns **only the result** [R10 §2.2, §3.4; S01.2]. It is never modeled on git-credential helpers (which return the secret — the anti-model). Three invariants are adopted: per-credential **destination constraints** (ssh-agent), **never pass tokens through** + **audience binding** (MCP spec MUSTs), and a policy-decision/credential-delivery split kept as an internal code boundary (the decision path holds zero secrets) [R10 §3.4]. Every worker holds only **auth-profile** references (named, broker-resolved) in its control-plane record — never raw secrets [XREF:S08]. Outward-effect operations (push/publish/send) route to the 4.2 gated-proposal flow and the broker performs signing/pushes only on approval [XREF:S13] — the broker is the single choke point to log, revoke, and rate-limit. **Secrets at rest** use systemd-creds + sops/age [G2 D2.2]; the broker signing key and any per-user store are `0700`, host-only, never in any sandbox (opencode stores tokens plaintext in `auth.json` — the store is encrypted at rest regardless) [R10 §6; SPIKE P2-S3 §5].

**Engine credential injection — resolved wire-side.** The engine (`claude -p`, `opencode serve`) needs *its own* subscription credential to reach the model endpoint, and adopt-don't-fork forbids splitting engine auth from tool execution. **Pattern-1** keeps it out of the sandbox: the sandbox holds a **sentinel**; a **credential-injection proxy** (TLS-terminating, per-process-trusted CA) substitutes the real subscription token **only** on the pinned model-egress request [R10 §3.4]. **P2-S3 proved pattern-1 VIABLE on both v0 lanes** — `claude` 2.1.214 and `opencode` 1.18.3 neither cert-pins its model endpoint against a per-process-trusted CA (`NODE_EXTRA_CA_CERTS`; Bun-compat honors it too); a Z.AI **401→200 purely from proxy-side substitution** was demonstrated end-to-end [SPIKE P2-S3]. Therefore **the engine's own credential is kept fully outside the task sandbox on both lanes at v0**; R10 §4 decision-changer #2 (cert-pinning → pattern-2 scoped-egress) does **not** fire for either lane, and the pattern-2 fallback is not required at v0. Binding wire facts:

- **Inject on the model path, not the host** — `claude` fans out to ~8 ancillary `api.anthropic.com` paths (bootstrap, oauth, mcp-registry, event_logging, eval). Inject the real token **only** on `/v1/messages` (Anthropic) / `/api/coding/paas/v4/chat/completions` (Z.AI), auth header `Authorization: Bearer`, and pass every other request untouched — injecting on telemetry/oauth endpoints is needless secret exposure [SPIKE P2-S3].
- **Per-process trust only** — the injection CA is Sinet-owned and trusted per-process (`NODE_EXTRA_CA_CERTS`); the system trust store is never touched. This does not weaken the engines against an *untrusted* MITM (they correctly reject that); pattern-1's security rests on the CA being Sinet-owned [SPIKE P2-S3].
- **One choke point, two purposes** — because the engine tolerates termination, the same proxy harvests the `anthropic-ratelimit-unified-*` headers as provider-signaled observed state (D4-clean, no window modeling), enriching the S10 park-timing and consumption meters [XREF:S10; SPIKE P2-S3].
- **Pin-regression canary (P-T01-2)** — the per-substrate conformance suite asserts, per engine version, that a trusted-CA terminating proxy on the model path still yields a 200, so a future engine release introducing cert-pinning is caught at upgrade time, not in production; if a lane ever regresses, `⚙ sandbox.model_egress_tls_terminate = false` for that lane falls back to pattern-2 (scoped-egress-only: the subscription credential sits in the sandbox but egress is pinned to the model host and no other credential/effect is present) [SPIKE P2-S3; XREF:S14].

### S11.6 Confinement classes as declarative data

A **confinement class** is a rung C0–C4 of the isolation ladder (S5), **declared per worker and carried declaratively** in the control-plane tables — behavioral content lives in template files, but **all enforcement state (class, grants, egress, credentials) lives exclusively in control-plane tables and is recompiled every run** (the guardrail split; workers can never alter it — 14.2) [R10 §3.6; XREF:S08]. The class is compiled into the sandbox launcher's parameters, srt-style: `filesystem` (workspace clone/none, `mounts` ro|rw|rw-no-delete, `denyRead` — credential stores **always** structurally denied), `network` (mode none|registries|fetch-broker|single-host + hostname allow-list, default-deny), `tools` (default-deny per role), `credentials` (auth-profile refs only), `outputs` (proposal-type caps), and `rule_of_two`.

**Rule-of-Two admission check.** Meta's Agents Rule of Two: within a session an agent may hold **at most two of** {processes untrusted input, accesses sensitive data/systems, can change state or communicate externally}. The control plane **statically refuses** a worker whose declared record asserts all three without a supervision gate (human-in-the-loop, i.e. the 4.2 proposal path, or another reliable validation) [R10 §2.4, §3.6]. A run may safely *transition* between two-property phases when the transition breaks the attack chain. This is a compile-time gate on the worker record, enforced outside agent code and prompt reach.

**Per-class concrete profiles** (v0 ships **C0–C2**; C3 at v0.1, C4 future — [G2 D2.3; XREF:S19]):

| Class | Model in loop | Filesystem | Network egress | Credentials | Output gating | Isolation tech |
|---|---|---|---|---|---|---|
| **C0** connectors | no | none (host-side deterministic code) | one named service host + IP deny-CIDR | one narrowest-scope key, **broker-held** (auth-profile) | deterministic; outward effects = proposals (4.2) | egress pin + scoped key |
| **C1** trusted reasoning | yes | **ro** workspace clone | **none** — empty netns, no route, no proxy | **none** | — | bwrap + seccomp + Landlock, no-route netns |
| **C2** workspace-write | yes | **rw** overlay workspace (upper=diff) + ro caches | registries allowlist (+ CDN hosts) via proxy | none in sandbox; git push = proposal via broker | every push/publish = proposal (4.2) | + registries proxy behind host default-drop |
| **C3** web-reading *(v0.1)* | yes | rw overlay workspace + ro caches | **fetch-broker host only**; raw web via broker, returned as data | none in sandbox | data-only; **tighter verification** of output | + **gVisor step-up** for hostile input |
| **C4** web-acting *(future, 12.2)* | yes | disposable profile | per-site scope | none in sandbox | per-action approval; full action log | tightest; nothing graduates in silently |

The threat-detection pass over agent output + patch + original intent (gh-aw safe-outputs shape) is the concrete mechanism for C3's "tighter verification" and for two-axis verification of steered output [R10 §2.5, §6; XREF:S07].

**Plan-declared class flows to helpers tighter-only (P-T05-1).** A coordinator's approved plan declares the run's confinement class; a spawned helper **inherits the coordinator's class and may only be tightened, never loosened** — the admission check rejects any helper spawn requesting a looser class. S11 owns the *mechanics* (the class is a control-plane field; the check is a compile-time comparison at spawn); the *policy* — which class attaches to which plan or stage — is [XREF:S06] §S06.6 [P-T05-1; XREF:S08].

### S11.7 Two owned escape-surface problems

**P-T09-1 — agent-writable configuration is an escape surface** (CVE-backed). CVE-2026-25725 (in-sandbox `settings.json` creation → SessionStart hook running with host privilege on restart), CVE-2025-53773 (Copilot `chat.tools.autoApprove` config-poisoning RCE), and DuneSlide/CVE-2026-50548/9 (working-directory/symlink escape) are one class: an agent poisons its own config or guardrails to escape — sharpened by the spike finding that engines leak operator settings and forget permission config on resume [R10 §5, §7-5]. **S11's mitigations (filesystem/sandbox side):** deny writes to **all** settings/config paths with **symlink resolution** (resolve, then deny — the DuneSlide vector); empty-env deny-by-default so there is nothing to scrub; and confinement **never depends on anything the engine "remembers"** (a resume that did not re-supply settings silently executed a parked call). This problem's **cousins are owned next door**: S03's engine-lowering channel checklist closes the config *channels* (`settingSources:[]`, explicit `--settings`, invocation-config re-supply on every resume) [XREF:S03], and S01's listener-binding lint closes the config-drift-as-escape channel at the process boundary [XREF:S01]; T14/S08 owns the permission schema the class compiles from [XREF:S08]. S11 owns the sandbox-side deny + empty-env; together they close the class. Registered in S17.

**P-T09-2 — allowlisted egress is an exfil channel** (residual). Even a correct firewall+proxy leaves exfil through *legitimately allowed* endpoints — CamoLeak through GitHub's own proxy; exfil-via-`api.anthropic.com` with an attacker key [R10 §7-6]. This is **uncontained by egress control alone** for C2/C3. Compensating **posture** (not a fix): minimal reachable surface (fewest allowed hosts); HTTP-method restriction (GET/HEAD/OPTIONS where the class allows); an on-host caching mirror to narrow C2 toward one internal host; and, for the model endpoint, the S11.5 injection proxy so an attacker holding only a sentinel cannot ride the channel with a real key. **Monitoring hook:** egress volume/pattern anomaly detection registers into the watchdog suite — the machinery is S14's; S11 supplies the posture and the hook [XREF:S14]. Honest disposition: bounded, not closed → S11.10. Registered in S17.

### S11.8 The privileged surface — G3 Def.7 discharge (T09-informed)

Def.7 requires the privileged resume-remediation path to be *designed at spec time with T09 review* [G3 Def.7]. The unprivileged `sinet` control plane needs two privileged capabilities: **(a)** start/stop **system-scope** run units, and **(b)** post-resume network remediation (restart `tailscaled` when wedged, re-assert `tailscale serve` — P-T13-1, S01.7). The design goal, applying T09's own principles (allowlist-not-denylist, no setuid, single-choke-point auditability [R10 §3.4, §5]), is the **smallest fixed-verb surface** that delivers both.

**Why system scope at all.** The per-run sandbox itself is **fully unprivileged** — bwrap under the shipped AppArmor profile, an *empty* netns, seccomp and Landlock all compose without privilege (S11.1–S11.4), and egress reaches a host proxy over a bind-mounted UDS, so run units carry no per-run root. System scope (not `--user`) is chosen only for **io-controller delegation, unit-level AppArmor confinement, and independence from the (degraded) user manager** [SPIKE P2-S2] — and starting a *system* unit is polkit-gated even when the unit runs `User=sinet`.

**Exact privileged verbs (the complete grant — nothing else):**

| Verb | On unit | Purpose |
|---|---|---|
| `start` / `stop` / `reset-failed` | `sinet-run@<run_id>.service` (system scope, runs `User=sinet`) | launch / park / cancel / GC a per-run sandbox |
| `start` | `sinet-netremediate.service` (root oneshot, fixed `ExecStart`) | post-resume `tailscaled` restart + `tailscale serve` re-assert (P-T13-1) |

Explicitly **not** granted: `manage-unit-files` (install/edit/enable/mask), `reload-daemon`, generic `StartTransientUnit` with caller-set properties, and start/stop of any unit outside those two names. The host nftables default-drop + proxy (S11.4) are installed once at boot by a root `sinet-egress-setup.service` that `sinet` cannot trigger — standing config, outside the on-demand grant.

**Two shapes compared:**

| | Shape A — broad polkit on transient units | **Shape B — fixed-`ExecStart` template unit + narrow polkit grant (CHOSEN)** |
|---|---|---|
| Mechanism | grant `sinet` `StartTransientUnit`/manage-units, scoped by unit-name pattern | root-install `sinet-run@.service` (fixed hardening + fixed `ExecStart=/usr/lib/sinet/run-launch %i`, `User=sinet`) + `sinet-netremediate.service`; grant `start`/`stop`/`reset-failed` on just those names |
| Least privilege | **FAILS** — polkit sees the unit *name* but not the *properties*; the caller sets `ExecStart`/`User`/hardening, so `sinet` could start `sinet-run-x` with `ExecStart=/bin/sh` as root | **HOLDS** — the privileged command line is fixed in the shipped unit; `sinet` supplies only *data* (the compiled confinement record at `/run/sinet/jobs/<run_id>.json`, read and schema-validated by `run-launch`), never properties |
| setuid | none | none — systemd units, not a setuid binary (avoids the firejail anti-pattern [R10 §5]) |
| Auditability | polkit + journald | polkit + journald **+ every trigger emits an event on the platform event log** [XREF:S14] |
| Verdict | REJECT | **ADOPT** [coordinator-draft] |

**Chosen (Shape B).** The run unit is realized as a **root-installed template `sinet-run@<run_id>.service`** whose `ExecStart` (the **run launcher**) is fixed and non-agent-reachable; the control plane, as unprivileged `sinet`, writes the per-run confinement record to a `sinet`-owned spool file and `start`s the instance via a **single scoped polkit rule** authorizing `org.freedesktop.systemd1.manage-units` for exactly `^sinet-run@.*\.service$` and `sinet-netremediate.service`. The launcher validates the spool record, composes the S11.1 stack (unprivileged), and execs the engine. Network remediation is a fixed-verb root oneshot: `sinet-netremediate.service` health-checks `tailscaled`, `systemctl restart tailscaled.service` if wedged, re-asserts `serve`, and writes its outcome to the event log — `sinet` can *trigger* it but never parameterize it, so there is no generic "restart tailscaled" grant. The decisive least-privilege argument is that **polkit cannot constrain transient-unit properties** — only fixing the `ExecStart` in a shipped unit makes a name-scoped grant safe.

*Reconciliation with S01.2* [coordinator-draft]: S01.2 describes run units as `systemd-run` transients named `sinet-run-<id>`; S11 refines the realization to a **template instance** `sinet-run@<id>.service` precisely so the polkit grant is property-safe. Functionally identical to S01's intent (per-run, ephemeral, `systemctl`-visible, `Restart=no`, own journal identity via `%i`) — S01.2 wording aligned at assembly (2026-07-19).

**G4 note.** This is the deliberate new-privileged-surface decision Def.7 demands. The recommended surface is the two-unit / one-polkit-rule design above; G4 acknowledges it as the sole privileged grant to the `sinet` user, with the run units themselves unprivileged.

### S11.9 Cleanup and teardown as a sandbox property

Teardown is part of what the sandbox *is*; S11 owns namespace/netns/upperdir teardown, S02 owns the workspace lifecycle/GC *policy* and the recovery ladder [XREF:S02].

- **Run teardown.** On run end / park / cancel, `stop sinet-run@<id>` (via the S11.8 grant) tears the sandbox down with the unit: the empty **per-run netns is destroyed with the unit**, and the bwrap process tree dies with the cgroup — the lane recipe `RemainAfterExit=yes` + `ExitType=cgroup` + `Type=exec` (never `--collect`) ensures the *tree*, not merely the sandbox binary exiting, is what the reconcile pass reads as dead [SPIKE P2-S2].
- **Overlay upperdir disposal.** The per-run `upperdir` is either **captured** (its diff committed/harvested as the deliverable, D9/S13) or **discarded**; disposal is linked to S02's workspace GC — an orphan worktree/upperdir holding **uncommitted** work is flagged, **never auto-deleted** [S02 §S02.5; XREF:S02].
- **Corpse GC.** After harvest the unit corpse is `reset-failed` (S11.8 grant) — the run-unit side of the S02.5 recovery-ladder GC step [XREF:S02].
- **Orphan handling.** A sandbox whose control plane died is reconciled by the S02 recovery ladder (ALIVE/WEDGED/FINISHED-DURING-OUTAGE/DEAD), never by PID-1 auto-restart (`Restart=no`); a WEDGED run is **paused-and-flagged, never auto-killed** (D1.3) [XREF:S02; S01.2].

### S11.10 Blast radius — the closing invariant

Injected content can still **steer a model's reasoning inside its sandbox** — that risk is **not fully removable** by any confinement, and the verification gates that judge steered output are themselves probabilistic (adaptive attacks beat 12 published defenses at >90%, 100% under human red-teaming) [R10 §2.4, §7; 4.7]. What remains uncontained even with the full stack, stated honestly: (a) in-sandbox reasoning steering (sabotaged code / poisoned analysis within the worker's own powers); (b) exfil through allowed egress (P-T09-2); (c) human-approver degradation (~93% approval rates; nothing anomalous to catch when the user typed the instruction); (d) persistent-memory/CLAUDE.md poisoning that reloads every session; (e) below-trifecta harms (misinformation, denial-of-wallet) [R10 §7].

**The guarantee — this section's closing invariant (MUST):** Sinet's confinement delivers **blast radius, not immunity — no credential access, no un-proposed outward effect, and a verification gate judging potentially steered output.** Containment, not immunity. This is the mid-2026 consensus posture (Rule of Two + "contain at the environment layer first" + "the attacker moves second"), not a hedge [4.7; S5.6; R10 §2.4, §4, §7].

---

**Settings introduced (⚙):** (all operator-editable with audit trail per G1 rider 1; auto-adjust only within operator ceilings)

| Name | Default | Clamp / range | Ratified by |
|---|---|---|---|
| `sandbox.egress_deny_cidrs` | {`169.254.169.254/32`, `169.254.0.0/16`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7`} | editable list; the metadata IP `169.254.169.254/32` is a non-removable floor [coordinator-draft] | R10 §2.3/§3.3 |
| `sandbox.block_outbound_doh` | `true` | {true, false}; false requires a recorded reason | R10 §2.3 |
| `sandbox.c2_registry_allowlist` | curated npm/pypi/crates/apt/go/maven/nuget/rubygems + their CDN hosts (Copilot/Codex preset) | editable list (data, like the price table) | R10 §3.3 |
| `sandbox.model_egress_tls_terminate` | `true` (per lane) | {true, false}; false ⇒ pattern-2 scoped-egress fallback for that lane | SPIKE P2-S3 / R10 §3.4 |

The seccomp-BPF profile, the Landlock ruleset, and the per-class profile defaults (S11.6) are **structural**, not ⚙ — versioned in `components.lock`/the worker schema, not operator dials.

**Known problems owned here:**
- **P-T09-1** — agent-writable configuration is an escape surface → sandbox-side deny-writes-to-config-paths (symlink-resolved) + empty-env; channel-closing cousins in S03 (engine lowering) and S01 (listener lint); schema in S08 (S11.7).
- **P-T09-2** — allowlisted egress is an exfil channel → uncontained by egress control alone; posture = minimal surface + method restriction + injection proxy; anomaly-monitoring hook in S14; honest residual in S11.10 (S11.7).
- **P-T05-1** (filed by T05, *mechanics here*) — plan-stage confinement must bind helpers → class is a control-plane field; admission check refuses looser-than-coordinator helper spawns; policy in S06.6 (S11.6).
- **P-T13-1** (filed by T13, *privileged path designed here*) — post-resume network reconcile → the fixed-verb `sinet-netremediate.service` triggered under the S11.8 grant; detection/duty in S01.7 (S11.8).
- **P-T01-2** (filed by T01, *extended here*) — engine schema/behavior drift → the per-substrate conformance suite gains a model-egress-MITM-tolerance canary so a cert-pinning regression is caught at upgrade (S11.5).

**Deferred / parked:**
- **C3 web-reading** (fetch-broker host + gVisor step-up) → v0.1 [G2 D2.3; S00.2]; the ladder is specified now so the v0 tech choice is not reversed to add it.
- **C4 web-acting** → future (12.2); parked behind the 15.3 gate [S00.2].
- **gVisor / microVM step-up** for a single-user run needing zero-trust isolation → re-entry only if a class must execute genuinely adversarial code, not merely read hostile web content [R10 §4].
- **Per-run veth + per-run nftables** (finer per-run egress than the host-level default-drop + UDS-proxy) → post-v0 option; would add a privileged per-run setup step [R10 §3.3].
- **GPU-in-sandbox** (12.1 future image-gen) → GPU work stays behind the T15 broker service outside every sandbox; direct `/dev/nvidia*` binding is never used at v0 (NVIDIAScape/CVE-2025-33219 ioctl LPE surface) [R10 §3.5; XREF:S12].
- **opencode native OS sandboxing** (issue #5529) → if it lands, reduce the per-user-jail workaround to configuration and reconsider per-run confinement on that lane [R10 §4].
- **Deferred host probes** (system-scope run-unit + real-tree `ExitType=cgroup`, reboot survival) → `TBD-OPERATOR`, batched with the S01/S02 suspend session [SPIKE P2-S2].

**Coverage:** (Scope → subsection)

| Feature-list item | Where |
|---|---|
| S5 confinement ladder (C0–C4) + S5.6 honest caveat | S11.6, S11.10 |
| 4.1 isolation with enumerated exceptions | S11.3 |
| 4.4 minimal powers per worker | S11.6 (declarative class, default-deny) |
| 4.7 untrusted content hostile; blast-radius guarantee | S11.10 |
| 4.2 outward effects only as proposals (no send/publish/push in sandbox) | S11.5, S11.6 |
| D2 credentials never in task sandboxes; sandbox = the boundary | S11.5 (broker + injection), S11.1 |
| S1.6 registered store → workspace clone | S11.3 |
| 12.1/12.2 future GPU / web-acting (sockets left) | Deferred/parked |
| 15.1 v0 ships C0–C2; sandbox stack; broker | S11.1, S11.5, S11.6 |
| G3 Def.7 privileged resume-remediation path | S11.8 |

**Open items for G4:** none open. Three drafting-time sub-choices are flagged inline as `[coordinator-draft]` for G4 attention: (1) the **privileged-surface design** (S11.8 Shape B — the deliberate new-privileged-surface decision Def.7 demands; recommended as the sole `sinet` grant, run units unprivileged — G4 note in S11.8); (2) the **run-unit realization** as a template instance `sinet-run@<id>.service` refining S01.2's `systemd-run` wording (coordinator reconciles at assembly); (3) the `sandbox.egress_deny_cidrs` non-removable-floor choice. The two problem ids **P-T09-1 / P-T09-2** are assigned here (the G2 follow-up list named them descriptively) for S17 to adopt.
