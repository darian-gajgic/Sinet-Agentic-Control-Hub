package memory

import (
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf7taxonomy_v4.go — the software question set EXACTLY as the P3-GF7 record
// ships it (v4), frozen at P3-V41.
//
// Same doctrine as rw12taxonomy_v2.go and gf3taxonomy_v3.go, applied one packet
// later and for the same reason (CONVENTIONS §57: governance content is a
// SNAPSHOT, never a live pointer). EnsureGF7TaxonomyGovernance marshalled
// whatever intake.SeedTaxonomies() returned, which is honest exactly while
// nobody edits the software set again. P3-V41 moves it to v4.1 (quality_bar
// weight 8 to 12, operator-ratified 2026-09-17), so without this file the next
// boot of any world would write v4.1 content under the GF7 originRef — a
// governed row attesting a record that never covered it — and gf7ContentDigest
// would stop being true of anything.
//
// The GF7 digest is NOT edited to match: it pins what the GF7 record covers,
// and this file is the content it pins. The v4.1 content enters the record the
// only honest way, as its own SUPERSESSION under its own provenance
// (EnsureV41TaxonomyGovernance, v41seeds.go) — Spec S09.8: a new version, never
// an in-place edit.
//
// Generated once from the v4 seed and frozen; TestGF7DigestsMatchTheShippedSeeds
// (constant against constant) and TestV41SnapshotStillHoldsTheV4Bytes keep it
// honest.

// gf7TaxonomySnapshot returns the question set the P3-GF7 record covers.
func gf7TaxonomySnapshot() map[intake.Family]*intake.Taxonomy {
	return map[intake.Family]*intake.Taxonomy{
		intake.FamilySoftware: {
			ID:      "software",
			Family:  intake.FamilySoftware,
			Version: "v4",
			Source:  "ClarifyCodeBench 10-type taxonomy (arXiv:2607.00711, Table 2) for the ten clarity slots, seeded P3-B2-2 — weights EVIDENCE-INFORMED from that benchmark: the types every model measurably fails (collection semantics, comparison rules, ordering & atomicity) weigh 12, ordinary clarity types 10/8, the types models natively handle (units, numerical precision) weigh 6. The three Deep-Plan slots (technology_stack 11, assets_media 10, look_feel 10) are REASONED, not measured: for a requester who is not a programmer they shape the deliverable more than output format or units do, so they sit above the natively-handled types — and strictly below the benchmark-failed 12, because reasoning does not outrank measurement. Deep-Plan revision drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; 8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate. Requester-facing v3 revision drafted P3-GF3-BE1 with claude-opus-5 on 2026-08-23 per Spec S06.5: every slot asks a question a person who is not a programmer can answer, offers 2 to 4 concrete labeled options, and carries a one-line plain-words why; the precise engineering form of each question moved into MustKnow, which is read by the planner and never shown as the asked question. Slot ids, weights, count and order are VERBATIM from v2 (the measured thing, untouched). Operator ratification PENDING at the resumed B6 gate. W2 taxonomy rebuild drafted P3-GF7 with claude-opus-5[1m] on 2026-08-27 per Spec S06.5 (\"drafted at implementation time with the strongest available frontier model\") — the same model line the P3-RW-12 and P3-GF3-BE1 taxonomy records name, so the drafting chain is one model line across all three versions. From the operator records b6-gate-operator-findings-r5-2026-08-23 (per-slot verdicts, the seven hard rules, the W2 order) and -r4 §F2, plus the live benchmark walk w1-nexus-live-harvest-2026-08-27 (H2 why-lines, H3 per-option effects, H10 inferred-not-asked, H23 the reference question set). THREE slots the operator killed as questions and one ruled with them (OQ1) keep their ids and weights and are never asked: behavior and terminology INVERT — the platform states its understanding and the requester corrects it — while indices_ranges and numerical_precision are settled internally and disclosed as assumptions. Every surviving asked slot was redrafted to the r5 §C bar: a plain purpose saying what breaks if unanswered, concrete options a non-programmer can pick between, exactly one recommended default, and an effect line on every option. TWO slots are NEW, their weights REASONED and never outranking the measured 12s: language_locale 9, because every seeded string, label and message in the deliverable is written in the language it names and changing it afterwards rewrites all of them — placing it above the delivery-shape and finishing-bar slots and below the technology choice everything else is built on; quality_bar 8, because it decides what verification gates on rather than what gets built, which is consequential but narrower — level with output_format, whose delivery shape it judges. Slot ids and weights of every surviving slot are VERBATIM from v3 (the measured thing, untouched). Operator ratification PENDING at the planning-rework exit gate.",
			Slots: []intake.Slot{
				{
					ID: "behavior", Name: "What it should do", Weight: 10, Ask: intake.AskNever,
					MustKnow: "NEVER ASKED. Derive the intended behavior from the request itself and STATE it: the plain-language restatement on the SPEC is the behavior understanding, and every behavior the request implies without saying outright goes in the assumptions list as a correctable statement (\"I am taking this to mean …\"), never as a question.",
				},
				{
					ID: "terminology", Name: "Words that mean something specific", Weight: 10, Ask: intake.AskNever,
					MustKnow: "NEVER ASKED. State, in the restatement and in the assumptions, the reading of every word in the request that carries a special or trade meaning (\"compatibility here means which cars a part fits\"), as a statement the requester can correct or confirm. Precise form of the underlying ambiguity: a domain term, action, or state is undefined, overloaded, or open to multiple interpretations.",
				},
				{
					ID: "edge_cases", Name: "When something unexpected happens", Weight: 10,
					MustKnow:    "Boundary or exceptional conditions are not specified, leaving behavior unclear for special inputs. Precise form: how should boundary and exceptional inputs be handled (empty, missing, malformed, extreme)?",
					Question:    "When something turns up that nobody planned for, what should happen?",
					Why:         "Bad input either stops the work loudly or slips through quietly, and this is where that is decided.",
					Recommended: "fail_loud",
					Options: []intake.Option{
						{Label: "Stop and say clearly that something is wrong", Value: "fail_loud", Effect: "Nothing wrong passes silently: whoever hit it sees a plain message instead of a result built on bad input. Recommended, because a loud stop annoys someone once and a quiet wrong answer gets trusted for months."},
						{Label: "Carry on with a sensible default and make a note of it", Value: "graceful", Effect: "The work keeps running through odd input and records what it did — nothing ever halts, and a wrong result can go unnoticed."},
						{Label: "I will say what to do, case by case", Value: "specify", Effect: "The cases you name are handled the way you said, and the rest fall back to stopping loudly."},
					},
				},
				{
					ID: "collection_semantics", Name: "The things it keeps track of", Weight: 12,
					MustKnow:    "A collection, container, or state object is mentioned, but its membership, update rule, or access semantics are underspecified. Precise form: for the collections/state involved, what belongs in them, and how are they updated or accessed?",
					Question:    "What things does this keep track of, and what should happen when the same one turns up twice?",
					Why:         "With no rule for repeats, the same thing appears twice in the list people see and gets counted twice in every total.",
					Recommended: "merge_newest",
					Options: []intake.Option{
						{Label: "Merge them into one and keep the newest details", Value: "merge_newest", Effect: "One entry per thing, always showing the latest information. Recommended: nothing disappears from view and nothing is shown twice."},
						{Label: "Keep both and flag them for me to look at", Value: "keep_both_flag", Effect: "Nothing is decided for you — both entries stay, marked as possible repeats, and you sort them out."},
						{Label: "Turn the second one away and say why", Value: "reject_new", Effect: "The second one never gets in and whoever added it is told why; strict, and a genuine entry that only looked like a repeat is refused too."},
					},
				},
				{
					ID: "comparison_rules", Name: "The order things are listed in", Weight: 12,
					MustKnow:    "The comparison key, tie-breaking rule, or stability requirement is not specified. Precise form: where things are compared, sorted, or deduplicated, by what key, and how are ties broken? Bind the rule to a NAMED surface: the default list and the results of a search or filter are separate orders and are stated separately.",
					Question:    "When this shows a list of things — the main list people browse, and what comes back after they search or filter — what order should they be in?",
					Why:         "A list nobody set an order for comes out in whatever order the data happened to be stored in, which reads as broken to the person looking at it.",
					Recommended: "match_then_newest",
					Options: []intake.Option{
						{Label: "Closest match to what they typed when they search; newest first in the main list", Value: "match_then_newest", Effect: "Searching puts the items matching the most of their words and filters at the top; the untouched main list leads with the most recently added. Recommended: it is the order people already expect from a list they can search."},
						{Label: "Newest first, in both", Value: "newest_first", Effect: "The main list and every search result lead with the most recently added item, so new things are always found first and older ones sink."},
						{Label: "A to Z by name, in both", Value: "alphabetical", Effect: "Both lists read like an index — the same order every time, easy to scan, and new items are no easier to find than old ones."},
						{Label: "I will describe the order I want", Value: "specify", Effect: "Both lists follow the rule you describe, and the plan repeats back what it understood before anything is built."},
					},
				},
				{
					ID: "ordering_atomicity", Name: "Steps that must not be left half-done", Weight: 12,
					MustKnow:    "Temporal order, simultaneity, or indivisible execution assumptions are unclear. Precise form: does order of operations or atomicity matter here — what must happen before what, and what must never interleave? Where the requester left the decision to the platform, work out the rule for every place a half-finished step would do damage and disclose each decision on the plan.",
					Question:    "Are there steps here that must happen in a fixed order, or that must finish completely or not at all?",
					Why:         "It decides whether something that fails halfway leaves a mess behind — a payment taken with no order recorded — or leaves nothing behind.",
					Recommended: "planner_chooses",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses", Effect: "I go through the request for the places where stopping halfway would do damage, decide the rule there, and list every one of those decisions on the plan for you to check. Recommended: this is genuinely the platform's job to work out, not yours."},
						{Label: "Yes, some steps must happen in a set order and I will say which", Value: "strict_order", Effect: "The order you name is fixed in the plan and the finished work is checked against it."},
						{Label: "Yes, some things must either finish completely or not happen at all", Value: "all_or_nothing", Effect: "Those steps are built so a failure halfway undoes itself, instead of leaving half a result behind."},
						{Label: "No, nothing here depends on order", Value: "no_constraint", Effect: "Steps are built in whatever order is simplest — faster to build, with nothing guarding against a half-finished run."},
					},
				},
				{
					ID: "indices_ranges", Name: "Counting and ranges", Weight: 8, Ask: intake.AskNever,
					MustKnow: "NEVER ASKED — settled internally and disclosed. Resolve counting and boundary conventions from the request: thirty items means thirty items, and a range the requester names (\"Monday to Friday\") includes both ends unless they said otherwise. DISCLOSE the choice as a listed assumption wherever it is load-bearing. Raise it as a 1.7 single-question escalation only where the two readings genuinely change the deliverable.",
				},
				{
					ID: "output_format", Name: "How you get it", Weight: 8,
					MustKnow:    "How the finished work should ARRIVE, and where it should run, is unstated. Precise form: is the deliverable something the requester starts and uses themselves, changes made in place inside the project, or something deployed where other people reach it — and what does \"done\" therefore mean for the requester's own use of it?",
					Question:    "When this is finished, how should it reach you?",
					Why:         "It decides whether the work ends with something you can open and use, or with files that still need someone to set them up.",
					Recommended: "run_locally",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses", Effect: "I pick the arrival shape that fits the goal and say on the plan exactly how the finished work will turn up."},
						{Label: "Something I can open and use on my own computer", Value: "run_locally", Effect: "It is finished when it starts on your machine with one command and you can click through it. Recommended: it is the only shape you can check for yourself, and anything else can still be built from it later."},
						{Label: "Changes made inside the project, ready for whoever runs it", Value: "in_project", Effect: "The work lands in the project's own layout and conventions; there is nothing to click at the end, so it is judged by reading it rather than using it."},
						{Label: "Live on the internet where other people can reach it", Value: "deployed", Effect: "The work includes putting it somewhere public and everything that comes with that — an address, hosting, and the extra checks before real people see it. Slower, and it is the one shape you cannot quietly undo."},
					},
				},
				{
					ID: "units", Name: "Units and measurements", Weight: 6,
					MustKnow:    "A quantity is specified without a clear unit, scale, prefix, or dimensional convention. Precise form: are all quantities' units and scales unambiguous — if not, which convention applies?",
					Question:    "Are there measurements involved, and which units should they be in?",
					Why:         "Every size, weight and distance the finished thing shows is written in these units, and changing them later means going back through every one.",
					Recommended: "planner_chooses",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses", Effect: "I take the units from your own words and from anything the project already uses, and say on the plan which ones I picked. Recommended: your request usually already contains the answer."},
						{Label: "Metric: millimetres, kilograms, degrees Celsius", Value: "metric", Effect: "Everything shown and everything typed in is metric, and anything arriving in other units is converted before it is displayed."},
						{Label: "Imperial: inches, pounds, degrees Fahrenheit", Value: "imperial", Effect: "Everything shown and everything typed in is imperial, with the same conversion rule the other way."},
						{Label: "There are no measurements in this", Value: "none", Effect: "Numbers are treated as plain counts and money, and nothing is converted."},
					},
				},
				{
					ID: "numerical_precision", Name: "Rounding", Weight: 6, Ask: intake.AskNever,
					MustKnow: "NEVER ASKED — settled internally and disclosed. Take rounding and precision from the kind of number involved: money to two decimal places in the currency the request implies, counts as whole numbers, percentages as the surrounding convention. Disclose it as a listed assumption in the requester's own terms (\"prices are shown as 12.34 euro\"). Raise it as a 1.7 single-question escalation only where the rounding rule genuinely changes what the person gets.",
				},
				{
					ID: "technology_stack", Name: "Technology choice", Weight: 11,
					MustKnow:    "Which language, framework, or platform the work should use is unstated — and everything else gets built on top of that choice, so a wrong one is expensive to undo later.",
					Question:    "What should this be built with?",
					Why:         "Everything else gets built on top of this, so it is the choice that is hardest to change later.",
					Recommended: "planner_chooses",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses", Effect: "I read what this project already contains and what the goal actually needs, pick from that, and name the choice on the plan before anything is built. Recommended: reading the project beats guessing at it, and you can still say no on the plan."},
						{Label: "Match whatever this project already uses", Value: "match_existing", Effect: "Nothing new is introduced — the work stays inside the tools already there, so it fits in from the first day and adds nothing to learn."},
						{Label: "The simplest thing that does the job", Value: "simplest", Effect: "Fewest moving parts, quickest to get working, easiest for you to run yourself — and least headroom if this grows later."},
						{Label: "I have something specific in mind and I will say what", Value: "specify", Effect: "The plan is built on exactly what you name, and says so plainly if any part of the goal does not fit it."},
					},
				},
				{
					ID: "assets_media", Name: "Pictures and other media", Weight: 10,
					MustKnow:    "Where the images, logos, or other media come from is unstated, so anything visible either stalls waiting for them or gets built around invented ones.",
					Question:    "Where should pictures, logos, and any other media come from?",
					Why:         "Anything people look at needs pictures from somewhere, and sorting that out late stalls the work.",
					Recommended: "placeholders",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses", Effect: "I pick a source that fits the look you asked for and name it on the plan before anything is built."},
						{Label: "I will supply the files", Value: "i_supply", Effect: "The work is built around your own images; until they arrive the pages hold marked stand-ins in the right shapes, so nothing waits."},
						{Label: "Use free images that are licensed for this", Value: "free_stock", Effect: "Real photographs, legally usable, copied into the project so nothing breaks when a website changes — but they will not be your actual products."},
						{Label: "Plain placeholders for now", Value: "placeholders", Effect: "Simple generated graphics stand in for every picture, so everything is built and checkable straight away. Recommended: nothing waits on files or licences, and swapping real pictures in later is a small job."},
					},
				},
				{
					ID: "look_feel", Name: "Look and feel", Weight: 10,
					MustKnow:    "The intended visual style is unstated, so anything the requester will actually look at gets built to somebody else's taste.",
					Question:    "How should it look?",
					Why:         "You are the one who will look at it, so this is your call rather than anyone else's taste.",
					Recommended: "planner_chooses",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses", Effect: "I take the style out of your own description of the project and show you the direction on the plan before anything is built. Recommended: this is the question your own words usually already answer."},
						{Label: "Plain and clean, nothing fancy", Value: "plain", Effect: "Nothing decorative — quick to build and easy to read, and it will not look like a designed product."},
						{Label: "Like something that already exists, and I will point at it", Value: "match_reference", Effect: "The layout, spacing and feel of what you point at are copied as closely as they can be, and the plan says which parts cannot be matched."},
						{Label: "I will describe the style I want", Value: "specify", Effect: "Your words become the style brief the finished work is checked against."},
					},
				},
				{
					ID: "language_locale", Name: "The language it speaks", Weight: 9,
					MustKnow:    "Which language the deliverable's own text is written in, and which locale conventions its dates, numbers and prices follow, is unstated; every seeded string, label, button and message depends on it. Precise form: name the content language(s) and the locale formatting conventions, and state whether the strings must be structured so a second language can be added later.",
					Question:    "What language should the finished thing speak to the people using it?",
					Why:         "Every word it shows is written in this language, so changing it afterwards means rewriting all of them.",
					Recommended: "english",
					Options: []intake.Option{
						{Label: "English", Value: "english", Effect: "All the visible text is English, written so a second language can be added later without moving anything around. Recommended: it is the safest single choice and it closes no door."},
						{Label: "The language I am writing to you in", Value: "my_language", Effect: "All the visible text is in the language of your request, and dates, numbers and prices follow that language's conventions."},
						{Label: "Two languages, with a switch", Value: "bilingual", Effect: "Every piece of text exists twice and whoever is using it can switch — roughly a third more work now, and every later change has to be made in both."},
						{Label: "I will name the languages", Value: "specify", Effect: "The plan is built for exactly the languages you name and says what each one adds to the work."},
					},
				},
				{
					ID: "quality_bar", Name: "What finished has to pass", Weight: 8,
					MustKnow:    "What the finished work must pass to count as done — feature-correctness alone, or a stated polish, performance or accessibility bar — is unstated. It decides what verification gates on: write the acceptance criteria against this answer. Precise form: name the checks the deliverable must pass and whether any of them are measured rather than judged.",
					Question:    "What does the finished thing have to pass before you would call it done?",
					Why:         "This is what the work is checked against at the end, so it decides what gets built along the way — a bar added afterwards means going back.",
					Recommended: "works_and_polished",
					Options: []intake.Option{
						{Label: "Everything I asked for works", Value: "feature_correct", Effect: "Checking asks one question per thing you asked for: does it do that? Quickest route to something usable, and nothing is judged on looks, speed or accessibility."},
						{Label: "Everything works, and it looks and behaves properly", Value: "works_and_polished", Effect: "On top of the feature checks, the finished thing is judged on readable text and contrast, nothing jumping around as it loads, and no errors running underneath. Recommended: it catches what you would notice in the first minute of using it, without a formal audit."},
						{Label: "It has to pass a named standard, and I will say which", Value: "audited", Effect: "The standard you name becomes part of the acceptance criteria and is measured on the built thing rather than assumed. Slower, and it can send back work that already does everything you asked for."},
					},
				},
			},
		},
	}
}
