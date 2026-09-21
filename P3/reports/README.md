# Stage reports (rule R5 — capped chat reports, full report on disk)

Every pipeline stage agent replies in chat with at most 1,200 characters and writes its full report here as `P3-<phase>-<n>-<stage>.md` (`grounding`, `execute`, `evaluate`, `finalize-r<n>`), committed in the packet's worktree branch so it merges with the packet. Reports are evidence for the drain, the landing, and the phase-gate report; they are never an input to a later packet's grounding (code + spec are the only truth).

The evaluation and finalize reports also carry the **draft STATE landing line** (≤600 chars) and the **draft CONVENTIONS § text** the coordinator pastes at landing (rule R6) — the coordinator copies, it does not compose.
