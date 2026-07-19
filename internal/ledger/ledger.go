// Package ledger is the ledger module seam of Spec S01.1: the Task Context
// Ledger — the compiled, revisioned working context of a task, persisted as
// run_events revisions so the D7 checkpoint payload is self-contained (Spec
// S02.2; internal schema Spec S05).
//
// B0-3 creates the package boundary only, so the module-seam set of Spec
// S01.1 is explicit in the tree from the first shell build; the machinery
// lands with its owning phase, B2 (pipeline, Spec S05).
package ledger
