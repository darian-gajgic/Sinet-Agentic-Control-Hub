# opencode adapter fixtures — provenance

Every `.sse` file in this directory reproduces RECORDED behavior of the pinned
engine (`opencode-ai@1.18.3`), never documentation (Spec S03.3 rule 4).

- **Live re-record, 2026-08-23**, on this host against the installed 1.18.3
  (`/home/sinep/.npm-global/bin/opencode`, `opencode --version` = `1.18.3` = the
  pin). A real `opencode serve` bound to `127.0.0.1` on an allocated port, under
  a disjoint XDG quartet, with `OPENCODE_SERVER_PASSWORD` set, driven over HTTP
  against a **loopback fake OpenAI-compatible provider** (`$0`, no credential,
  no provider network call). The v1 `GET /event` stream was captured verbatim;
  session/message/permission ids below are the real recorded ids.
- **SPIKE P2-S1 (2026-07-18)** — the live Z.AI park battery: `permission.asked`
  / `permission.replied` frame shapes, the always-class fan-out (one reply → N
  `permission.replied`, Probe 6), the v1-only permission-event fact (Probe 6-B),
  and the silent shutdown rejection (Probe 5).
- **SPIKE G1-S3 (2026-07-17)** — the OpenAPI surface at the pin (`GET /doc`),
  the `GET /session/status` `retry` variant, and the XDG containment result.

Frames are the exact `data: {...}` payload lines of the v1 SSE stream; `:` lines
are SSE comments (the engine's keepalive shape) and are skipped by the parser.

| file | what it records |
|---|---|
| `happy.sse` | one clean text-only turn: one paid call, `finish:"stop"`, `session.idle` |
| `tooltrun.sse` | a tool-using turn: TWO paid calls (`tool-calls` then `stop`), a completed tool part |
| `gate.sse` | the turn parks on `permission.asked` mid-tool-call — the stream stops there |
| `gate-replied.sse` | the frames that follow a `once` reply: `permission.replied`, tool completed, second paid call, idle |
| `fanout.sse` | SPIKE P2-S1 Probe 6: ONE reply fans out to THREE `permission.replied` events, one per requestID |
| `unknown.sse` | forward tolerance: an unknown type, a malformed line, a v2-generation frame, an SSE comment — the paid call and the idle still land |
| `retry.sse` | the `GET /session/status` `retry` variant on the bus (`attempt`/`next`/provider `action`) — a limit signal forwarded as DATA |
| `crash.sse` | the engine's own error report on an assistant message (`error` set, no `finish`) |
| `error-then-success.sse` | drain r1 D1 — an error mid-turn the turn RECOVERS from; COMPOSED from recorded shapes (crash.sse's error object on a message the engine also completes with its token row), not a fresh recording — see the inline header |
| `parallel-tools.sse` | drain r1 D5 — three bash calls in flight at once (SPIKE P2-S1 Probe 6 recorded three simultaneous parks); one sibling completing must not declare a safe boundary (P-T01-4) — see the inline header |
