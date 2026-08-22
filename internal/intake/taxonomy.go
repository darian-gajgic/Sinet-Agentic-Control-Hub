package intake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Interview taxonomies (Spec S06.5): each task family carries a versioned
// must-know taxonomy — the taxonomy decides WHAT must be asked; models only
// phrase. Taxonomies are knowledge objects: versioned, attributable,
// operator-editable, formally entering through the 8.3 knowledge gate when
// Spec S09 lands (B3); until then the seed ships here and its content is a
// B2-gate ratification item (TBD-P3 seed, Spec S06.5 + S19.5).

// plannerChoosesValue is the canonical option value for "you choose for me
// and show me what you picked" — the default the platform offers wherever a
// question asks for a technical or aesthetic decision the requester may have
// no opinion about (P3-RW-12 R1). It resolves the slot like any other answer,
// and the choice the planner actually made comes back on the S06.9 approval
// card's restatement and steps, where Re-plan can contest it. Nothing new is
// needed to make that work: an answered slot is an answered slot.
const plannerChoosesValue = "planner_chooses"

// Option is one labeled answer option of an interview question (Spec
// S06.5: 2–4 labeled options plus free text; free text is always
// available and is not an Option).
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Slot is one must-know slot of a family taxonomy. Weight is shipped in
// the taxonomy file and operator-editable (G1 P8).
type Slot struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MustKnow string `json:"must_know"`
	Weight   int    `json:"weight"`
	Question string `json:"question"`
	// Why is the one-line plain-words reason this question is worth answering,
	// written for the requester (P3-GF3-BE1 R1; design note §2.A(iii)). It is
	// requester-facing where MustKnow is the internal rationale.
	Why     string   `json:"why,omitempty"`
	Options []Option `json:"options,omitempty"`
}

// Taxonomy is one versioned family question set.
type Taxonomy struct {
	ID      string `json:"id"`
	Family  Family `json:"family"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Slots   []Slot `json:"slots"`
}

// Validate checks the structural rules a question set must satisfy.
func (t *Taxonomy) Validate() error {
	if t.ID == "" || t.Version == "" {
		return fmt.Errorf("%w: taxonomy requires id and version", ErrTaxonomy)
	}
	if len(t.Slots) == 0 {
		return fmt.Errorf("%w: taxonomy %q has no slots", ErrTaxonomy, t.ID)
	}
	seen := make(map[string]bool, len(t.Slots))
	for _, s := range t.Slots {
		switch {
		case s.ID == "":
			return fmt.Errorf("%w: slot without id in %q", ErrTaxonomy, t.ID)
		case seen[s.ID]:
			return fmt.Errorf("%w: duplicate slot %q in %q", ErrTaxonomy, s.ID, t.ID)
		case s.Weight <= 0:
			return fmt.Errorf("%w: slot %q needs a positive weight", ErrTaxonomy, s.ID)
		case s.Question == "":
			return fmt.Errorf("%w: slot %q without a question", ErrTaxonomy, s.ID)
		case len(s.Options) == 1 || len(s.Options) > 4:
			// S06.5: options come as 2–4 labeled choices (or none — free
			// text only).
			return fmt.Errorf("%w: slot %q must carry 0 or 2–4 options", ErrTaxonomy, s.ID)
		}
		seen[s.ID] = true
	}
	return nil
}

// Slot returns the slot by id, or nil.
func (t *Taxonomy) Slot(id string) *Slot {
	for i := range t.Slots {
		if t.Slots[i].ID == id {
			return &t.Slots[i]
		}
	}
	return nil
}

// Clearance computes the deterministic Clearance indicator (0–100, G1 P8):
// 100 × resolved weight / total weight over the active question set.
func (t *Taxonomy) Clearance(resolved map[string]bool) float64 {
	total, res := 0, 0
	for _, s := range t.Slots {
		total += s.Weight
		if resolved[s.ID] {
			res += s.Weight
		}
	}
	if total == 0 {
		return 0
	}
	return 100 * float64(res) / float64(total)
}

// Unresolved returns the unresolved slots, highest weight first (stable
// within equal weights) — the S06.5 card ordering.
func (t *Taxonomy) Unresolved(resolved map[string]bool) []Slot {
	var out []Slot
	for _, s := range t.Slots {
		if !resolved[s.ID] {
			out = append(out, s)
		}
	}
	for i := 1; i < len(out); i++ { // insertion sort: stable, tiny n
		for j := i; j > 0 && out[j].Weight > out[j-1].Weight; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// LoadTaxonomy reads an operator-edited taxonomy file (strict JSON,
// unknown fields rejected — the components.lock discipline for platform
// data files).
func LoadTaxonomy(path string) (*Taxonomy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("intake: read taxonomy %s: %w", path, err)
	}
	var t Taxonomy
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrTaxonomy, path, err)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// SeedTaxonomies returns the seed question sets (fresh copies) — one per
// family in the vocabulary, at the Deep-Plan depth the operator directed on
// 2026-08-12 (Spec S06.5 "question sets are per kind-of-task"; the S06.2 "v0
// ships software plus a generic fallback" line is a FLOOR the directive
// raises, additively — S15.2, no S00.9 amendment).
//
// Two drafting rules run through all six sets:
//
//   - Every requester-facing string is written for someone who is not a
//     programmer. The person answering these questions is describing what they
//     want, not specifying a system.
//   - Every question that asks for a technical or aesthetic DECISION offers
//     "you choose for me and show me what you picked" as its first option
//     (plannerChoosesValue). Nobody is forced to have an opinion to get a
//     result, and the choice the platform made comes back on the approval card
//     where it can be contested.
//
// Weights are evidence-informed where evidence exists and reasoned (with the
// reasoning recorded in each set's Source) where it does not. The evidence is
// the ClarifyCodeBench benchmark, and it covers the software family only: no
// comparable measurement of research/content/data/chore request ambiguity
// exists, so those weights say so rather than borrowing an authority they do
// not have.
func SeedTaxonomies() map[Family]*Taxonomy {
	return map[Family]*Taxonomy{
		FamilySoftware: softwareSeed(),
		FamilyResearch: researchSeed(),
		FamilyContent:  contentSeed(),
		FamilyData:     dataSeed(),
		FamilyChore:    choreSeed(),
		FamilyGeneric:  genericSeed(),
	}
}

// rw12Provenance is the drafting record every set revised or written at
// P3-RW-12 carries (Spec S06.5: taxonomies are "drafted at implementation time
// with the strongest available frontier model", entering through the 8.3
// knowledge gate — versioned, attributable, removable).
const rw12Provenance = "drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; " +
	"8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate"

// gf3Provenance is the drafting record the two sets revised at P3-GF3-BE1
// carry. It is APPENDED to each set's existing Source: the v2 record stays
// whole, because the ids and weights it attests to are unchanged and the
// citation behind them is still the reason they are what they are.
//
// Ratification is stated as PENDING on purpose. The v3 content ships and is
// governed under its own supersession from the first boot (S09.8), with the
// record saying plainly that the operator has not seen it yet; the resumed B6
// gate rules, and a refusal is executed as its own supersession back to v2.
// The alternative would leave the governed file at v2 while the runtime serves
// v3, which is the live-pointer drift the snapshot doctrine closed.
const gf3Provenance = "Requester-facing v3 revision drafted P3-GF3-BE1 with claude-opus-5 on 2026-08-23 per Spec S06.5: " +
	"every slot asks a question a person who is not a programmer can answer, offers 2 to 4 concrete labeled options, and " +
	"carries a one-line plain-words why; the precise engineering form of each question moved into MustKnow, which is read by " +
	"the planner and never shown as the asked question. Slot ids, weights, count and order are VERBATIM from v2 (the measured " +
	"thing, untouched). Operator ratification PENDING at the resumed B6 gate."

// softwareSeed is the software family's question set (v3).
//
// The ten ClarifyCodeBench clarity types (arXiv:2607.00711 Table 2) are kept
// verbatim from the B2-2 seed, ids and weights intact — they are the one
// MEASURED thing in this file. P3-RW-12 added the three questions a requester
// who is not a programmer is never asked but always has an answer to: what to
// build it with, where the pictures come from, and what it should look like.
//
// P3-GF3-BE1 rewrites what is ASKED, and nothing else. The benchmark's own
// phrasings ("what belongs in the collections, and how are they updated or
// accessed?") describe a clarity type to an engineer; the operator watched a
// non-programmer meet them on a real card and have nothing to type. Each slot
// now asks a question in the requester's world, offers concrete options, and
// says in one plain line why it is worth answering; the precise form moved into
// MustKnow, which the planner reads and the requester never sees.
//
// Where a slot asks for a DECISION the requester may have no opinion about, the
// planner-chooses option leads (the §57 rule). The three benchmark-failed slots
// deliberately do NOT offer it: those are the types models measurably get
// wrong, so inviting a guess there would spend the one thing the evidence says
// to ask about. A requester with no answer still has the free-text box and the
// per-slot skip, which converts the slot to a disclosed assumption on the
// approval card rather than to a silent guess.
func softwareSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "software",
		Family:  FamilySoftware,
		Version: "v3",
		Source: "ClarifyCodeBench 10-type taxonomy (arXiv:2607.00711, Table 2) for the ten clarity slots, seeded P3-B2-2 — " +
			"weights EVIDENCE-INFORMED from that benchmark: the types every model measurably fails (collection semantics, " +
			"comparison rules, ordering & atomicity) weigh 12, ordinary clarity types 10/8, the types models natively handle " +
			"(units, numerical precision) weigh 6. The three Deep-Plan slots (technology_stack 11, assets_media 10, look_feel 10) " +
			"are REASONED, not measured: for a requester who is not a programmer they shape the deliverable more than output " +
			"format or units do, so they sit above the natively-handled types — and strictly below the benchmark-failed 12, " +
			"because reasoning does not outrank measurement. Deep-Plan revision " + rw12Provenance + ". " + gf3Provenance,
		Slots: []Slot{
			{
				ID: "behavior", Name: "What it should do", Weight: 10,
				MustKnow: "The required function, objective, or side effect is underspecified, so the intended behavior is not uniquely determined. " +
					"Precise form: what exactly should this do — what behavior or outcome makes it correct?",
				Question: "What should this do for the people who end up using it?",
				Why:      "Everything else in the plan is built to make this one thing true.",
				Options: []Option{
					{Label: "Let people do something they cannot do today", Value: "new_capability"},
					{Label: "Fix or improve something that already exists", Value: "improve_existing"},
					{Label: "Take over a job somebody does by hand today", Value: "automate_manual"},
					{Label: "I will describe it in my own words", Value: "specify"},
				},
			},
			{
				ID: "terminology", Name: "Words that mean something specific", Weight: 10,
				MustKnow: "A domain term, action, or state is undefined, overloaded, or open to multiple interpretations. " +
					"Precise form: are there terms in the request that need pinning down — what do they mean here?",
				Question: "Do any words in your request mean something particular in your line of work?",
				Why:      "One word can mean two things, and building the other one is expensive to undo.",
				Options: []Option{
					{Label: "No, everything means what it usually means", Value: "none"},
					{Label: "Yes, a few words do and I will explain them", Value: "some_explain"},
					{Label: "Yes, and there is a list or a page I can point you at", Value: "glossary"},
				},
			},
			{
				ID: "edge_cases", Name: "When something unexpected happens", Weight: 10,
				MustKnow: "Boundary or exceptional conditions are not specified, leaving behavior unclear for special inputs. " +
					"Precise form: how should boundary and exceptional inputs be handled (empty, missing, malformed, extreme)?",
				Question: "When something turns up that nobody planned for, what should happen?",
				Why:      "Most unpleasant surprises live here, and this decides whether they are loud or quiet.",
				Options: []Option{
					{Label: "Stop and say clearly that something is wrong", Value: "fail_loud"},
					{Label: "Carry on with a sensible default and make a note of it", Value: "graceful"},
					{Label: "I will say what to do, case by case", Value: "specify"},
				},
			},
			{
				ID: "collection_semantics", Name: "The things it keeps track of", Weight: 12,
				MustKnow: "A collection, container, or state object is mentioned, but its membership, update rule, or access semantics are underspecified. " +
					"Precise form: for the collections/state involved, what belongs in them, and how are they updated or accessed?",
				Question: "What things does this keep track of, and what should happen when the same one turns up twice?",
				Why:      "Repeats are the most common mess in anything that keeps a list, and the rule has to be decided once.",
				Options: []Option{
					{Label: "Merge them into one and keep the newest details", Value: "merge_newest"},
					{Label: "Keep both and flag them for me to look at", Value: "keep_both_flag"},
					{Label: "Turn the second one away and say why", Value: "reject_new"},
				},
			},
			{
				ID: "comparison_rules", Name: "What order things come in", Weight: 12,
				MustKnow: "The comparison key, tie-breaking rule, or stability requirement is not specified. " +
					"Precise form: where things are compared, sorted, or deduplicated, by what key, and how are ties broken?",
				Question: "When things get listed or ranked, what order should they be in?",
				Why:      "A list in the wrong order looks broken to whoever reads it, and the rule is cheap to set now.",
				Options: []Option{
					{Label: "Newest first", Value: "newest_first"},
					{Label: "Alphabetical, by name", Value: "alphabetical"},
					{Label: "Closest to what the person was looking for", Value: "best_match"},
					{Label: "I will describe the order I want", Value: "specify"},
				},
			},
			{
				ID: "ordering_atomicity", Name: "What has to happen in order", Weight: 12,
				MustKnow: "Temporal order, simultaneity, or indivisible execution assumptions are unclear. " +
					"Precise form: does order of operations or atomicity matter here — what must happen before what, and what must never interleave?",
				Question: "Does anything here have to happen in a strict order, or happen completely or not at all?",
				Why:      "This is what stands between you and a half-finished job, like a payment taken with no order recorded.",
				Options: []Option{
					{Label: "Yes, some steps must happen in a set order and I will say which", Value: "strict_order"},
					{Label: "Yes, some things must either finish completely or not happen at all", Value: "all_or_nothing"},
					{Label: "No, nothing here depends on order", Value: "no_constraint"},
				},
			},
			{
				ID: "indices_ranges", Name: "Counting and ranges", Weight: 8,
				MustKnow: "Index bases, interval boundaries, or inclusion rules are underspecified. " +
					"Precise form: for indices and ranges, zero- or one-based, and are boundaries inclusive or exclusive?",
				Question: "When you say something like the first ten, or Monday to Friday, should both ends be counted in?",
				Why:      "Being out by one comes from here, and it is far easier to prevent than to find later.",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Count both ends in, so Monday to Friday is five days", Value: "inclusive"},
					{Label: "Count the start in but not the end", Value: "start_only"},
					{Label: "I will say, case by case", Value: "specify"},
				},
			},
			{
				ID: "output_format", Name: "The shape of what comes out", Weight: 8,
				MustKnow: "The required output structure, layout, or presentation rule is missing or unclear. " +
					"Precise form: what shape should the output take?",
				Question: "What should the result look like when it comes out?",
				Why:      "This is the part you will actually see, so it is worth a moment now.",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Match whatever this project already does", Value: "conventional"},
					{Label: "The exact shape matters and I will describe it", Value: "specify"},
					{Label: "Anything clear and readable is fine", Value: "any"},
				},
			},
			{
				ID: "units", Name: "Units and measurements", Weight: 6,
				MustKnow: "A quantity is specified without a clear unit, scale, prefix, or dimensional convention. " +
					"Precise form: are all quantities' units and scales unambiguous — if not, which convention applies?",
				Question: "Are there measurements involved, and which units should they be in?",
				Why:      "Mixed-up units stay invisible: everything looks right until a number is ten times off.",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Metric: millimetres, kilograms, degrees Celsius", Value: "metric"},
					{Label: "Imperial: inches, pounds, degrees Fahrenheit", Value: "imperial"},
					{Label: "There are no measurements in this", Value: "none"},
				},
			},
			{
				ID: "numerical_precision", Name: "Rounding", Weight: 6,
				MustKnow: "Precision, rounding, tolerance, or error handling requirements are unclear. " +
					"Precise form: do precision, rounding, or tolerance requirements apply to numeric results?",
				Question: "Do any numbers need rounding a particular way, like money or percentages?",
				Why:      "Rounding decides whether totals add up the way the person reading them expects.",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Money: two decimal places, rounded the usual way", Value: "money"},
					{Label: "Whole numbers only", Value: "whole"},
					{Label: "I will say what needs rounding and how", Value: "specify"},
				},
			},
			// ---- The three Deep-Plan slots (P3-RW-12) ----
			{
				ID: "technology_stack", Name: "Technology choice", Weight: 11,
				MustKnow: "Which language, framework, or platform the work should use is unstated — and everything else gets built on top of that choice, so a wrong one is expensive to undo later.",
				Question: "What should this be built with?",
				Why:      "Everything else gets built on top of this, so it is the choice that is hardest to change later.",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Match whatever this project already uses", Value: "match_existing"},
					{Label: "The simplest thing that does the job", Value: "simplest"},
					{Label: "I have something specific in mind and I will say what", Value: "specify"},
				},
			},
			{
				ID: "assets_media", Name: "Pictures and other media", Weight: 10,
				MustKnow: "Where the images, logos, or other media come from is unstated, so anything visible either stalls waiting for them or gets built around invented ones.",
				Question: "Where should pictures, logos, and any other media come from?",
				Why:      "Anything people look at needs pictures from somewhere, and sorting that out late stalls the work.",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "I will supply the files", Value: "i_supply"},
					{Label: "Use free images that are licensed for this", Value: "free_stock"},
					{Label: "Plain placeholders for now", Value: "placeholders"},
				},
			},
			{
				ID: "look_feel", Name: "Look and feel", Weight: 10,
				MustKnow: "The intended visual style is unstated, so anything the requester will actually look at gets built to somebody else's taste.",
				Question: "How should it look?",
				Why:      "You are the one who will look at it, so this is your call rather than anyone else's taste.",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Plain and clean, nothing fancy", Value: "plain"},
					{Label: "Like something that already exists, and I will point at it", Value: "match_reference"},
					{Label: "I will describe the style I want", Value: "specify"},
				},
			},
		},
	}
}

// researchSeed is the research family's Deep-Plan question set.
//
// The weight order follows the cost of getting each one wrong. A research
// answer that answers a different question, or serves a decision it was not
// aimed at, is worthless no matter how well sourced — so the question itself
// and the decision behind it lead. Sourcing and recency come next because they
// decide whether the answer is TRUE, and a stale fact reads exactly like a
// current one. Form and audience only shape delivery, so they trail. The
// "what you already believe" slot is deliberately mid-weight: it is the input
// that lets the platform contradict a false premise in the request instead of
// researching it as though it were true (Spec 1.7 ask-don't-assume).
func researchSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "research",
		Family:  FamilyResearch,
		Version: "v1",
		Source: "Deep-Plan must-know set for the research family. Slots and weights are REASONED — no benchmark of research-request " +
			"ambiguity comparable to ClarifyCodeBench exists, and none is implied here. Ordering rationale, slot by slot: the " +
			"question itself (12) and the decision it serves (11) lead, because an answer aimed at the wrong question is worthless " +
			"however well sourced; depth (10) is next, since it is the single biggest lever on what the work costs; sources, " +
			"recency and the form of the answer share 9 — the first two decide whether the answer is TRUE today, the third is " +
			"weighted with them because a research result delivered in the wrong shape gets re-done; balance and the requester's " +
			"own prior belief share 8 as the guards against one-sided answers and against researching a false premise as though it " +
			"were established; the edges of the question (7), the reader (6) and practical limits (5) shape delivery only. " + rw12Provenance,
		Slots: []Slot{
			{
				ID: "question", Name: "The question", Weight: 12,
				MustKnow: "The exact question to answer is not pinned down, so the work risks answering a nearby question well and the real one not at all.",
				Question: "What do you want to find out? Put it as a question you want answered.",
			},
			{
				ID: "decision", Name: "What it's for", Weight: 11,
				MustKnow: "What the requester will DO with the answer is unstated — and it decides how deep, how balanced, and how detailed the answer has to be.",
				Question: "What will you do with the answer — what decision or next step does it feed?",
			},
			{
				ID: "depth", Name: "How deep to go", Weight: 10,
				MustKnow: "How much rigour the answer needs is unstated, and the gap between a quick scan and a thorough investigation is most of the cost.",
				Question: "How deep should this go?",
				Options: []Option{
					{Label: "A quick scan — the gist, fast", Value: "quick"},
					{Label: "A solid answer with the main sources checked", Value: "solid"},
					{Label: "As thorough as it takes, awkward corners included", Value: "thorough"},
				},
			},
			{
				ID: "sources", Name: "Sources to use or avoid", Weight: 9,
				MustKnow: "Which sources count as trustworthy here, and which must be left out, is unstated.",
				Question: "Are there sources you trust — or sources you do not want this built on?",
				Options: []Option{
					{Label: "You choose the sources and show me which ones you used", Value: plannerChoosesValue},
					{Label: "Official or first-hand sources only", Value: "primary_only"},
					{Label: "I'll name the ones to use or avoid", Value: "specify"},
				},
			},
			{
				ID: "recency", Name: "How current it must be", Weight: 9,
				MustKnow: "Whether older material still counts is unstated — and something out of date reads exactly like something current.",
				Question: "How current does this have to be?",
				Options: []Option{
					{Label: "You choose and tell me the cut-off you used", Value: plannerChoosesValue},
					{Label: "Only the last year or so", Value: "last_year"},
					{Label: "Recent enough to still hold — a few years is fine", Value: "few_years"},
					{Label: "Age doesn't matter for this", Value: "any_age"},
				},
			},
			{
				ID: "output_form", Name: "Form of the answer", Weight: 9,
				MustKnow: "The shape the answer should arrive in is unstated.",
				Question: "What should the answer look like when it lands?",
				Options: []Option{
					{Label: "A short answer with the reasoning behind it", Value: "short_answer"},
					{Label: "A longer written report", Value: "report"},
					{Label: "A side-by-side comparison", Value: "comparison"},
					{Label: "A recommendation I can act on", Value: "recommendation"},
				},
			},
			{
				ID: "balance", Name: "Both sides", Weight: 8,
				MustKnow: "Whether the requester wants the case against as well as the case for is unstated, and a one-sided answer reads as confident whether or not it should.",
				Question: "Do you want the arguments against as well as the arguments for?",
				Options: []Option{
					{Label: "Yes — show me both sides and where they disagree", Value: "both_sides"},
					{Label: "Just the best current answer", Value: "best_answer"},
					{Label: "Mostly the case for, but flag anything that breaks it", Value: "case_with_risks"},
				},
			},
			{
				ID: "prior_belief", Name: "What you already believe", Weight: 8,
				MustKnow: "What the requester already knows or assumes is unstated — so an assumption baked into the request would be researched as though it were established fact.",
				Question: "What do you already know or believe about this? If something in your request turns out to be wrong, this is what lets the platform tell you so.",
			},
			{
				ID: "scope_bounds", Name: "Edges of the question", Weight: 7,
				MustKnow: "What falls inside and outside the question is unstated.",
				Question: "Anything this should deliberately cover — or deliberately leave alone?",
			},
			{
				ID: "audience", Name: "Who reads it", Weight: 6,
				MustKnow: "Who the answer is written for is unclear, and that sets how much background it has to carry.",
				Question: "Who is going to read this, and how much do they already know about it?",
			},
			{
				ID: "constraints", Name: "Practical limits", Weight: 5,
				MustKnow: "Practical limits — languages, paid sources, time, confidentiality — are unstated.",
				Question: "Any practical limits — languages, paid sources, time, or anything that must stay private?",
			},
		},
	}
}

// contentSeed is the content family's Deep-Plan question set.
//
// Weighting reasoning: a piece written for the wrong audience, or missing the
// points it exists to make, fails completely however well written it is — so
// what it is, who it is for, and what must be in it lead. Tone and length are
// the next most visible things a reader notices and the cheapest to get wrong
// at scale. Media, look and review expectations come last, but they are here
// because the operator directive names asset and look-and-feel choices for
// content too, not only software.
func contentSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "content",
		Family:  FamilyContent,
		Version: "v1",
		Source: "Deep-Plan must-know set for the content family. Slots and weights are REASONED, not benchmark-derived. " +
			"Ordering rationale: the wrong kind of piece, the wrong reader, or a missing key message fails the whole piece (12/11); " +
			"purpose, tone and length are what a reader notices first (10/10/9); references, media and look shape presentation (8/8/7); " +
			"review expectations and forbidden claims are guards on the finish (7/6). Asset and look-and-feel questions appear here " +
			"because the 2026-08-12 operator directive names them beyond the software family. " + rw12Provenance,
		Slots: []Slot{
			{
				ID: "piece_type", Name: "What you want made", Weight: 12,
				MustKnow: "What kind of piece this is, and where it will be published or used, is unstated — and that one fact sets length, form and register together.",
				Question: "What kind of piece is this, and where will it be used?",
				Options: []Option{
					{Label: "A page or post on a website", Value: "web"},
					{Label: "An email or message", Value: "email"},
					{Label: "A document or report", Value: "document"},
					{Label: "Something else — I'll describe it", Value: "other"},
				},
			},
			{
				ID: "audience", Name: "Who it's for", Weight: 11,
				MustKnow: "Who reads it, and what they already know, is unstated — so the piece cannot know what to explain and what to assume.",
				Question: "Who is this for, and what do they already know about the subject?",
			},
			{
				ID: "key_messages", Name: "What must be said", Weight: 11,
				MustKnow: "The points the piece MUST land are unstated, so a draft can read well and still miss the reason it was asked for.",
				Question: "What has to be in it? List the points that must come across.",
			},
			{
				ID: "purpose_action", Name: "What it should achieve", Weight: 10,
				MustKnow: "What the reader should think, feel, or do afterwards is unstated.",
				Question: "After reading it, what should the reader think or do?",
			},
			{
				ID: "tone_voice", Name: "Tone and voice", Weight: 10,
				MustKnow: "The voice the piece should speak in is unstated, and tone is the first thing a reader reacts to.",
				Question: "What tone should it strike?",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Plain and straightforward", Value: "plain"},
					{Label: "Warm and personal", Value: "warm"},
					{Label: "Formal and professional", Value: "formal"},
				},
			},
			{
				ID: "length_format", Name: "Length and shape", Weight: 9,
				MustKnow: "How long the piece should be, and whether it needs a set structure, is unstated.",
				Question: "How long should it be, and does it need a particular structure?",
				Options: []Option{
					{Label: "You choose a sensible length and show me", Value: plannerChoosesValue},
					{Label: "Short — a few paragraphs", Value: "short"},
					{Label: "Medium — about a page", Value: "medium"},
					{Label: "Long — as much as the subject needs", Value: "long"},
				},
			},
			{
				ID: "references", Name: "Examples to follow", Weight: 8,
				MustKnow: "Whether an existing piece should be imitated in style or structure is unstated.",
				Question: "Is there anything existing you'd like this to resemble? Point at it if so.",
			},
			{
				ID: "assets_media", Name: "Pictures and other media", Weight: 8,
				MustKnow: "Whether the piece needs images or other media, and where they come from, is unstated.",
				Question: "Should it include pictures or other media, and where should those come from?",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "I'll supply the files", Value: "i_supply"},
					{Label: "Use free images that are licensed for this", Value: "free_stock"},
					{Label: "Words only, no media", Value: "none"},
				},
			},
			{
				ID: "look_feel", Name: "Look and feel", Weight: 7,
				MustKnow: "How the piece should be presented is unstated, and anything published somewhere visible gets judged on how it looks.",
				Question: "How should it look on the page?",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Match the style of wherever it's going", Value: "match_home"},
					{Label: "Plain text, no styling needed", Value: "plain_text"},
					{Label: "I'll describe the look I want", Value: "specify"},
				},
			},
			{
				ID: "review", Name: "How you'll review it", Weight: 7,
				MustKnow: "How many rounds of review the requester expects, and who signs it off, is unstated.",
				Question: "How do you want to review this?",
				Options: []Option{
					{Label: "One draft, then I'll say what to change", Value: "one_draft"},
					{Label: "Give me a couple of options to choose between", Value: "options"},
					{Label: "Final and ready to use", Value: "final"},
				},
			},
			{
				ID: "must_avoid", Name: "Claims and things to avoid", Weight: 6,
				MustKnow: "Claims that must not be made, wording that is off-limits, or facts that must be stated exactly are unstated.",
				Question: "Anything that must NOT be said — or must be said exactly a certain way?",
			},
		},
	}
}

// dataSeed is the data family's Deep-Plan question set.
//
// Weighting reasoning: work on data cannot begin without the data and cannot
// be judged without knowing what was wanted from it, so those two lead
// together. Access, output form, the correctness bar, the missing-data posture
// and privacy all sit in the upper-middle band because each one silently
// changes every number downstream — the messy-data rule especially: a quiet
// choice to drop incomplete rows is a choice about what the answer means. Slice
// and definitions guard against confident nonsense; repeatability and tooling
// shape the build and trail.
func dataSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "data",
		Family:  FamilyData,
		Version: "v1",
		Source: "Deep-Plan must-know set for the data family. Slots and weights are REASONED, not benchmark-derived. " +
			"Ordering rationale: without the source and the wanted transformation there is no task at all (12/12); access, output " +
			"form, the correctness bar, the missing-data posture and privacy each silently change what every number means (10/10/9/9/9); " +
			"which slice and what the fields mean guard against confident nonsense (8/8); repeatability and tooling shape the build (6/6). " + rw12Provenance,
		Slots: []Slot{
			{
				ID: "source_data", Name: "Where the data is", Weight: 12,
				MustKnow: "Where the data lives and what form it is in is unstated — nothing can be worked on until that is known.",
				Question: "Where is the data, and what form is it in — a spreadsheet, an export, a database, files?",
			},
			{
				ID: "question_transform", Name: "What you want from it", Weight: 12,
				MustKnow: "Whether the requester wants a question answered or the data turned into something else is unstated, and those are different tasks.",
				Question: "What do you want done with it — a question answered, or the data turned into something else?",
			},
			{
				ID: "access", Name: "How to get to it", Weight: 10,
				MustKnow: "How the platform is meant to reach the data is unstated, and work that cannot read its input cannot start.",
				Question: "How should the platform get to the data?",
				Options: []Option{
					{Label: "I'll attach or upload the file", Value: "i_supply"},
					{Label: "It's already in a project the platform knows", Value: "in_project"},
					{Label: "It needs a login or key — I'll set that up", Value: "credentials"},
					{Label: "I'll paste in the part that matters", Value: "paste"},
				},
			},
			{
				ID: "output_form", Name: "Form of the result", Weight: 10,
				MustKnow: "The form the result should take is unstated.",
				Question: "What should come back?",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "A spreadsheet or table", Value: "table"},
					{Label: "A short written answer", Value: "written"},
					{Label: "Charts, or a small report", Value: "charts"},
				},
			},
			{
				ID: "correctness_bar", Name: "How sure it has to be", Weight: 9,
				MustKnow: "How much checking the numbers need is unstated, and the cost of a quiet error varies enormously with what they are used for.",
				Question: "How certain do these numbers have to be?",
				Options: []Option{
					{Label: "Roughly right is fine — I'll sanity-check it", Value: "rough"},
					{Label: "Spot-checked against the source", Value: "spot_checked"},
					{Label: "Checked carefully — real decisions ride on it", Value: "validated"},
				},
			},
			{
				ID: "messy_data", Name: "Gaps and mess", Weight: 9,
				MustKnow: "How to treat missing, duplicated, or plainly wrong entries is unstated — and whichever rule gets picked silently changes every number downstream.",
				Question: "What should happen with missing, duplicated, or obviously wrong entries?",
				Options: []Option{
					{Label: "You choose a sensible rule and tell me what you did", Value: plannerChoosesValue},
					{Label: "Leave gaps as gaps — never guess", Value: "leave_gaps"},
					{Label: "Drop the incomplete rows", Value: "drop"},
					{Label: "Ask me whenever it matters", Value: "ask"},
				},
			},
			{
				ID: "privacy", Name: "Private or sensitive contents", Weight: 9,
				MustKnow: "Whether the data carries personal or confidential contents is unstated, and it constrains where the work may run and what may appear in the result.",
				Question: "Does the data contain anything personal or confidential?",
				Options: []Option{
					{Label: "Nothing sensitive in it", Value: "none"},
					{Label: "It has personal details — keep them out of the result", Value: "personal"},
					{Label: "Confidential — it must not leave this machine", Value: "confidential"},
				},
			},
			{
				ID: "scope_slice", Name: "Which part of it", Weight: 8,
				MustKnow: "Which rows, period, or segment actually matter is unstated.",
				Question: "Which part of the data matters — all of it, a date range, certain rows?",
			},
			{
				ID: "definitions", Name: "What the fields mean", Weight: 8,
				MustKnow: "What the columns, codes, or terms mean is undefined, and guessing them produces answers that look right and are not.",
				Question: "Are there columns, codes, or terms whose meaning isn't obvious? What do they mean?",
			},
			{
				ID: "repeatability", Name: "Once, or again", Weight: 6,
				MustKnow: "Whether this is a one-off or must be repeatable is unstated, and it changes what gets built.",
				Question: "Is this a one-off, or something you'll want to run again?",
				Options: []Option{
					{Label: "Just this once", Value: "one_off"},
					{Label: "I'll want to run it again on new data", Value: "repeatable"},
					{Label: "It should run on a schedule", Value: "scheduled"},
				},
			},
			{
				ID: "tooling", Name: "What to do it with", Weight: 6,
				MustKnow: "Whether a particular tool or format is required at the other end is unstated.",
				Question: "Any preference for what this is done with?",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Keep it in a spreadsheet", Value: "spreadsheet"},
					{Label: "Whatever copes with the size best", Value: "size_appropriate"},
					{Label: "I have a tool in mind — I'll say which", Value: "specify"},
				},
			},
		},
	}
}

// choreSeed is the chore family's Deep-Plan question set.
//
// Weighting reasoning: a chore runs unattended and repeatedly, which changes
// what matters. "What does done look like EACH TIME" leads, because a routine
// with no definition of done cannot be judged on any run. The trigger and the
// per-run variables come next: a chore built around values that were fixed at
// planning time does the wrong thing quietly from the second run onward.
// Failure posture and notification are weighted above their one-off equivalents
// for the same reason — an unattended failure nobody hears about is the chore
// failure mode. What it may touch is high for the same reason: an outward
// effect repeated on a schedule is an outward effect multiplied.
func choreSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "chore",
		Family:  FamilyChore,
		Version: "v1",
		Source: "Deep-Plan must-know set for the chore family. Slots and weights are REASONED, not benchmark-derived. " +
			"Ordering rationale specific to unattended repetition, slot by slot: with no per-run definition of done, no run can be " +
			"judged at all (12); the trigger decides when it happens (11); the procedure of one run and the inputs that vary from " +
			"run to run share 10, because a chore built around values fixed at planning time does the wrong thing quietly from run " +
			"two onward; the success signal and the failure posture share 9, and the notification preference and the touch scope " +
			"share 8 — all four weighted above their one-off equivalents, because an unheard failure and an outward effect repeated " +
			"on a schedule are the characteristic chore harms; the stop condition (6) and what record it keeps (5) trail. " + rw12Provenance,
		Slots: []Slot{
			{
				ID: "done_looks_like", Name: "What done looks like", Weight: 12,
				MustKnow: "What a finished run looks like is unstated, so nothing can tell a good run from a bad one.",
				Question: "Each time this runs, what has to be true at the end for it to count as done?",
			},
			{
				ID: "trigger_schedule", Name: "When it runs", Weight: 11,
				MustKnow: "What sets the chore off — a clock, an event, or a request — is unstated.",
				Question: "When should this run?",
				Options: []Option{
					{Label: "On a schedule — I'll say how often", Value: "schedule"},
					{Label: "When something happens — I'll say what", Value: "event"},
					{Label: "Only when I ask for it", Value: "on_demand"},
				},
			},
			{
				ID: "steps_each_time", Name: "What happens each time", Weight: 10,
				MustKnow: "The actual work of a single run is unstated.",
				Question: "Walk me through what should happen on one run, in order.",
			},
			{
				ID: "varying_inputs", Name: "What changes each run", Weight: 10,
				MustKnow: "Which inputs differ from run to run is unstated — and a chore built around values fixed at planning time quietly does the wrong thing from the second run onward.",
				Question: "What is different from one run to the next — a date, a file, a name?",
			},
			{
				ID: "success_signal", Name: "How you'll know it worked", Weight: 9,
				MustKnow: "The observable signal of a successful run is unstated.",
				Question: "What would you look at to confirm a run worked?",
			},
			{
				ID: "failure_posture", Name: "When it goes wrong", Weight: 9,
				MustKnow: "What should happen on a failed run is unstated — and unattended work fails when nobody is watching.",
				Question: "If a run fails, what should happen?",
				Options: []Option{
					{Label: "Try again, then tell me if it still fails", Value: "retry_then_notify"},
					{Label: "Stop straight away and tell me", Value: "stop_notify"},
					{Label: "Skip that run and carry on next time", Value: "skip"},
				},
			},
			{
				ID: "notification", Name: "How much you want to hear", Weight: 8,
				MustKnow: "How much reporting the requester wants is unstated: too much becomes noise that hides the failures, too little hides them outright.",
				Question: "How much do you want to hear from it?",
				Options: []Option{
					{Label: "Only when something needs me", Value: "exceptions"},
					{Label: "Every run, briefly", Value: "every_run"},
					{Label: "A summary now and then", Value: "digest"},
				},
			},
			{
				ID: "touch_scope", Name: "What it may touch", Weight: 8,
				MustKnow: "What the chore may change, send, or spend is unstated — and whatever it is, it happens again on every run.",
				Question: "What is it allowed to change or send? Anything it must never touch?",
			},
			{
				ID: "stop_condition", Name: "When it should stop", Weight: 6,
				MustKnow: "How long the chore should keep running is unstated.",
				Question: "Should this keep running indefinitely, or stop at some point?",
			},
			{
				ID: "record_keeping", Name: "What record to keep", Weight: 5,
				MustKnow: "What trail the chore should leave behind is unstated.",
				Question: "What record should it keep of what it did?",
				Options: []Option{
					{Label: "You choose for me and show me what you picked", Value: plannerChoosesValue},
					{Label: "Keep a log of every run", Value: "log_all"},
					{Label: "Only keep the failures", Value: "failures_only"},
				},
			},
		},
	}
}

// genericSeed is the generic fallback set covering unmatched requests
// (Spec S06.2/S06.5). Drafted at P3-B2-2 (the TBD-P3 seed session);
// content is a B2-gate ratification item, governed under S09.10.
//
// It is BOTH the FamilyGeneric question set and the stand-in for a family
// with no seeded set — the second role no longer fires for any value in the
// vocabulary now that all six ship, and the machinery stays for an operator
// override map or a family seeded later.
//
// P3-RW-12 reviewed it and changed nothing. P3-GF3-BE1 does change it, for the
// reason that review missed: plain WORDS are not the same as an ANSWERABLE
// question. Six of the eight slots offered a blank box, and a blank box is
// where a person who came to describe what they want gives up. Every slot now
// carries concrete options and a one-line why; ids, weights, count and order
// are verbatim. It stays technology-agnostic, so no slot asks for a technical
// decision and none leads with the planner-chooses default; the free-text box
// and the per-slot skip carry a requester who has no answer.
func genericSeed() *Taxonomy {
	return &Taxonomy{
		ID:      "generic",
		Family:  FamilyGeneric,
		Version: "v3",
		Source: "Generic fallback must-know set; drafted P3-B2-2 per Spec S06.5 TBD-P3; 8.3-gate entry pending S09 (B3). " +
			gf3Provenance,
		Slots: []Slot{
			{
				ID: "goal", Name: "What done looks like", Weight: 12,
				MustKnow: "The outcome that makes the task done is not uniquely determined. " +
					"Precise form: what outcome makes this done — what will you look at to say it worked?",
				Question: "What has to be true at the end for you to call this done?",
				Why:      "This is what the finished work gets checked against, so it is the one to get right.",
				Options: []Option{
					{Label: "Something exists that did not exist before", Value: "something_new"},
					{Label: "Something broken or missing is put right", Value: "fixed"},
					{Label: "A question I have is answered", Value: "answered"},
					{Label: "I will describe it in my own words", Value: "specify"},
				},
			},
			{
				ID: "deliverable", Name: "What you get", Weight: 10,
				MustKnow: "The form the result should take is unspecified.",
				Question: "What form should the result take?",
				Why:      "This is what actually lands in your hands at the end.",
				Options: []Option{
					{Label: "A document or write-up", Value: "document"},
					{Label: "A working change (code, settings, files)", Value: "change"},
					{Label: "A recommendation with the reasoning behind it", Value: "recommendation"},
					{Label: "Something else, and I will describe it", Value: "other"},
				},
			},
			{
				ID: "scope", Name: "Where the edges are", Weight: 10,
				MustKnow: "What is in and out of scope is unstated. " +
					"Precise form: what should this explicitly include, and what should it not touch?",
				Question: "Is there anything this should deliberately stay away from?",
				Why:      "Saying what is out of bounds now is what stops work you never wanted.",
				Options: []Option{
					{Label: "Keep to exactly what I asked for, nothing extra", Value: "narrow"},
					{Label: "Go a little further where it obviously helps", Value: "sensible_extras"},
					{Label: "There are things it must not touch and I will name them", Value: "specify_limits"},
				},
			},
			{
				ID: "inputs", Name: "What to build on", Weight: 8,
				MustKnow: "Which materials, sources, or prior work the task should build on is unclear. " +
					"Precise form: what should this be based on — specific files, sources, or prior work?",
				Question: "Is there anything existing this should build on, like files, notes, or earlier work?",
				Why:      "Starting from what you already have is faster and lands closer to what you meant.",
				Options: []Option{
					{Label: "Yes, and I will supply it or point at it", Value: "i_supply"},
					{Label: "Yes, it is already somewhere the platform can see", Value: "in_project"},
					{Label: "No, start from nothing", Value: "from_scratch"},
				},
			},
			{
				ID: "constraints", Name: "Hard limits", Weight: 8,
				MustKnow: "Hard constraints (tools, style, budget, privacy, compatibility) are unstated.",
				Question: "Are there hard limits, like a budget, a tool you must use, or something that has to stay private?",
				Why:      "A limit that turns up late usually means doing the work a second time.",
				Options: []Option{
					{Label: "None that I can think of", Value: "none"},
					{Label: "Yes, and I will name them", Value: "specify"},
					{Label: "Keep it cheap and simple, whatever that takes", Value: "cheap_simple"},
				},
			},
			{
				ID: "audience", Name: "Who it is for", Weight: 6,
				MustKnow: "Who the result is for, and where it will be used, is unclear.",
				Question: "Who is going to use or read this?",
				Why:      "It sets how much the result has to explain itself.",
				Options: []Option{
					{Label: "Just me", Value: "just_me"},
					{Label: "Me and a few people I work with", Value: "small_group"},
					{Label: "Customers, or the public", Value: "public"},
					{Label: "I will describe who they are", Value: "specify"},
				},
			},
			{
				ID: "quality_bar", Name: "How thorough", Weight: 6,
				MustKnow: "The intended thoroughness of the work is unstated.",
				Question: "How thorough should this be?",
				Why:      "This is the biggest lever on how long it takes and what it costs.",
				Options: []Option{
					{Label: "Quick and rough is fine", Value: "quick"},
					{Label: "Solid, normal quality", Value: "normal"},
					{Label: "As thorough as it takes", Value: "thorough"},
				},
			},
			{
				ID: "deadline", Name: "Timing", Weight: 4,
				MustKnow: "Whether timing matters is unstated.",
				Question: "Is there a date this needs to be ready by?",
				Why:      "If there is a date, the plan can be cut to fit it instead of missing it.",
				Options: []Option{
					{Label: "No, whenever it is ready", Value: "no_deadline"},
					{Label: "As soon as it can be done", Value: "asap"},
					{Label: "There is a date and I will give it", Value: "specify"},
				},
			},
		},
	}
}
