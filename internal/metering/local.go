package metering

import "encoding/json"

// local.go — the S12.1 local free-tier metering surface (brief R18/R19; §8
// readings 4+5). A local duty call (internal/local) writes ONE D7 checkpoint
// row on its consuming run whose usage block carries a `local` MARKER; the
// lane on that row is otherwise joined from runs (S02.2 — the run's lane,
// e.g. anthropic for a <task>.intake run). The fold + the pressure gauge
// HONOR the per-row lane override so the local call meters on the permanent
// free tier, not on the consuming run's paid lane.
//
// This is the READ side of a JSON WIRE CONTRACT written by internal/local
// (usage.go); neither package imports the other (the import wall, CONVENTIONS
// §26). The contract: {"input_tokens":N,"output_tokens":M,"local":{"lane":
// "local","duty":…,"model":…,"model_sha256":…,"engine_build":…}}.

// LaneLocal is the S03.2 local lane name (metering's copy of the wire-contract
// literal internal/local stamps into the marker — a string, not a shared
// symbol, keeping the import wall clean).
const LaneLocal = "local"

// localMarker is the `local` sub-object of a local call's D7 usage block.
type localMarker struct {
	Lane        string `json:"lane"`
	Duty        string `json:"duty"`
	Model       string `json:"model"`
	ModelSHA256 string `json:"model_sha256"`
	EngineBuild string `json:"engine_build"`
}

// parseLocalMarker returns the local marker when the usage block carries one
// with lane "local", else nil (a paid-lane row has no `local` field). Tolerant
// decode — the ledger is a read model over already-persisted rows.
func parseLocalMarker(usageJSON json.RawMessage) *localMarker {
	if len(usageJSON) == 0 {
		return nil
	}
	var w struct {
		Local *localMarker `json:"local"`
	}
	if err := json.Unmarshal(usageJSON, &w); err != nil {
		return nil
	}
	if w.Local == nil || w.Local.Lane != LaneLocal {
		return nil
	}
	return w.Local
}

// effectiveLane returns the lane a usage row meters on: the local marker's
// lane override when present, else the run-joined lane (§8 reading 4).
func effectiveLane(runLane string, usageJSON json.RawMessage) string {
	if m := parseLocalMarker(usageJSON); m != nil {
		return m.Lane
	}
	return runLane
}

// priceLocal prices a local-lane row at the permanent free tier's TRUE $0,
// labeled zero-allowance — NOT UNPRICED (brief R19; S12.1 "priced at $0 …
// zero-allowance receipt lines"). This is explicitly DISTINCT from the S10.3
// never-price-a-flat-lane-from-$0 guard (EffectiveDatedTable.Price), which
// targets PAID flat-rate lanes' engine-reported $0 rows and continues to bind
// them — a local row never reaches that guard because it never consults the
// price table (§8 reading 5; the conformance test pins both behaviors side by
// side).
func priceLocal(cur Currency, tableVersion string) PricedCost {
	return PricedCost{
		Unpriced:     false,
		CostUSD:      0,
		Currency:     cur,
		TableVersion: tableVersion,
		Source:       "local free tier / zero allowance (S12.1)",
	}
}

// ZeroAllowance reports whether a line item is a local free-tier line — the
// receipt/surface labels it "free tier / zero allowance" (R19). A $0 local
// line is distinct from an UNPRICED paid line (Unpriced() true).
func (l LineItem) ZeroAllowance() bool { return l.Lane == LaneLocal }
