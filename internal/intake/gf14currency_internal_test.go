package intake

// gf14currency_internal_test.go — P3-GF14 R3, the suppression PROPERTY: a
// currency named ANYWHERE the platform can see settles the question, and a
// settled fact is never asked again (CONVENTIONS §60/GF12). Stated over every
// scanned location rather than over the one the witnessed case happened to use.

import (
	"strings"
	"testing"
)

// pricedPair is the witnessed shape: money numbers on a criterion the requester
// will read, with no currency on them.
func pricedPair() *Pair {
	return &Pair{
		Spec: Spec{
			Restatement: "Requester wants: a one-page price list",
			Outcome:     []string{"a printable one-page list"},
			ACs: []AC{
				{N: 1, Plain: "the list shows every candle with its name and its price (8, 9, 9, 10, 8, 11 respectively)"},
			},
		},
		Plan: Plan{Steps: []Step{{ID: "S-1", Title: "Lay out the list", DoneWhen: "one page"}}},
	}
}

func pricedState() *State {
	return &State{
		TaskID: "t-candles", RunID: "t-candles.intake", Owner: "u1",
		Req: Request{UserID: "u1", Title: "A one-page price list for my candle shop",
			Text: "Six candles, each with a name and a price."},
	}
}

func TestGF14CurrencyBackstopFiresOnTheWitnessedShape(t *testing.T) {
	p := &Pipeline{}
	if !p.currencyGapStands(pricedState(), pricedPair()) {
		t.Fatal("bare money numbers with no currency anywhere must raise the open point (R3)")
	}
}

// TestGF14CurrencySuppressionHoldsOverEveryScannedLocation is the property: it
// does not matter WHERE the currency was named — request, answer, assumption,
// supplied fact, escalation, the registered project's slice, or the pair itself
// — the platform never asks about a fact it already holds.
func TestGF14CurrencySuppressionHoldsOverEveryScannedLocation(t *testing.T) {
	const token = "euros"
	locations := []struct {
		name string
		seed func(st *State, pair *Pair)
	}{
		{"the request text", func(st *State, _ *Pair) { st.Req.Text += " Prices are in " + token + "." }},
		{"the request title", func(st *State, _ *Pair) { st.Req.Title += " (" + token + ")" }},
		{"an answered slot", func(st *State, _ *Pair) {
			st.Resolutions = append(st.Resolutions, SlotResolution{SlotID: "s1", How: ResolvedAnswered, Value: token})
		}},
		{"a standing assumption", func(st *State, _ *Pair) {
			st.Resolutions = append(st.Resolutions, SlotResolution{SlotID: "s1", How: ResolvedAssumption, Assumption: "prices in " + token})
		}},
		{"an answered open point", func(st *State, _ *Pair) {
			st.SettledMarkers = append(st.SettledMarkers, SettledMarker{Marker: currencyMarker, Answer: token})
		}},
		{"a requester-supplied fact", func(st *State, _ *Pair) {
			st.Supplied = append(st.Supplied, SuppliedFact{RuleID: "P47-1", Fact: "the prices are in " + token})
		}},
		{"an answered escalation", func(st *State, _ *Pair) {
			st.Escalations = append(st.Escalations, EscalationAnswer{Question: "which money?", Answer: token})
		}},
		{"the registered project's conventions", func(st *State, _ *Pair) {
			st.Registry = &RegistrySlice{Project: "shop", Conventions: []string{"all prices in " + token}}
		}},
		{"a registry-supplied slot", func(st *State, _ *Pair) {
			st.Registry = &RegistrySlice{Project: "shop", ResolvedSlots: map[string]string{"currency": token}}
		}},
		{"the pair's own assumptions", func(_ *State, pair *Pair) {
			pair.Spec.Assumptions = append(pair.Spec.Assumptions, Assumption{Text: "prices shown in " + token})
		}},
		{"the pair's open points", func(_ *State, pair *Pair) {
			pair.Spec.Clarifications = append(pair.Spec.Clarifications, "confirm the "+token+" figures")
		}},
		{"the criterion itself", func(_ *State, pair *Pair) {
			pair.Spec.ACs[0].Plain = "the list shows every candle with its name and its price (€8, €9, €9, €10, €8, €11)"
		}},
		{"a plan step", func(_ *State, pair *Pair) {
			pair.Plan.Steps[0].Approach = "I write each price with the " + token + " sign in front of it."
		}},
		{"an ISO code rather than a word", func(st *State, _ *Pair) { st.Req.Text += " All figures CHF." }},
		{"a symbol rather than a word", func(st *State, _ *Pair) { st.Req.Text += " The cheapest is £8." }},
	}
	p := &Pipeline{}
	for _, loc := range locations {
		t.Run(loc.name, func(t *testing.T) {
			st, pair := pricedState(), pricedPair()
			if !p.currencyGapStands(st, pair) {
				t.Fatal("non-vacuity: the unseeded fixture must raise the point, or this proves nothing")
			}
			loc.seed(st, pair)
			if p.currencyGapStands(st, pair) {
				t.Fatal("the platform asked for a currency it already holds — a settled fact stays settled")
			}
		})
	}
}

// TestGF14CurrencyBackstopStaysOutOfUnpricedWork pins the other direction: the
// backstop is about MONEY, so a plan with numbers and no money context is left
// entirely alone.
func TestGF14CurrencyBackstopStaysOutOfUnpricedWork(t *testing.T) {
	p := &Pipeline{}
	st := pricedState()
	st.Req.Title, st.Req.Text = "Fix the widget", "Please fix the widget in the repo."
	pair := pricedPair()
	pair.Spec.ACs = []AC{{N: 1, Plain: "the widget renders 6 rows and the tests pass"}}
	pair.Spec.Restatement = "Requester wants: the widget fixed"
	pair.Spec.Outcome = []string{"the widget works"}
	if p.currencyGapStands(st, pair) {
		t.Fatal("a plan with no money in it must never be asked about a currency")
	}
}

// TestGF14CurrencyQuestionIsOfferedInPlainWords pins the served content: a
// finite choice set, an honest leave-it-as-it-is arm, and no citation soup.
func TestGF14CurrencyQuestionIsOfferedInPlainWords(t *testing.T) {
	opts := markerOptions(currencyMarker)
	if len(opts) < 2 {
		t.Fatalf("the currency question offers %d options, want a finite choice set", len(opts))
	}
	if markerOptions("some planner's own open point") != nil {
		t.Fatal("a marker the platform did not author must carry no invented option set")
	}
	leave := opts[len(opts)-1]
	if !strings.Contains(strings.ToLower(leave.Value), "no currency") || leave.Effect == "" {
		t.Fatalf("the last arm must be the honest leave-them-bare choice with its consequence: %+v", leave)
	}
	for _, s := range append([]string{currencyMarker, markerWhy(currencyMarker)}, leave.Label, leave.Effect) {
		for _, token := range []string{"S06", "P47", "§", "Spec/", "NEEDS-CLARIFICATION"} {
			if strings.Contains(s, token) {
				t.Errorf("served text carries the platform's own vocabulary (%q): %q", token, s)
			}
		}
	}
}
