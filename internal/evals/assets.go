package evals

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// Per-asset eval-object binding (Spec S14.8 ¶2): every worker template version
// and every rubric version resolves to its eval set — golden cases (25–50,
// grown from real outcomes) plus a planted-defect suite the judge MUST fail.
//
// The v0 software content is the EXISTING artifacts: verify.SeedGoldenSet's 20
// planted cases ARE the planted-defect suite and its 6 clean controls are the
// TNR controls, judged under verify.SeedSoftwareRubric. No case content is
// invented here; growth stays on the ratified paths (S07.10 incident refresh,
// S08.6 dry-run pair seeding). The research-domain planted-citation suites are
// v0.1 and are NOT built (the entailment calibration set stays idle, S07.4/A8).

// SetRef is a concrete (eval-set id, version) pair — what an eval-hook ref
// resolves TO.
type SetRef struct {
	ID      string
	Version int
}

func (r SetRef) String() string { return fmt.Sprintf("%s@v%d", r.ID, r.Version) }

// EvalHooks mirrors the S08.1 template eval-hook refs across the import wall
// (internal/evals never imports internal/worker).
type EvalHooks struct {
	GoldenSetRef     string
	PlantedDefectRef string
}

// WorkerAsset is one worker template version as the catalog sees it, supplied
// by the WorkerHooksFunc seam.
type WorkerAsset struct {
	TemplateID string
	VersionID  string
	Version    int
	Hooks      EvalHooks
}

// WorkerHooksFunc resolves a worker template version id to its asset record.
// Satisfied at the composition root from internal/worker (the FlagByModelFunc
// seam precedent, CONVENTIONS §28); nil ⇒ worker binding is unavailable.
type WorkerHooksFunc func(ctx context.Context, versionID string) (WorkerAsset, error)

// EvalObject is an asset version bound to its eval set (Spec S14.8 ¶2).
type EvalObject struct {
	AssetKind    string
	AssetID      string
	AssetVersion int
	// GoldenSet / PlantedSuite are the resolved (set id, version) pairs. At v0
	// they name the same case set: the planted cases of the golden set ARE the
	// planted-defect suite (Spec S14.8 ¶2's code classes — test-adequacy
	// defects and known-bad artifacts).
	GoldenSet    SetRef
	PlantedSuite SetRef
	// GoldenCases / PlantedCases / CleanCases are the resolved case counts.
	// PlantedCases is the TPR denominator, CleanCases the TNR denominator.
	GoldenCases  int
	PlantedCases int
	CleanCases   int
}

// Catalog binds asset versions to their eval objects.
type Catalog struct {
	// WorkerHooks is the S08.1 eval-hook lookup seam.
	WorkerHooks WorkerHooksFunc
}

// Rubric binds a rubric version to its eval object: the domain's golden set,
// whose planted cases are the suite the judge must fail (Spec S14.8 ¶2/¶5).
func (c *Catalog) Rubric(r *verify.RubricBundle) (EvalObject, error) {
	set, ok := setForDomain(r.Domain)
	if !ok {
		return EvalObject{}, fmt.Errorf("%w: no golden set for domain %q", ErrUnknownEvalSet, r.Domain)
	}
	obj := bind(AssetRubric, r.ID, r.Version, set)
	obj.PlantedSuite = obj.GoldenSet
	return obj, nil
}

// WorkerVersion binds a worker template version to its eval object by resolving
// the S08.1 GoldenSetRef / PlantedDefectRef hooks to concrete (set, version)
// pairs. A version whose hooks are empty inherits its domain's set — the eval
// obligation is per-asset, so an unhooked worker is bound, never exempt.
func (c *Catalog) WorkerVersion(ctx context.Context, versionID, domain string) (EvalObject, error) {
	if c.WorkerHooks == nil {
		return EvalObject{}, fmt.Errorf("%w: worker eval-hook seam not wired", ErrUnknownEvalSet)
	}
	a, err := c.WorkerHooks(ctx, versionID)
	if err != nil {
		return EvalObject{}, err
	}
	domainSet, ok := setForDomain(domain)
	if !ok && (a.Hooks.GoldenSetRef == "" || a.Hooks.PlantedDefectRef == "") {
		return EvalObject{}, fmt.Errorf("%w: no golden set for domain %q", ErrUnknownEvalSet, domain)
	}
	golden, err := resolveRef(a.Hooks.GoldenSetRef, domainSet)
	if err != nil {
		return EvalObject{}, err
	}
	planted, err := resolveRef(a.Hooks.PlantedDefectRef, domainSet)
	if err != nil {
		return EvalObject{}, err
	}
	obj := bind(AssetWorker, a.TemplateID, a.Version, golden)
	obj.PlantedSuite = SetRef{ID: planted.ID, Version: planted.Version}
	return obj, nil
}

// bind fills an eval object's counts from the resolved golden set.
func bind(kind, id string, version int, set *verify.GoldenSet) EvalObject {
	obj := EvalObject{
		AssetKind: kind, AssetID: id, AssetVersion: version,
		GoldenSet:   SetRef{ID: set.ID, Version: set.Version},
		GoldenCases: len(set.Cases),
	}
	for _, c := range set.Cases {
		if c.DefectClass == cleanClass {
			obj.CleanCases++
		} else {
			obj.PlantedCases++
		}
	}
	return obj
}

// cleanClass is the golden-set label for a clean control (verify's seed
// vocabulary): the TNR controls, as opposed to the planted defects.
const cleanClass = "clean"

// setForDomain returns the in-tree eval set for a domain. Only the software
// domain has one at v0 (Spec S07.8/S19 launch-domain roster); web-research
// planted-citation suites are v0.1.
func setForDomain(domain string) (*verify.GoldenSet, bool) {
	set := verify.SeedGoldenSet()
	if domain != set.Domain {
		return nil, false
	}
	return set, true
}

// resolveRef resolves an eval-hook ref ("golden-software" or
// "golden-software@2") to a concrete (set, version) pair. An empty ref falls
// back to the asset's domain set; a ref naming an unknown set or a version the
// in-tree set is not at fails loudly (a dangling ref never resolves to
// something that does not exist).
func resolveRef(ref string, fallback *verify.GoldenSet) (*verify.GoldenSet, error) {
	if strings.TrimSpace(ref) == "" {
		if fallback == nil {
			return nil, fmt.Errorf("%w: empty ref and no domain set", ErrUnknownEvalSet)
		}
		return fallback, nil
	}
	id, version, hasVersion := strings.Cut(ref, "@")
	set := verify.SeedGoldenSet()
	if id != set.ID {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEvalSet, ref)
	}
	if hasVersion {
		n, err := strconv.Atoi(strings.TrimPrefix(version, "v"))
		if err != nil {
			return nil, fmt.Errorf("%w: %q has a malformed version", ErrUnknownEvalSet, ref)
		}
		if n != set.Version {
			return nil, fmt.Errorf("%w: %q is not the in-tree version (v%d)", ErrUnknownEvalSet, ref, set.Version)
		}
	}
	return set, nil
}
