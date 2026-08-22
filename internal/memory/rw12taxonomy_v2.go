package memory

import (
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// rw12taxonomy_v2.go — the six interview question sets EXACTLY as the
// P3-RW-12 gate ratified them (P3/STATE.md, the P3-RW-12 landing entry,
// operator, 2026-08-13): software at v2, research/content/data/chore at v1,
// generic at the v1 the B2 gate approved and RW-12 reviewed unchanged.
//
// It is a frozen SNAPSHOT, and it is here for the reason b2taxonomy_v1.go
// states: a ratification record that follows the code is not a record.
// EnsureRW12TaxonomyGovernance used to marshal whatever intake.SeedTaxonomies()
// returned today, which was honest exactly while nobody edited a question set.
// P3-GF3-BE1 edits two of them (software and generic move to v3), so without
// this file the next boot of any world would write v3 content under the RW-12
// originRef — a governed row attesting a gate that never read it — and
// rw12ContentDigest would stop being true of anything.
//
// The digests are NOT edited to match: they pin what the RW-12 gate ratified,
// and this file is the content they pin. The v3 content enters the record the
// only honest way, as a SUPERSESSION under its own provenance
// (EnsureGF3TaxonomyGovernance, gf3seeds.go) — S09.8: a new version, never an
// in-place edit.
//
// Generated once from the RW-12 seeds and frozen; the digest tripwire
// (TestRW12DigestsMatchTheShippedSeeds) is what keeps it honest.

// rw12TaxonomySnapshot returns the RW-12-ratified question sets.
func rw12TaxonomySnapshot() map[intake.Family]*intake.Taxonomy {
	return map[intake.Family]*intake.Taxonomy{
		intake.FamilySoftware: {
			ID:      "software",
			Family:  intake.FamilySoftware,
			Version: "v2",
			Source:  "ClarifyCodeBench 10-type taxonomy (arXiv:2607.00711, Table 2) for the ten clarity slots, seeded P3-B2-2 — weights EVIDENCE-INFORMED from that benchmark: the types every model measurably fails (collection semantics, comparison rules, ordering & atomicity) weigh 12, ordinary clarity types 10/8, the types models natively handle (units, numerical precision) weigh 6. The three Deep-Plan slots (technology_stack 11, assets_media 10, look_feel 10) are REASONED, not measured: for a requester who is not a programmer they shape the deliverable more than output format or units do, so they sit above the natively-handled types — and strictly below the benchmark-failed 12, because reasoning does not outrank measurement. Deep-Plan revision drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; 8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate",
			Slots: []intake.Slot{
				{
					ID: "behavior", Name: "Behavior", Weight: 10,
					MustKnow: "The required function, objective, or side effect is underspecified, so the intended behavior is not uniquely determined.",
					Question: "What exactly should this do — what behavior or outcome makes it correct?",
				},
				{
					ID: "terminology", Name: "Terminology", Weight: 10,
					MustKnow: "A domain term, action, or state is undefined, overloaded, or open to multiple interpretations.",
					Question: "Are there terms in the request that need pinning down — what do they mean here?",
				},
				{
					ID: "edge_cases", Name: "Edge Cases", Weight: 10,
					MustKnow: "Boundary or exceptional conditions are not specified, leaving behavior unclear for special inputs.",
					Question: "How should boundary and exceptional inputs be handled (empty, missing, malformed, extreme)?",
					Options: []intake.Option{
						{Label: "Fail loudly on anything unexpected", Value: "fail_loud"},
						{Label: "Handle gracefully with sensible defaults", Value: "graceful"},
						{Label: "I'll specify per case", Value: "specify"},
					},
				},
				{
					ID: "collection_semantics", Name: "Collection Semantics", Weight: 12,
					MustKnow: "A collection, container, or state object is mentioned, but its membership, update rule, or access semantics are underspecified.",
					Question: "For the collections/state involved: what belongs in them, and how are they updated or accessed?",
				},
				{
					ID: "comparison_rules", Name: "Comparison Rules", Weight: 12,
					MustKnow: "The comparison key, tie-breaking rule, or stability requirement is not specified.",
					Question: "Where things are compared, sorted, or deduplicated: by what key, and how are ties broken?",
				},
				{
					ID: "ordering_atomicity", Name: "Ordering & Atomicity", Weight: 12,
					MustKnow: "Temporal order, simultaneity, or indivisible execution assumptions are unclear.",
					Question: "Does order of operations or atomicity matter here — what must happen before what, and what must never interleave?",
				},
				{
					ID: "indices_ranges", Name: "Indices & Ranges", Weight: 8,
					MustKnow: "Index bases, interval boundaries, or inclusion rules are underspecified.",
					Question: "For indices and ranges: zero- or one-based, and are boundaries inclusive or exclusive?",
					Options: []intake.Option{
						{Label: "Zero-based, end-exclusive (language-conventional)", Value: "zero_exclusive"},
						{Label: "One-based, inclusive", Value: "one_inclusive"},
						{Label: "I'll specify", Value: "specify"},
					},
				},
				{
					ID: "output_format", Name: "Output Format", Weight: 8,
					MustKnow: "The required output structure, layout, or presentation rule is missing or unclear.",
					Question: "What shape should the output take?",
					Options: []intake.Option{
						{Label: "Match the existing/conventional format", Value: "conventional"},
						{Label: "Exact format matters — I'll specify", Value: "specify"},
						{Label: "Any clear format is fine", Value: "any"},
					},
				},
				{
					ID: "units", Name: "Units", Weight: 6,
					MustKnow: "A quantity is specified without a clear unit, scale, prefix, or dimensional convention.",
					Question: "Are all quantities' units and scales unambiguous — if not, which convention applies?",
				},
				{
					ID: "numerical_precision", Name: "Numerical Precision", Weight: 6,
					MustKnow: "Precision, rounding, tolerance, or error handling requirements are unclear.",
					Question: "Do precision, rounding, or tolerance requirements apply to numeric results?",
				},
				{
					ID: "technology_stack", Name: "Technology choice", Weight: 11,
					MustKnow: "Which language, framework, or platform the work should use is unstated — and everything else gets built on top of that choice, so a wrong one is expensive to undo later.",
					Question: "What should this be built with?",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Match whatever this project already uses", Value: "match_existing"},
						{Label: "The simplest thing that does the job", Value: "simplest"},
						{Label: "I have something specific in mind — I'll say what", Value: "specify"},
					},
				},
				{
					ID: "assets_media", Name: "Pictures and other media", Weight: 10,
					MustKnow: "Where the images, logos, or other media come from is unstated, so anything visible either stalls waiting for them or gets built around invented ones.",
					Question: "Where should pictures, logos, and any other media come from?",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "I'll supply the files", Value: "i_supply"},
						{Label: "Use free images that are licensed for this", Value: "free_stock"},
						{Label: "Plain placeholders for now", Value: "placeholders"},
					},
				},
				{
					ID: "look_feel", Name: "Look and feel", Weight: 10,
					MustKnow: "The intended visual style is unstated, so anything the requester will actually look at gets built to somebody else's taste.",
					Question: "How should it look?",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Plain and clean, nothing fancy", Value: "plain"},
						{Label: "Like something that already exists — I'll point at it", Value: "match_reference"},
						{Label: "I'll describe the style I want", Value: "specify"},
					},
				},
			},
		},
		intake.FamilyResearch: {
			ID:      "research",
			Family:  intake.FamilyResearch,
			Version: "v1",
			Source:  "Deep-Plan must-know set for the research family. Slots and weights are REASONED — no benchmark of research-request ambiguity comparable to ClarifyCodeBench exists, and none is implied here. Ordering rationale, slot by slot: the question itself (12) and the decision it serves (11) lead, because an answer aimed at the wrong question is worthless however well sourced; depth (10) is next, since it is the single biggest lever on what the work costs; sources, recency and the form of the answer share 9 — the first two decide whether the answer is TRUE today, the third is weighted with them because a research result delivered in the wrong shape gets re-done; balance and the requester's own prior belief share 8 as the guards against one-sided answers and against researching a false premise as though it were established; the edges of the question (7), the reader (6) and practical limits (5) shape delivery only. drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; 8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate",
			Slots: []intake.Slot{
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
					Options: []intake.Option{
						{Label: "A quick scan — the gist, fast", Value: "quick"},
						{Label: "A solid answer with the main sources checked", Value: "solid"},
						{Label: "As thorough as it takes, awkward corners included", Value: "thorough"},
					},
				},
				{
					ID: "sources", Name: "Sources to use or avoid", Weight: 9,
					MustKnow: "Which sources count as trustworthy here, and which must be left out, is unstated.",
					Question: "Are there sources you trust — or sources you do not want this built on?",
					Options: []intake.Option{
						{Label: "You choose the sources and show me which ones you used", Value: "planner_chooses"},
						{Label: "Official or first-hand sources only", Value: "primary_only"},
						{Label: "I'll name the ones to use or avoid", Value: "specify"},
					},
				},
				{
					ID: "recency", Name: "How current it must be", Weight: 9,
					MustKnow: "Whether older material still counts is unstated — and something out of date reads exactly like something current.",
					Question: "How current does this have to be?",
					Options: []intake.Option{
						{Label: "You choose and tell me the cut-off you used", Value: "planner_chooses"},
						{Label: "Only the last year or so", Value: "last_year"},
						{Label: "Recent enough to still hold — a few years is fine", Value: "few_years"},
						{Label: "Age doesn't matter for this", Value: "any_age"},
					},
				},
				{
					ID: "output_form", Name: "Form of the answer", Weight: 9,
					MustKnow: "The shape the answer should arrive in is unstated.",
					Question: "What should the answer look like when it lands?",
					Options: []intake.Option{
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
					Options: []intake.Option{
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
		},
		intake.FamilyContent: {
			ID:      "content",
			Family:  intake.FamilyContent,
			Version: "v1",
			Source:  "Deep-Plan must-know set for the content family. Slots and weights are REASONED, not benchmark-derived. Ordering rationale: the wrong kind of piece, the wrong reader, or a missing key message fails the whole piece (12/11); purpose, tone and length are what a reader notices first (10/10/9); references, media and look shape presentation (8/8/7); review expectations and forbidden claims are guards on the finish (7/6). Asset and look-and-feel questions appear here because the 2026-08-12 operator directive names them beyond the software family. drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; 8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate",
			Slots: []intake.Slot{
				{
					ID: "piece_type", Name: "What you want made", Weight: 12,
					MustKnow: "What kind of piece this is, and where it will be published or used, is unstated — and that one fact sets length, form and register together.",
					Question: "What kind of piece is this, and where will it be used?",
					Options: []intake.Option{
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
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Plain and straightforward", Value: "plain"},
						{Label: "Warm and personal", Value: "warm"},
						{Label: "Formal and professional", Value: "formal"},
					},
				},
				{
					ID: "length_format", Name: "Length and shape", Weight: 9,
					MustKnow: "How long the piece should be, and whether it needs a set structure, is unstated.",
					Question: "How long should it be, and does it need a particular structure?",
					Options: []intake.Option{
						{Label: "You choose a sensible length and show me", Value: "planner_chooses"},
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
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "I'll supply the files", Value: "i_supply"},
						{Label: "Use free images that are licensed for this", Value: "free_stock"},
						{Label: "Words only, no media", Value: "none"},
					},
				},
				{
					ID: "look_feel", Name: "Look and feel", Weight: 7,
					MustKnow: "How the piece should be presented is unstated, and anything published somewhere visible gets judged on how it looks.",
					Question: "How should it look on the page?",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Match the style of wherever it's going", Value: "match_home"},
						{Label: "Plain text, no styling needed", Value: "plain_text"},
						{Label: "I'll describe the look I want", Value: "specify"},
					},
				},
				{
					ID: "review", Name: "How you'll review it", Weight: 7,
					MustKnow: "How many rounds of review the requester expects, and who signs it off, is unstated.",
					Question: "How do you want to review this?",
					Options: []intake.Option{
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
		},
		intake.FamilyData: {
			ID:      "data",
			Family:  intake.FamilyData,
			Version: "v1",
			Source:  "Deep-Plan must-know set for the data family. Slots and weights are REASONED, not benchmark-derived. Ordering rationale: without the source and the wanted transformation there is no task at all (12/12); access, output form, the correctness bar, the missing-data posture and privacy each silently change what every number means (10/10/9/9/9); which slice and what the fields mean guard against confident nonsense (8/8); repeatability and tooling shape the build (6/6). drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; 8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate",
			Slots: []intake.Slot{
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
					Options: []intake.Option{
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
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "A spreadsheet or table", Value: "table"},
						{Label: "A short written answer", Value: "written"},
						{Label: "Charts, or a small report", Value: "charts"},
					},
				},
				{
					ID: "correctness_bar", Name: "How sure it has to be", Weight: 9,
					MustKnow: "How much checking the numbers need is unstated, and the cost of a quiet error varies enormously with what they are used for.",
					Question: "How certain do these numbers have to be?",
					Options: []intake.Option{
						{Label: "Roughly right is fine — I'll sanity-check it", Value: "rough"},
						{Label: "Spot-checked against the source", Value: "spot_checked"},
						{Label: "Checked carefully — real decisions ride on it", Value: "validated"},
					},
				},
				{
					ID: "messy_data", Name: "Gaps and mess", Weight: 9,
					MustKnow: "How to treat missing, duplicated, or plainly wrong entries is unstated — and whichever rule gets picked silently changes every number downstream.",
					Question: "What should happen with missing, duplicated, or obviously wrong entries?",
					Options: []intake.Option{
						{Label: "You choose a sensible rule and tell me what you did", Value: "planner_chooses"},
						{Label: "Leave gaps as gaps — never guess", Value: "leave_gaps"},
						{Label: "Drop the incomplete rows", Value: "drop"},
						{Label: "Ask me whenever it matters", Value: "ask"},
					},
				},
				{
					ID: "privacy", Name: "Private or sensitive contents", Weight: 9,
					MustKnow: "Whether the data carries personal or confidential contents is unstated, and it constrains where the work may run and what may appear in the result.",
					Question: "Does the data contain anything personal or confidential?",
					Options: []intake.Option{
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
					Options: []intake.Option{
						{Label: "Just this once", Value: "one_off"},
						{Label: "I'll want to run it again on new data", Value: "repeatable"},
						{Label: "It should run on a schedule", Value: "scheduled"},
					},
				},
				{
					ID: "tooling", Name: "What to do it with", Weight: 6,
					MustKnow: "Whether a particular tool or format is required at the other end is unstated.",
					Question: "Any preference for what this is done with?",
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Keep it in a spreadsheet", Value: "spreadsheet"},
						{Label: "Whatever copes with the size best", Value: "size_appropriate"},
						{Label: "I have a tool in mind — I'll say which", Value: "specify"},
					},
				},
			},
		},
		intake.FamilyChore: {
			ID:      "chore",
			Family:  intake.FamilyChore,
			Version: "v1",
			Source:  "Deep-Plan must-know set for the chore family. Slots and weights are REASONED, not benchmark-derived. Ordering rationale specific to unattended repetition, slot by slot: with no per-run definition of done, no run can be judged at all (12); the trigger decides when it happens (11); the procedure of one run and the inputs that vary from run to run share 10, because a chore built around values fixed at planning time does the wrong thing quietly from run two onward; the success signal and the failure posture share 9, and the notification preference and the touch scope share 8 — all four weighted above their one-off equivalents, because an unheard failure and an outward effect repeated on a schedule are the characteristic chore harms; the stop condition (6) and what record it keeps (5) trail. drafted P3-RW-12 with claude-opus-5 on 2026-08-13 per Spec S06.5; 8.3-gate entry as a governed S09.10 house object, ratified by the operator at the P3-RW-12 packet gate",
			Slots: []intake.Slot{
				{
					ID: "done_looks_like", Name: "What done looks like", Weight: 12,
					MustKnow: "What a finished run looks like is unstated, so nothing can tell a good run from a bad one.",
					Question: "Each time this runs, what has to be true at the end for it to count as done?",
				},
				{
					ID: "trigger_schedule", Name: "When it runs", Weight: 11,
					MustKnow: "What sets the chore off — a clock, an event, or a request — is unstated.",
					Question: "When should this run?",
					Options: []intake.Option{
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
					Options: []intake.Option{
						{Label: "Try again, then tell me if it still fails", Value: "retry_then_notify"},
						{Label: "Stop straight away and tell me", Value: "stop_notify"},
						{Label: "Skip that run and carry on next time", Value: "skip"},
					},
				},
				{
					ID: "notification", Name: "How much you want to hear", Weight: 8,
					MustKnow: "How much reporting the requester wants is unstated: too much becomes noise that hides the failures, too little hides them outright.",
					Question: "How much do you want to hear from it?",
					Options: []intake.Option{
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
					Options: []intake.Option{
						{Label: "You choose for me and show me what you picked", Value: "planner_chooses"},
						{Label: "Keep a log of every run", Value: "log_all"},
						{Label: "Only keep the failures", Value: "failures_only"},
					},
				},
			},
		},
		intake.FamilyGeneric: {
			ID:      "generic",
			Family:  intake.FamilyGeneric,
			Version: "v1",
			Source:  "Generic fallback must-know set; drafted P3-B2-2 per Spec S06.5 TBD-P3; 8.3-gate entry pending S09 (B3)",
			Slots: []intake.Slot{
				{
					ID: "goal", Name: "Goal", Weight: 12,
					MustKnow: "The outcome that makes the task done is not uniquely determined.",
					Question: "What outcome makes this done — what will you look at to say it worked?",
				},
				{
					ID: "deliverable", Name: "Deliverable", Weight: 10,
					MustKnow: "The form the result should take is unspecified.",
					Question: "What form should the result take?",
					Options: []intake.Option{
						{Label: "A document or write-up", Value: "document"},
						{Label: "A working change (code, config, files)", Value: "change"},
						{Label: "A recommendation with reasoning", Value: "recommendation"},
						{Label: "Something else — I'll describe", Value: "other"},
					},
				},
				{
					ID: "scope", Name: "Scope", Weight: 10,
					MustKnow: "What is in and out of scope is unstated.",
					Question: "Where are the edges — what should this explicitly include, and what should it not touch?",
				},
				{
					ID: "inputs", Name: "Inputs & sources", Weight: 8,
					MustKnow: "Which materials, sources, or prior work the task should build on is unclear.",
					Question: "What should this be based on — specific files, sources, or prior work?",
				},
				{
					ID: "constraints", Name: "Constraints", Weight: 8,
					MustKnow: "Hard constraints (tools, style, budget, privacy, compatibility) are unstated.",
					Question: "Any hard constraints — tools to use or avoid, style, budget, privacy?",
				},
				{
					ID: "audience", Name: "Audience", Weight: 6,
					MustKnow: "Who the result is for, and where it will be used, is unclear.",
					Question: "Who is this for, and where will it be used?",
				},
				{
					ID: "quality_bar", Name: "Quality bar", Weight: 6,
					MustKnow: "The intended thoroughness of the work is unstated.",
					Question: "How thorough should this be?",
					Options: []intake.Option{
						{Label: "Quick and rough is fine", Value: "quick"},
						{Label: "Solid, normal quality", Value: "normal"},
						{Label: "As thorough as it takes", Value: "thorough"},
					},
				},
				{
					ID: "deadline", Name: "Timing", Weight: 4,
					MustKnow: "Whether timing matters is unstated.",
					Question: "Is there a deadline or preferred timing?",
				},
			},
		},
	}
}
