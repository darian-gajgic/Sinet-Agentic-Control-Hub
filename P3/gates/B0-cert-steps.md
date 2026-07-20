# B0 gate — first cert + front chain on `sinet.tailfd0b1e.ts.net`

Operator runbook, documented at P3-B0-5 for gate execution. **EXECUTED 2026-07-20 at the B2 gate (D2.2) — see the Execution record at the end**; the step-7 Caddyfile below is the corrected version applied to the host. It performs the two host-side steps the spec staged for this gate: the first TLS cert on the A1 hostname and the S01.4 front chain (`tailscale serve` → Caddy → `sinet-control` on 127.0.0.1). Spec anchors: S01.4 (chain shape, `/events` unbuffered, HTTP/2 at serve), S01.8 (hostname `sinet` = amendment A1; observables register), S01.9 (identity headers), S16.3 (Caddy row: pin + path-scoped license recorded at onboarding).

Command syntax below was verified against the host's installed CLI (`tailscale` **1.98.8**, checked 2026-07-19, read-only). Ports carry no spec meaning (S01.4): this doc uses **8482** for `sinet-control` (the existing dev default) and **8481** for Caddy.

**Before you start — two acknowledgements:**

- **Step 4 publishes `sinet.tailfd0b1e.ts.net` to the public Certificate Transparency logs, permanently** (S01.8 register rows 1 and 3). This is by design and why the bland hostname was chosen first. Operator item 7 (observables-register sign-off) covers exactly this; sign it no later than first deploy.
- The tailnet must have **MagicDNS + HTTPS Certificates enabled**: https://login.tailscale.com/admin/dns → "HTTPS Certificates" → Enable. `tailscale serve` fails with a clear message if this is off.

## A. First cert + serve, verified against the bare API

1. Confirm the machine name is `sinet` (settled 2026-07-19):

   ```
   tailscale status --self | head -1
   ```

   Expected: the first line shows `sinet` and its tailnet IP. If not, stop — the A1 hostname must be live before any cert exists.

2. Start `sinet-control` if it is not already running. At this gate it may still be the dev-mode invocation (the chain does not care how control is started; after the unit install is approved, `http_addr = 127.0.0.1:8482` moves into `/etc/sinet/bootstrap.conf`):

   ```
   ./sinet control --http-addr 127.0.0.1:8482
   ```

   Expected: log line `http: serving loopback-only addr=127.0.0.1:8482`, then `sinet-control: ready`.

3. Confirm the API answers locally:

   ```
   curl -s http://127.0.0.1:8482/api/health
   ```

   Expected: `{"ready":true,"mode":"running",...}`.

4. **The cert step.** Point `tailscale serve` at the backend in the background (persists across tailscaled restarts). The first HTTPS request makes tailscaled obtain the Let's Encrypt cert for `sinet.tailfd0b1e.ts.net` automatically — no key files land on disk, TLS terminates inside tailscaled (S01.4):

   ```
   sudo tailscale serve --bg 8481
   ```

   Expected output names `https://sinet.tailfd0b1e.ts.net/` proxying to `http://127.0.0.1:8481`. (8481 is Caddy's future port — configured now so the chain shape never changes; until section B is done, requests will 502, which is fine and expected. If you prefer to verify serve before Caddy exists, temporarily use `sudo tailscale serve --bg 8482` and re-run this step with 8481 after B.)

   Note: `tailscale cert sinet.tailfd0b1e.ts.net` also exists (verified in the host CLI) but is NOT used — it writes cert/key files to disk for self-terminated TLS, which this chain deliberately avoids.

5. Inspect the serve config:

   ```
   tailscale serve status
   ```

   Expected: `https://sinet.tailfd0b1e.ts.net (tailnet only)` → `http://127.0.0.1:8481`. "tailnet only" is the D1 wall — never enable `funnel`.

## B. Caddy onboarding + the full chain

6. Install Caddy from the Ubuntu 26.04 archive (adopted organ, own unit, own journal identity — S01.2):

   ```
   sudo apt install caddy
   caddy version
   ```

   Expected: a version string. **Record it** — the S16.3 Caddy row's `TBD-P3(pin at onboarding)` resolves to exactly this version, and the coordinator must add the `components.lock` entry (pin + path-scoped Apache-2.0 license check at that version + funeral plan per S16.4) in the same sitting. The lock entry is a coordinator task, not an operator command; flag it when reporting this runbook done.

7. Replace `/etc/caddy/Caddyfile` with the router config (routing only — TLS stays at serve, assets stay in the control binary, S01.4):

   ```
   {
   	# TLS terminates at tailscale serve (Spec S01.4); Caddy is a plain-HTTP
   	# loopback router and must never fetch certs of its own.
   	auto_https off
   }

   # Any-host site on 8481: tailscale serve forwards the original ts.net Host
   # header, so the site address must not host-match; bind pins the listener
   # to loopback (P-T13-2: the only non-loopback listener on this host is
   # tailscaled).
   http://:8481 {
   	bind 127.0.0.1

   	# The one SSE endpoint rides unbuffered (Spec S01.4: "/events unbuffered").
   	@sse path /events
   	handle @sse {
   		reverse_proxy 127.0.0.1:8482 {
   			flush_interval -1
   		}
   	}
   	handle {
   		reverse_proxy 127.0.0.1:8482
   	}
   }
   ```

   *(Corrected at execution — the originally documented `http://127.0.0.1:8481` site address bound `*:8481`: a Caddy site-address host is host-**matched**, not bound, and would also have missed requests carrying the forwarded ts.net Host header. See the Execution record.)*

8. Apply and confirm Caddy binds loopback only (P-T13-2: the only non-loopback listener on this host stays `tailscaled`):

   ```
   sudo systemctl reload caddy
   ss -tlnp | grep -E '8481|8482'
   ```

   Expected: both ports bound on `127.0.0.1` only.

## C. Verification through the chain

9. Health through the full chain — run from any tailnet device (phone browser works: `https://sinet.tailfd0b1e.ts.net/api/health`):

   ```
   curl -s https://sinet.tailfd0b1e.ts.net/api/health
   ```

   Expected: the same health JSON as step 3. The first run of this step is what mints the cert — allow a few extra seconds once.

10. **Identity-header check** (S01.4 injection → S01.9 parse). From a tailnet device:

    ```
    curl -s https://sinet.tailfd0b1e.ts.net/api/auth/session
    ```

    Expected: `"hint":{"device_login":"<your tailscale account login>"}` in the response — proof that serve injects `Tailscale-User-Login` and the platform parses it. `authenticated` is `false` until step 11.

11. **First real login** (this is also the production household bootstrap — the users table starts empty). Pick your operator id and PIN:

    ```
    curl -s -X POST -H 'Content-Type: application/json' \
      -d '{"user_id":"<you>","display_name":"<Name>","role":"operator","pin":"<PIN>"}' \
      https://sinet.tailfd0b1e.ts.net/api/auth/users
    curl -s -c /tmp/sinet-jar -X POST -H 'Content-Type: application/json' \
      -d '{"user_id":"<you>","pin":"<PIN>"}' \
      https://sinet.tailfd0b1e.ts.net/api/auth/login
    ```

    Expected: `{"user_id":"<you>"}` then a login body with a 30-day `expires`; the jar now holds the `sinet_session` cookie (Secure, HttpOnly, SameSite=Lax).

12. **SSE through the chain, unbuffered** (session required in production — that is the S01.9 wall working):

    ```
    curl -s -N -m 5 -b /tmp/sinet-jar 'https://sinet.tailfd0b1e.ts.net/events?after_seq=0'
    ```

    Expected: `id:/event:/data:` frames arrive immediately (`platform.started`, `auth.user_created`, `auth.login`, …), not after a buffer delay; the same URL without the cookie returns 401.

13. Optional observation for the register record: within a day, `https://crt.sh/?q=sinet.tailfd0b1e.ts.net` shows the issuance — register rows 1 and 3 now exist in the world, as signed off under operator item 7.

## Deferred beyond this gate (spec-staged)

- **systemd unit installs** for the sinet unit set (`sinet units` output) — a separate B0-gate approval item; this runbook works against a dev-mode control process.
- **Preview subdomain routes via Caddy's admin API** — S13, arrives at B4; today's Caddyfile deliberately contains no admin-API usage.
- **SPA serving through this chain** — the assets embed into the control binary at B6 (S01.5); nothing to configure here when they land.
- **P-T13-1 post-wake serve health-check + remediation path** — S01.7 duty, designed with S11's privileged-surface work (B1+).
- **Device grants** (trusted auto-login for personal devices) — machinery is live now via `POST /api/auth/grants`, but granting is a deliberate per-device operator decision; the default for every device is shared (PIN required), and nothing at this gate requires any grant.

## Execution record — 2026-07-20 (B2 gate, D2.2; coordinator-driven, operator sudo window + explicit CT-acknowledgement "go")

Steps 1–12 executed and verified (same evening); step 13 due within a day. Findings and deviations, in execution order:

- **Pre-step-4 discovery:** a stale Nexus-era serve entry (`/` → `https+insecure://127.0.0.1:8777`, dead backend — nothing listening) had survived the 2026-07-19 machine rename. Cleared with `sudo tailscale serve reset` before configuring 8481.
- **The cert predates this gate.** Step 9 answered in 15 ms — TLS was already warm. Inspection: issued **2026-07-19 21:27:21 UTC** (Let's Encrypt, CN/SAN `sinet.tailfd0b1e.ts.net`, notAfter 2026-10-17, auto-renewed inside tailscaled). The leftover serve config minted it the moment the renamed FQDN went live — CT publication therefore happened 2026-07-19, a day before the operator's item-7 signature, which covers it retroactively (signed 2026-07-20, this session). STATE's 2026-07-19 "nothing is in CT logs yet" was already stale minutes after it was written. MagicDNS + the HTTPS-Certificates toggle were confirmed on (the pre-existing cert proves the latter).
- **Step 7 defect, fixed in place:** the documented Caddyfile produced a **wildcard** listener (`*:8481`) and would have host-match-missed the forwarded ts.net Host header. Corrected to `http://:8481` + `bind 127.0.0.1` (the block above is the applied config); `ss` re-verified both 8481/8482 loopback-only (P-T13-2 ✓). Recorded in the Caddy lock entry too.
- **Step 6 done:** caddy **2.6.2-14** from Ubuntu resolute/universe (apt install simulation-vetted first: +libnss3-tools only, nothing removed/upgraded). S16.3 `TBD-P3(pin at onboarding)` + path-scoped license resolved in the `components.lock` organ-unit entry — Apache-2.0 per the installed package's Debian copyright record (one vendored file dual Apache-2.0/BSD-3-Clause, a Go-stdlib borrow).
- **Posture changed mid-execution (benign, coordinated):** steps 2–3 ran against a coordinator-started dev-mode process (`~/.local/state/sinet`, dev-default 8482) until the **parallel D2.1 session** installed the production units and cleanly SIGTERM'd it; the chain was then re-verified end-to-end against `sinet-control.service` (`/var/lib/sinet`, `bootstrap.conf` → 127.0.0.1:8482). Production-posture results match the runbook exactly: step 9 health ✓; step 10 `{"authenticated":false,"hint":{"device_login":"dariannixda@gmail.com"}}` ✓ (serve injection + S01.9 parse proven); `/events` without a cookie → **401** ✓. The superseded dev leftovers (`~/.local/state/sinet` empty bootstrap DB, `~/.local/bin/sinet` scratch binary) were left in place at operator preference — note `~/.local/bin` precedes `/usr/local/bin` on PATH, so a stale `sinet` CLI resolves to the scratch binary until it is removed.
- **Serve config final state:** `https://sinet.tailfd0b1e.ts.net (tailnet only)` → `http://127.0.0.1:8481` (Caddy) → `127.0.0.1:8482` (the unit). "tailnet only" confirmed — funnel never enabled (D1 wall).
- **Steps 11–12 (production household bootstrap), with one slip caught and fixed:** operator user `darian` (role operator) created — but the create landed with the runbook's placeholder PIN still in the payload (curl-editing confusion; the operator's subsequent login with their real PIN returned invalid credentials). Diagnosed by a coordinator probe login against the placeholder; **rotated immediately** via `POST /api/auth/pin` (step-up re-prompt + rotation in one operator-run chain; SetPIN invalidates all other sessions, killing the probe session), re-login with the real PIN verified. Step 12 verified live: SSE through the chain with the session cookie streams immediately (frames id:1…: platform lifecycle, `auth.user_created`, both `auth.login`s, `auth.reprompt(pin_set)`, `auth.pin_set` — the full S01.9 every-auth-act-is-an-event trail); cookie-less request → 401 (verified earlier in production posture). Runbook CLOSED except the step-13 crt.sh observation (due within a day).
