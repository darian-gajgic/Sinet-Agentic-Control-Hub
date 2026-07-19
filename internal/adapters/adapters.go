// Package adapters is the adapter seam of Spec S01.3 (and the adapters
// module seam of S01.1): engine/provider specifics behind the D3 contract
// verbs, so a new engine or provider is one new adapter with orchestration,
// metering, and state untouched (Spec S03).
//
// B0-3 creates the package boundary only, so the seam is explicit in the
// tree from the first shell build; the first real adapter and lane land with
// the owning phase, B1 (execution substrate, Spec S03).
package adapters
