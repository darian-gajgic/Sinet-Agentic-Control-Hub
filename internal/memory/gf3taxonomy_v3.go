package memory

import (
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf3taxonomy_v3.go — the software and generic question sets EXACTLY as the
// P3-GF3-BE1 record ships them (v3), frozen at P3-GF7.
//
// Same doctrine as rw12taxonomy_v2.go, applied one packet later and for the
// same reason: a ratification record that follows the code is not a record.
// EnsureGF3TaxonomyGovernance marshalled whatever intake.SeedTaxonomies()
// returned, which was honest exactly while nobody edited a question set again.
// P3-GF7 moves the SOFTWARE set to v4 (the operator's W2 taxonomy rebuild), so
// without this file the next boot of any world would write v4 content under the
// GF3 originRef — a governed row attesting a record that never covered it — and
// gf3ContentDigest would stop being true of anything.
//
// The GF3 digests are NOT edited to match: they pin what the GF3 record covers,
// and this file is the content they pin. The v4 content enters the record the
// only honest way, as its own SUPERSESSION under its own provenance
// (EnsureGF7TaxonomyGovernance, gf7seeds.go) — S09.8: a new version, never an
// in-place edit. The generic set is unchanged by P3-GF7 and is frozen here for
// the same reason RW-12 froze the generic set it did not touch: the trap is the
// live POINTER, and the moment two things agree is the only cheap moment to pin
// them.
//
// Generated once from the v3 seeds and frozen; TestGF3DigestsCoverTheRevisedSeeds
// (constant against constant) and TestGF7SnapshotStillHoldsTheV3GenericSeed keep
// it honest.

// gf3TaxonomySnapshot returns the question sets the P3-GF3-BE1 record covers.
func gf3TaxonomySnapshot() map[intake.Family]*intake.Taxonomy {
	return map[intake.Family]*intake.Taxonomy{
		intake.FamilySoftware: {
			ID:      "software",
			Family:  intake.FamilySoftware,
			Version: "v3",
			Source:  "ClarifyCodeBench 10-type taxonomy (arXiv:2607.00711, Table 2) for the ten clarity slots, seeded P3-B2-2 — weights EVIDENCE-INFORMED from that benchmark: the types every model measurably fails (collection semantics, comparison rules, ordering & atomicity) weigh 12, ordinary clarity types 10/8, the types models natively handle (units, numerical precision) weigh 6. The three Deep-Plan slots (technology_stack 11, assets_media 10, look_feel 10) are REASONED, not measured: for a requester who is not a programmer they shape the deliverable more than output format or units do, so they sit above the natively-handled types — and strictly below the benchmark-failed 12, because reasoning does not outrank measurement. Deep-Plan revision drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; 8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate. Requester-facing v3 revision drafted P3-GF3-BE1 with claude-opus-5 on 2026-08-23 per Spec S06.5: every slot asks a question a person who is not a programmer can answer, offers 2 to 4 concrete labeled options, and carries a one-line plain-words why; the precise engineering form of each question moved into MustKnow, which is read by the planner and never shown as the asked question. Slot ids, weights, count and order are VERBATIM from v2 (the measured thing, untouched). Operator ratification PENDING at the resumed B6 gate.",
			Slots: []intake.Slot{
				{
					ID: "behavior", Name: "What it should do", Weight: 10,
					MustKnow: "The required function, objective, or side effect is underspecified, so the intended behavior is not uniquely determined. Precise form: what exactly should this do — what behavior or outcome makes it correct?",
					Question: "What should this do for the people who end up using it?",
					Why:      "Everything else in the plan is built to make this one thing true.",
					Options: []intake.Option{
						{Label: "Let people do something they cannot do today", Value: "new_capability"},
						{Label: "Fix or improve something that already exists", Value: "improve_existing"},
						{Label: "Take over a job somebody does by hand today", Value: "automate_manual"},
						{Label: "I will describe it in my own words", Value: "specify"},
					},
				},
				{
					ID: "terminology", Name: "Words that mean something specific", Weight: 10,
					MustKnow: "A domain term, action, or state is undefined, overloaded, or open to multiple interpretations. Precise form: are there terms in the request that need pinning down — what do they mean here?",
					Question: "Do any words in your request mean something particular in your line of work?",
					Why:      "One word can mean two things, and building the other one is expensive to undo.",
					Options: []intake.Option{
						{Label: "No, everything means what it usually means", Value: "none"},
						{Label: "Yes, a few words do and I will explain them", Value: "some_explain"},
						{Label: "Yes, and there is a list or a page I can point you at", Value: "glossary"},
					},
				},
				{
					ID: "edge_cases", Name: "When something unexpected happens", Weight: 10,
					MustKnow: "Boundary or exceptional conditions are not specified, leaving behavior unclear for special inputs. Precise form: how should boundary and exceptional inputs be handled (empty, missing, malformed, extreme)?",
					Question: "When something turns up that nobody planned for, what should happen?",
					Why:      "Most unpleasant surprises live here, and this decides whether they are loud or quiet.",
					Options: []intake.Option{
						{Label: "Stop and say clearly that something is wrong", Value: "fail_loud"},
						{Label: "Carry on with a sensible default and make a note of it", Value: "graceful"},
						{Label: "I will say what to do, case by case", Value: "specify"},
					},
				},
				{
					ID: "collection_semantics", Name: "The things it keeps track of", Weight: 12,
					MustKnow: "A collection, container, or state object is mentioned, but its membership, update rule, or access semantics are underspecified. Precise form: for the collections/state involved, what belongs in them, and how are they updated or accessed?",
					Question: "What things does this keep track of, and what should happen when the same one turns up twice?",
					Why:      "Repeats are the most common mess in anything that keeps a list, and the rule has to be decided once.",
					Options: []intake.Option{
						{Label: "Merge them into one and keep the newest details", Value: "merge_newest"},
						{Label: "Keep both and flag them for me to look at", Value: "keep_both_flag"},
						{Label: "Turn the second one away and say why", Value: "reject_new"},
					},
				},
				{
					ID: "comparison_rules", Name: "What order things come in", Weight: 12,
					MustKnow: "The comparison key, tie-breaking rule, or stability requirement is not specified. Precise form: where things are compared, sorted, or deduplicated, by what key, and how are ties broken?",
					Question: "When things get listed or ranked, what order should they be in?",
					Why:      "A list in the wrong order looks broken to whoever reads it, and the rule is cheap to set now.",
					Options: []intake.Option{
						{Label: "Newest first", Value: "newest_first"},
						{Label: "Alphabetical, by name", Value: "alphabetical"},
						{Label: "Closest to what the person was looking for", Value: "best_match"},
						{Label: "I will describe the order I want", Value: "specify"},
					},
				},
				{
					ID: "ordering_atomicity", Name: "What has to happen in order", Weight: 12,
					MustKnow: "Temporal order, simultaneity, or indivisible execution assumptions are unclear. Precise form: does order of operations or atomicity matter here — what must happen before what, and what must never interleave?",
					Question: "Does anything here have to happen in a strict order, or happen completely or not at all?",
					Why:      "This is what stands between you and a half-finished job, like a payment taken with no order recorded.",
					Options: []intake.Option{
						{Label: "Yes, some steps must happen in a set order and I will say which", Value: "strict_order"},
						{Label: "Yes, some things must either finish completely or not happen at all", Value: "all_or_nothing"},
						{Label: "No, nothing here depends on order", Value: "no_constraint"},
					},
				},
				{
					ID: "indices_ranges", Name: "Counting and ranges", Weight: 8,
					MustKnow: "Index bases, interval boundaries, or inclusion rules are underspecified. Precise form: for indices and ranges, zero- or one-based, and are boundaries inclusive or exclusive?",
					Question: "When you say something like the first ten, or Monday to Friday, should both ends be counted in?",
					Why:      "Being out by one comes from here, and it is far easier to prevent than to find later.",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Count both ends in, so Monday to Friday is five days", Value: "inclusive"},
						{Label: "Count the start in but not the end", Value: "start_only"},
						{Label: "I will say, case by case", Value: "specify"},
					},
				},
				{
					ID: "output_format", Name: "The shape of what comes out", Weight: 8,
					MustKnow: "The required output structure, layout, or presentation rule is missing or unclear. Precise form: what shape should the output take?",
					Question: "What should the result look like when it comes out?",
					Why:      "This is the part you will actually see, so it is worth a moment now.",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Match whatever this project already does", Value: "conventional"},
						{Label: "The exact shape matters and I will describe it", Value: "specify"},
						{Label: "Anything clear and readable is fine", Value: "any"},
					},
				},
				{
					ID: "units", Name: "Units and measurements", Weight: 6,
					MustKnow: "A quantity is specified without a clear unit, scale, prefix, or dimensional convention. Precise form: are all quantities' units and scales unambiguous — if not, which convention applies?",
					Question: "Are there measurements involved, and which units should they be in?",
					Why:      "Mixed-up units stay invisible: everything looks right until a number is ten times off.",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Metric: millimetres, kilograms, degrees Celsius", Value: "metric"},
						{Label: "Imperial: inches, pounds, degrees Fahrenheit", Value: "imperial"},
						{Label: "There are no measurements in this", Value: "none"},
					},
				},
				{
					ID: "numerical_precision", Name: "Rounding", Weight: 6,
					MustKnow: "Precision, rounding, tolerance, or error handling requirements are unclear. Precise form: do precision, rounding, or tolerance requirements apply to numeric results?",
					Question: "Do any numbers need rounding a particular way, like money or percentages?",
					Why:      "Rounding decides whether totals add up the way the person reading them expects.",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Money: two decimal places, rounded the usual way", Value: "money"},
						{Label: "Whole numbers only", Value: "whole"},
						{Label: "I will say what needs rounding and how", Value: "specify"},
					},
				},
				{
					ID: "technology_stack", Name: "Technology choice", Weight: 11,
					MustKnow: "Which language, framework, or platform the work should use is unstated — and everything else gets built on top of that choice, so a wrong one is expensive to undo later.",
					Question: "What should this be built with?",
					Why:      "Everything else gets built on top of this, so it is the choice that is hardest to change later.",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
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
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
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
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Plain and clean, nothing fancy", Value: "plain"},
						{Label: "Like something that already exists, and I will point at it", Value: "match_reference"},
						{Label: "I will describe the style I want", Value: "specify"},
					},
				},
			},
		},
		intake.FamilyGeneric: {
			ID:      "generic",
			Family:  intake.FamilyGeneric,
			Version: "v3",
			Source:  "Generic fallback must-know set; drafted P3-B2-2 per Spec S06.5 TBD-P3; 8.3-gate entry pending S09 (B3). Requester-facing v3 revision drafted P3-GF3-BE1 with claude-opus-5 on 2026-08-23 per Spec S06.5: every slot asks a question a person who is not a programmer can answer, offers 2 to 4 concrete labeled options, and carries a one-line plain-words why; the precise engineering form of each question moved into MustKnow, which is read by the planner and never shown as the asked question. Slot ids, weights, count and order are VERBATIM from v2 (the measured thing, untouched). Operator ratification PENDING at the resumed B6 gate.",
			Slots: []intake.Slot{
				{
					ID: "goal", Name: "What done looks like", Weight: 12,
					MustKnow: "The outcome that makes the task done is not uniquely determined. Precise form: what outcome makes this done — what will you look at to say it worked?",
					Question: "What has to be true at the end for you to call this done?",
					Why:      "This is what the finished work gets checked against, so it is the one to get right.",
					Options: []intake.Option{
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
					Options: []intake.Option{
						{Label: "A document or write-up", Value: "document"},
						{Label: "A working change (code, settings, files)", Value: "change"},
						{Label: "A recommendation with the reasoning behind it", Value: "recommendation"},
						{Label: "Something else, and I will describe it", Value: "other"},
					},
				},
				{
					ID: "scope", Name: "Where the edges are", Weight: 10,
					MustKnow: "What is in and out of scope is unstated. Precise form: what should this explicitly include, and what should it not touch?",
					Question: "Is there anything this should deliberately stay away from?",
					Why:      "Saying what is out of bounds now is what stops work you never wanted.",
					Options: []intake.Option{
						{Label: "Keep to exactly what I asked for, nothing extra", Value: "narrow"},
						{Label: "Go a little further where it obviously helps", Value: "sensible_extras"},
						{Label: "There are things it must not touch and I will name them", Value: "specify_limits"},
					},
				},
				{
					ID: "inputs", Name: "What to build on", Weight: 8,
					MustKnow: "Which materials, sources, or prior work the task should build on is unclear. Precise form: what should this be based on — specific files, sources, or prior work?",
					Question: "Is there anything existing this should build on, like files, notes, or earlier work?",
					Why:      "Starting from what you already have is faster and lands closer to what you meant.",
					Options: []intake.Option{
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
					Options: []intake.Option{
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
					Options: []intake.Option{
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
					Options: []intake.Option{
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
					Options: []intake.Option{
						{Label: "No, whenever it is ready", Value: "no_deadline"},
						{Label: "As soon as it can be done", Value: "asap"},
						{Label: "There is a date and I will give it", Value: "specify"},
					},
				},
			},
		},
	}
}
