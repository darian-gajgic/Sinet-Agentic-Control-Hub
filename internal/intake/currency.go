package intake

import (
	"regexp"
	"strings"
)

// The unstated-unit backstop (P3-GF14 R3). A live intake drafted a price list
// whose six prices reached the approval card as bare numbers — "price (8, 9, 9,
// 10, 8, 11 respectively)" — with the currency named nowhere: no open point, no
// listed assumption. Only the requester's own contest made them euros.
//
// The ask-don't-assume rule already covers it (Spec S06.5: a CONSEQUENTIAL
// ambiguity — one that would change an acceptance criterion — raises exactly
// one question, and a non-consequential one becomes a logged assumption), and
// S06.6's marker contract already has the machinery: a NEEDS-CLARIFICATION
// marker is either asked or converted to a listed assumption, and an artifact
// with open markers cannot reach approval. What was missing is the half that
// GUARANTEES it — the model had already missed the gap when the plan arrived,
// so nothing platform-side would ever have caught it.
//
// This is a pair-acceptance validator, not a spine stage: S06.7's spine list
// stays closed at (a)–(d), and semantic guards belong with the ratified
// validators (CONVENTIONS §60). The teaching half rides the planner's own
// schema, so the model raises it first and this only catches what got through.

// currencyMarker is the platform-authored open point. It is the marker TEXT and
// the question the card asks, so the two can never drift, and it is matched by
// identity when the card composes its option set.
const currencyMarker = "Which currency are these prices in?"

// moneyRuleID is the P47 rule whose lexicon defines money context. Reusing it
// keeps ONE money vocabulary in the tree: the rule file is the operator-editable
// place where "what counts as a price" is written down (Spec S06.3).
const moneyRuleID = "P47-1"

// currencyTokens are the ways a currency can already be named. A hit anywhere
// the platform can see means the fact is settled and the backstop never fires
// — a currency answered, assumed, or simply written in the request stays
// settled, permanently (CONVENTIONS §60/GF12: settled facts stay settled).
var currencyTokens = regexp.MustCompile(`(?i)[$€£¥₹]|\b(?:` +
	`eur|usd|gbp|chf|jpy|cad|aud|sek|nok|dkk|pln|cny|rmb|` +
	`euros?|dollars?|pounds?|sterling|francs?|yen|yuan|rupees?|kronor|krona|kroner|krone|zloty` +
	`)\b`)

// bareNumber is a standalone quantity: a digit run that no currency token is
// attached to (the currency-amount forms are stripped before this runs).
var bareNumber = regexp.MustCompile(`\d`)

// currencyGapStands reports whether the pair puts money numbers in front of the
// requester with no currency named anywhere the platform can see.
func (p *Pipeline) currencyGapStands(st *State, pair *Pair) bool {
	cues := p.moneyCues()
	if len(cues) == 0 {
		return false
	}
	if !moneyNumbersAreBare(pair, cues) {
		return false
	}
	return !currencyNamedAnywhere(st, pair)
}

// moneyCues compiles the money-context cue set from the P47 rule file's own
// prices-and-costs row. An absent row (an operator-edited file) yields no cues
// and the backstop stands down: it never invents a lexicon of its own.
func (p *Pipeline) moneyCues() []*regexp.Regexp {
	for _, r := range p.triggers().Rules {
		if r.ID != moneyRuleID {
			continue
		}
		out := make([]*regexp.Regexp, 0, len(r.Cues))
		for _, cue := range r.Cues {
			re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(cue) + `\b`)
			if err != nil {
				continue
			}
			out = append(out, re)
		}
		return out
	}
	return nil
}

// moneyNumbersAreBare reports whether any line the REQUESTER will read carries
// a money cue and a quantity with no currency on it. Only requester-facing
// content counts: the acceptance criteria, the outcome, and the plan steps are
// what the approval card shows and what the result is judged against.
func moneyNumbersAreBare(pair *Pair, cues []*regexp.Regexp) bool {
	lines := make([]string, 0, len(pair.Spec.ACs)+len(pair.Spec.Outcome)+len(pair.Plan.Steps))
	for _, ac := range pair.Spec.ACs {
		lines = append(lines, ac.Plain, ac.Structured)
	}
	lines = append(lines, pair.Spec.Outcome...)
	for _, s := range pair.Plan.Steps {
		lines = append(lines, s.Title, s.DoneWhen, s.Approach)
	}
	for _, line := range lines {
		if !anyMatch(cues, line) {
			continue
		}
		if bareNumber.MatchString(currencyTokens.ReplaceAllString(line, " ")) {
			return true
		}
	}
	return false
}

// currencyNamedAnywhere scans everything the platform holds about this task for
// a currency token: the request, the answers and resolutions on the record, the
// answered-marker record, the requester-supplied facts and escalations, the
// registered project's slice, and the pair itself.
func currencyNamedAnywhere(st *State, pair *Pair) bool {
	var b strings.Builder
	add := func(parts ...string) {
		for _, s := range parts {
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	add(st.Req.Title, st.Req.Text)
	for _, r := range st.Resolutions {
		add(r.Value, r.Assumption)
	}
	for _, m := range st.SettledMarkers {
		add(m.Marker, m.Answer)
	}
	for _, f := range st.Supplied {
		add(f.Fact)
	}
	for _, e := range st.Escalations {
		add(e.Question, e.Answer)
	}
	if st.Registry != nil {
		add(st.Registry.Conventions...)
		for _, v := range st.Registry.ResolvedSlots {
			add(v)
		}
	}
	add(pair.Spec.Restatement)
	add(pair.Spec.Outcome...)
	add(pair.Spec.Constraints...)
	add(pair.Spec.OutOfScope...)
	add(pair.Spec.Clarifications...)
	for _, ac := range pair.Spec.ACs {
		add(ac.Plain, ac.Structured)
	}
	for _, a := range pair.Spec.Assumptions {
		add(a.Text)
	}
	for _, f := range pair.Spec.Supplied {
		add(f.Fact)
	}
	for _, s := range pair.Plan.Steps {
		add(s.Title, s.DoneWhen, s.Approach)
	}
	add(pair.Plan.Risks...)
	return currencyTokens.MatchString(b.String())
}

func anyMatch(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// currencyOptions is the finite choice set the currency question carries: a
// small platform-authored list, free text always available beside it, and NO
// recommendation — the platform has no fact to recommend from. The last option
// is the honest skip: the prices stay exactly as they were supplied, and the
// choice lands on the approval card as a listed assumption like every other.
func currencyOptions() []Option {
	return []Option{
		{Label: "Euros (€)", Value: "euros (€)"},
		{Label: "US dollars ($)", Value: "US dollars ($)"},
		{Label: "British pounds (£)", Value: "British pounds (£)"},
		{Label: "Swiss francs (CHF)", Value: "Swiss francs (CHF)"},
		{
			Label:  "Leave the numbers as they are",
			Value:  "show the numbers exactly as supplied, with no currency on them",
			Effect: "The prices appear as plain numbers, with nothing saying what money they are in.",
		},
	}
}

// markerOptions offers a finite choice set for a marker the PLATFORM authored.
// A planner's own marker text carries none: the platform does not know the
// answer set for a question it did not write, and inventing one would be a
// guess wearing a choice list's clothes.
func markerOptions(marker string) []Option {
	if strings.TrimSpace(marker) == currencyMarker {
		return currencyOptions()
	}
	return nil
}

// markerWhy says why the platform raised its own marker, in plain words.
func markerWhy(marker string) string {
	if strings.TrimSpace(marker) == currencyMarker {
		return "The plan shows prices as plain numbers and nothing says what money they are in, so the list could come out in the wrong one."
	}
	return ""
}

// hasClarification reports whether the emission already carries this marker.
func hasClarification(markers []string, marker string) bool {
	for _, m := range markers {
		if strings.TrimSpace(m) == marker {
			return true
		}
	}
	return false
}
