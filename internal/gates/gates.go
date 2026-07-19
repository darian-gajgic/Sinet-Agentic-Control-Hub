// Package gates is the gates module seam of Spec S01.1: the D7
// checkpoint-and-gate machinery — ask/approval gating and the two-phase
// effect journal that makes outward effects exactly-once (Spec S02.4, S02.7).
//
// B0-3 creates the package boundary only, so the module-seam set of Spec
// S01.1 is explicit in the tree from the first shell build; the machinery
// lands with its owning packet, P3-B0-4 (run FSM + checkpoint/effect journal
// + recovery ladder, Spec S02).
package gates
