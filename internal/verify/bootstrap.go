package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// The bootstrap verification posture [A14, 2026-08-27] — Spec S07.8. A
// launch-domain deliverable whose registered project has NO captured
// build/test/lint command (every fresh scaffold's first task) is never a
// verification refusal and never parks the run. V0 runs unchanged; V1 runs the
// S07.3 stage-contract checks with every executable-ladder rung recording
// UNVERIFIABLE-HERE rather than being skipped or reported as PASS; V2 runs
// with its verdict advisory and visibly marked non-authoritative; requester
// review is mandatory at every stakes tier.
//
// Bootstrap is COMPUTED from the registry's current capture, never a default
// for an unwired seam: a launch domain whose pack machinery is simply not
// there stays the loud ErrNoCheckPack refusal, and a capture the platform
// cannot read stays ErrBadPack. Honest absence still fails loud; what changes
// is that "this project has nothing to run yet" is an absence with a defined
// posture rather than an outage.

// Posture names the verification posture a round ran under (Spec S07.8). The
// empty posture is the full one: the domain's check pack executed.
type Posture string

// PostureBootstrap marks a round whose launch domain had no executable rung to
// run [A14, 2026-08-27]. Its V2 verdict is advisory and non-authoritative, so
// no code path may mark its work verified or deliver it without a human act.
const PostureBootstrap Posture = "bootstrap"

// BootstrapAttribution is the stable attribution marker on every record the
// bootstrap posture writes — the ladder rungs' outcomes and the PLAN steps'
// contracts alike. It names the absent substrate rather than an upstream
// check, which is exactly what ContractUnverifiable means when "the deciding
// substrate is not wired" (Spec S07.3).
const BootstrapAttribution = "check-pack:absent"

// BootstrapPostureNote is the requester-facing disclosure the posture carries
// onto the round record, the round's findings, every card terminal raised
// under it, and the receipt (Spec S07.8: "the verdict card and receipt name
// the bootstrap posture in plain words, including that capturing the project's
// commands restores the full ladder"). Plain words for a person, never an
// error chain, and it promises no door it does not have.
const BootstrapPostureNote = "This project has no build, test or lint command captured yet, so the checks that would prove this work correct could not run. Nothing was passed off as checked: every check rung is recorded as unverifiable here, the judge's verdict is advisory only, and your review is what decides this work. Capturing the project's commands restores the full ladder from the next revision on."

// BootstrapPack is the check-pack resolution for a registered project whose
// capture holds no executable rung [A14, 2026-08-27]. It carries no checks by
// construction — bootstrap invents nothing on a project's behalf — and it is
// deliberately NOT a valid pack: Validate still refuses a pack without checks,
// and the drain branches on the posture instead of running it.
func BootstrapPack(domain string, version int) *CheckPack {
	return &CheckPack{Domain: domain, Version: version, Posture: PostureBootstrap}
}

// bootstrap reports whether p is the bootstrap resolution.
func (p *CheckPack) bootstrap() bool { return p != nil && p.Posture == PostureBootstrap }

// executes reports whether p has rungs to run — the condition a CheckRunner is
// required for. A bootstrap resolution has none.
func (p *CheckPack) executes() bool { return p != nil && len(p.Checks) > 0 }

// packPosture names the posture a round resolving to pack runs under.
func packPosture(pack *CheckPack) Posture {
	if pack.bootstrap() {
		return PostureBootstrap
	}
	return ""
}

// bootstrapPostureFinding carries the posture into the round's findings, from
// where it reaches the requester on the review surface (through reviewable's
// posture exemption), the judge's prior-findings scope, and the durable round
// record. Note severity by construction — a permanent blocker would drive
// REVISE to the cap and park the run, which is the wall S07.8 abolishes —
// under the CHECK-INTEGRITY category (the ratified reading: it is a fact about
// the suite, like the quarantine-skip note). That category's card raiser fires
// only on blockers, so the disclosure spams no inbox, and ComputeVerdict
// excludes the category from its note count, which is why the drain states the
// advisory SHIP-with-notes downgrade itself.
func bootstrapPostureFinding() Finding {
	return Finding{
		Severity: SeverityNote,
		Category: CatCheckIntegrity,
		Anchor:   BootstrapAttribution,
		Text:     BootstrapPostureNote,
	}
}

// bootstrapPostureKey is the disclosure's stable identity (criterion + anchor
// + category). It is fixed, so the note is the SAME finding in every round it
// is raised in — which is what both exemptions below key on, by IDENTITY and
// never by category.
var bootstrapPostureKey = bootstrapPostureFinding().Key()

// isPostureDisclosure reports whether f is the bootstrap posture disclosure.
//
// Two rules exempt exactly this finding, and nothing else:
//
//   - the S07.6 new-note suppression (validateFindings). Suppression exists to
//     stop goalposts drifting round by round; the posture is not a new goalpost
//     but a fact about how THIS round was verified, and which round the
//     bootstrap posture first appears in must never decide whether the
//     requester is told about it.
//   - the review-stream strip (reviewable). Suite defects stay out of the
//     deliverable's review channel because regenerating the deliverable cannot
//     fix a broken check; the posture is the requester's answer to "why was
//     nothing checked", and the review surface is exactly where the mandatory
//     V3 decision is made (Spec S07.8). It rides as a note, so it still
//     triggers no rework round.
func isPostureDisclosure(f Finding) bool { return f.Key() == bootstrapPostureKey }

// bootstrapV1 is the V1 result of a bootstrap round (Spec S07.8).
//
// The posture is not execution-less [A16, 2026-09-17]. Commands the platform
// DETECTED in the produced tree (Spec S13.7's re-scan, carried here as a pack
// with ProvenanceDetected) are RUN, cheap-first over the ladder, inside the
// same network-off verification sandbox a captured pack's rungs use — and
// their verdicts are derived platform-side from the exit status, never from
// anything the command says about itself (Spec S07.3 rule 3). They are
// EVIDENCE and nothing more: they carry no frozen-criterion key and no PLAN
// step id (rule 4), they raise no finding of their own (Spec S07.5: a finding
// citing no criterion can only be a note), and they never graduate the
// project — the posture, the mandatory review and the advisory verdict are
// unchanged by every one of them passing.
//
// A ladder stage no detected rung covers still records UNVERIFIABLE-HERE under
// the absent-command placeholder, so nothing is silently skipped and no stage
// is reported twice.
//
// Every PLAN step's "Done when" contract is then DECIDED from the tree — the
// files the work produced — as far as those files can decide it: write-set
// globs, named files, structural facts [A16, 2026-09-17]. Only a contract no such
// fact reaches keeps the absent-pack attribution, now with the reason
// recorded. A refuted contract raises one blocker so it reaches a person
// (Spec S07.7); coverage is the approved plan's AC map, which decides the
// criterion that blocker cites (Spec S06.6/S07.5).
func bootstrapV1(ctx context.Context, pack *CheckPack, runner CheckRunner, req CheckRequest, steps []intake.Step, coverage map[string][]string) V1Result {
	tree := req.Workspace
	res := V1Result{Findings: []Finding{bootstrapPostureFinding()}}
	if pack != nil {
		res.PackVersion = pack.Version
		res.PackVerifiedOn = pack.VerifiedOn
	}
	res.Checks = detectedRungs(ctx, pack, runner, req, tree)
	idx, walkErr := indexTree(tree)
	for _, s := range steps {
		sc := decideFromTree(s, idx, walkErr)
		res.Steps = append(res.Steps, sc)
		if sc.State == ContractFail {
			res.Findings = append(res.Findings, contractFinding(sc, s, coverage))
		}
	}
	return res
}

// postureDetail appends the disclosure to a card's detail lines so every
// terminal raised under the bootstrap posture names it (Spec S07.8).
func postureDetail(detail []string, p Posture) []string {
	if p != PostureBootstrap {
		return detail
	}
	out := make([]string, 0, len(detail)+1)
	out = append(out, detail...)
	return append(out, BootstrapPostureNote)
}

// ---- The detected execution rung [A16, 2026-09-17] ----

// hostResidentInterpreters names the command interpreters the verification
// sandbox already contains.
//
// STRUCTURAL, not a ⚙ setting: it describes what the sandbox profile's
// read-only bind set actually holds (every class ro-binds /usr and /bin), and
// Spec S11 declares sandbox profiles structural and versioned in code
// (CONVENTIONS §12). An operator turning this into a number could only make
// the platform lie about its own mount table. Flagged to the settings-tab
// ledger as an interim constant.
var hostResidentInterpreters = map[string]bool{
	"node": true, "npm": true, "npx": true, "sh": true, "bash": true,
}

// detectedDepsDir is the directory an npm script's own tooling resolves from.
// Structural for the same reason: it is a property of the npm toolchain, not a
// preference anybody could hold a different opinion about.
const detectedDepsDir = "node_modules"

// detectedRungs runs the pack's detected commands over the ladder, cheap-first,
// and fills every stage they do not cover with its honest placeholder.
func detectedRungs(ctx context.Context, pack *CheckPack, runner CheckRunner, req CheckRequest, tree string) []CheckOutcome {
	var out []CheckOutcome
	web := webShapedTree(tree)
	firstFailure, failedStage := "", -1
	for _, stage := range ladderOrder {
		rank, _ := stageRank(stage)
		covered := false
		for _, c := range packChecksAt(pack, stage) {
			covered = true
			var o CheckOutcome
			if failedStage >= 0 && rank > failedStage {
				// First-upstream-failure attribution (Spec S07.3): a rung at a
				// stage after a failing one is undecided, not failing.
				o = CheckOutcome{
					CheckID: c.ID, Stage: c.Stage, State: CheckUnverifiable, AttributedTo: firstFailure,
					Detail: "an earlier rung failed, so this one could not say anything about the work",
				}
			} else {
				o = detectedOutcome(ctx, c, runner, req, tree)
				if o.State == CheckFailed {
					if failedStage < 0 || rank < failedStage {
						failedStage = rank
					}
					if firstFailure == "" {
						firstFailure = c.ID
					}
				}
			}
			out = append(out, o)
		}
		if !covered {
			out = append(out, placeholderRung(stage, web))
		}
	}
	return out
}

// packChecksAt returns a pack's checks at one ladder stage, in pack order.
func packChecksAt(pack *CheckPack, stage LadderStage) []Check {
	if pack == nil {
		return nil
	}
	var out []Check
	for _, c := range pack.Checks {
		if c.Stage == stage {
			out = append(out, c)
		}
	}
	return out
}

// detectedOutcome runs one detected rung and derives its outcome.
//
// EVIDENCE BY CONSTRUCTION (Spec S07.3 rule 4): the outcome carries no ACKey
// and no StepID, so ValidateAxis1 can never bind it as a mechanical AC fact and
// stepContracts can never let it decide a PLAN contract. The guarantee is
// structural — the fields are simply never filled on this path — rather than a
// property of whatever pack arrives here.
func detectedOutcome(ctx context.Context, c Check, runner CheckRunner, req CheckRequest, tree string) CheckOutcome {
	o := CheckOutcome{CheckID: c.ID, Stage: c.Stage}
	if reason := unrunnableReason(c, tree, runner); reason != "" {
		// Decided BEFORE execution, from the tree (Spec S07.3 rule 1): a rung
		// the sandbox cannot supply the tool for is never handed to the runner,
		// so it can never become a FAIL that blames the work for a platform
		// condition — and never the occasion for an egress exception.
		o.State, o.AttributedTo, o.Detail = CheckUnverifiable, DetectedUnrunnable, reason
		return o
	}
	r := req
	r.Check = c
	res, err := runner.RunCheck(ctx, r)
	if err != nil {
		// A runner tool failure is not a verdict. It is recorded and raises no
		// finding: a blocker here would drive REVISE to the cap and park the
		// run, which is the wall Spec S07.8 abolishes.
		o.State, o.Detail = CheckRunnerFailed, err.Error()
		return o
	}
	// The verdict is the exit status, read platform-side (Spec S07.3 rule 3).
	o.State, o.ExitCode = CheckPassed, res.ExitCode
	o.EvidenceRef, o.EvidenceSHA = res.EvidenceRef, res.EvidenceSHA
	if res.ExitCode != 0 {
		o.State = CheckFailed
	}
	return o
}

// placeholderRung is the outcome for a ladder stage no detected command covers:
// UNVERIFIABLE-HERE naming what is actually missing.
//
// The e2e rung of a WEB-SHAPED tree is the one stage where that is not a
// command. Spec S07.3's e2e rung for a web deliverable is the platform-authored
// acceptance walk [A16], and what it lacks is a browser, not a command line —
// so it says so. Web-shaped is decided from the tree (an index.html at its
// root, the S13.8 disposition marker) and never from the executor's report; a
// Go CLI's e2e rung keeps the absent-command reason, because there the missing
// thing really is a command and blaming an unadopted browser would be a false
// explanation in the other direction.
func placeholderRung(stage LadderStage, web bool) CheckOutcome {
	o := CheckOutcome{CheckID: "ladder:" + string(stage), Stage: stage, State: CheckUnverifiable}
	if stage == StageE2E && web {
		o.AttributedTo, o.Detail = WalkUnavailable, WalkUnavailableReason
		return o
	}
	o.AttributedTo = BootstrapAttribution
	// Spec S07.8's bootstrap posture, in the disclosure's own register.
	o.Detail = fmt.Sprintf("no %s command is captured for this project, so this rung is recorded as unproven here rather than passed", stage)
	return o
}

// webShapedTree reports whether the produced tree is a web deliverable — an
// index.html at its root. The predicate is LOCAL on purpose: internal/verify
// never imports internal/preview (CONVENTIONS §15), whose table answers a
// different question (how to LAUNCH a project, not what shape it is).
func webShapedTree(tree string) bool {
	if tree == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(tree, "index.html"))
	return err == nil
}

// unrunnableReason states, in plain words, why a detected rung cannot run here
// — or "" when it can. Every reason is decided from the tree before anything
// executes (Spec S07.3 rule 1).
//
// Two preconditions, both npm-shaped because package.json is the only manifest
// the detected path consults. A Go or Rust toolchain needs only the host
// toolchain, which every sandbox class already binds.
func unrunnableReason(c Check, tree string, runner CheckRunner) string {
	if runner == nil {
		return "the platform has no sandbox wired to run project commands in, so this command was not run"
	}
	script, ok := manifestScript(c.Argv)
	if !ok {
		return ""
	}
	scripts := manifestScripts(tree)
	if scripts == nil {
		// No manifest to decide from: absence of evidence is not a reason to
		// refuse to run. The exit status decides, as it does for every rung
		// whose toolchain this path does not read.
		return ""
	}
	body, declared := scripts[script]
	if !declared {
		return fmt.Sprintf("this project declares no %q script, so running it would have failed for a reason that has nothing to do with the work", script)
	}
	tool, _, _ := strings.Cut(strings.TrimSpace(body), " ")
	if tool == "" || hostResidentInterpreters[tool] {
		return ""
	}
	if _, err := os.Stat(filepath.Join(tree, detectedDepsDir)); err == nil {
		return ""
	}
	return fmt.Sprintf("the %q script runs %q, which comes from the project's installed dependencies — they are not in the copy of the work under review, and the checking sandbox has no network to fetch them, so this command could not run here", script, tool)
}

// manifestScript names the package.json script a detected command runs, if it
// runs one. The command is the shell line of the rung's argv (`/bin/sh -lc
// <line>`), and the two forms the re-scan can produce are `npm run <script>` /
// `npm <script>` and their pnpm equivalents.
func manifestScript(argv []string) (string, bool) {
	if len(argv) == 0 {
		return "", false
	}
	fields := strings.Fields(argv[len(argv)-1])
	if len(fields) < 2 || (fields[0] != "npm" && fields[0] != "pnpm") {
		return "", false
	}
	if fields[1] == "run" {
		if len(fields) < 3 {
			return "", false
		}
		return fields[2], true
	}
	return fields[1], true
}

// manifestScripts reads the scripts a tree's package.json declares. Nil means
// there is no manifest to decide from — an absent, unreadable or malformed one
// — which is distinct from a manifest that declares nothing.
func manifestScripts(tree string) map[string]string {
	if tree == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(tree, "package.json"))
	if err != nil {
		return nil
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	if manifest.Scripts == nil {
		return map[string]string{}
	}
	return manifest.Scripts
}
