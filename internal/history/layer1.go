package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
)

// layer1.go — S14.10 ¶2: "the local model classifies intent and fills slots,
// grammar-constrained [XREF:S12]; answers render with the query named. This is
// the reliability floor."
//
// The floor is structural, in three pieces:
//
//   - RunQuery executes a catalog entry with bound slot values and NO model in
//     the path at all. A caller that already knows which question it is asking
//     never touches the local tier.
//   - Ask adds intent classification on duty alias `intent-filling` (S12.4:
//     fast tier, grammar-constrained slot types). Its failure mode is FIXED by
//     that row: "below threshold ⇒ a 'which of these did you mean?' card —
//     never a guess." Every path that cannot resolve confidently produces the
//     card, and no path guesses.
//   - Every answer carries its query name, because newAnswer is the only
//     constructor and it takes the name.
//
// The classification is TWO grammar-constrained calls, not one: choosing among
// ~51 named queries and filling a chosen query's typed slots are different
// grammars, and a single schema would have to admit every slot of every query
// — which is precisely the un-typed shape S14.10 ¶2 rules out. Both schemas put
// the required free-text `reason` FIRST in property order and carry an explicit
// abstain member (S12.4; internal/local/schema.go's F5 finding, and duty.go's
// assertAbstain is the belt).

// Duty-call structural constants (S18 ratifies no key; §35 precedent).
const (
	// intentMaxTokens caps the intent call. Its reason: the answer is one enum
	// value plus a sentence of reasoning; the cap bounds a runaway generation,
	// never a legitimate answer.
	intentMaxTokens = 400
	// slotMaxTokens caps the slot-fill call, for the same reason over a handful
	// of short string values.
	slotMaxTokens = 400
	// disambiguationChoices bounds a card. Its reason: a card a person cannot
	// read is not a disambiguation, it is the catalog. The full catalog remains
	// available through Catalog().
	disambiguationChoices = 8
)

// intentLabelField is the schema field the S12.5 margin is taken over — the
// routing-decisive label of this duty (the one that picks the query).
const intentLabelField = "query"

// RunQuery is the Layer-1 verb with no model in the path: execute a NAMED
// catalog query with bound slot values, owner-scoped per S01.9.
//
// Slot values are BOUND, never formatted into the statement. An unknown slot
// name or a missing declared slot is an error rather than a silent empty
// filter, because a filter that silently did not apply returns a WIDER answer
// than the caller asked for.
func (s *Store) RunQuery(ctx context.Context, name string, slots map[string]string, scope Scope, limit int) (Answer, error) {
	q, ok := QueryByName(name)
	if !ok {
		return Answer{}, fmt.Errorf("%w: %q", ErrNoSuchQuery, name)
	}
	a := newAnswer(LayerCanned, q.Name)
	a.Notes = append(a.Notes, q.Description)

	args := make([]any, 0, len(q.Slots)+3)
	for _, sl := range q.Slots {
		v, ok := slots[sl.Name]
		if !ok {
			return Answer{}, fmt.Errorf("%w: query %q needs slot %q (%s)", ErrSlot, q.Name, sl.Name, sl.Type)
		}
		args = append(args, v)
	}
	for supplied := range slots {
		if !q.hasSlot(supplied) {
			return Answer{}, fmt.Errorf("%w: query %q has no slot %q", ErrSlot, q.Name, supplied)
		}
	}
	limit = clampLimit(limit)
	args = append(args, scope.scopeArg(), scope.scopeArg(), limit+1)

	if err := s.query(ctx, &a, q.SQL, args, limit); err != nil {
		return Answer{}, err
	}
	return a, nil
}

func (q Query) hasSlot(name string) bool {
	for _, sl := range q.Slots {
		if sl.Name == name {
			return true
		}
	}
	return false
}

// Ask is the Layer-1 natural-language entry point: classify the question's
// intent onto a catalog query, fill that query's typed slots, and run it.
//
// It NEVER guesses. Every unresolved path — no local stack, no advisory run,
// an abstain, an unknown query name, a below-threshold margin, an unfilled
// required slot — returns the disambiguation card, and the card names real
// catalog queries the asker can pick from.
func (s *Store) Ask(ctx context.Context, question string, scope Scope, limit int) (Answer, error) {
	intent, err := s.classify(ctx, question)
	if err != nil {
		return Answer{}, err
	}
	if intent.Card != nil {
		return *intent.Card, nil
	}
	a, err := s.RunQuery(ctx, intent.Query, intent.Slots, scope, limit)
	if err != nil {
		if errors.Is(err, ErrSlot) || errors.Is(err, ErrNoSuchQuery) {
			// The model produced a shape the catalog does not accept. That is a
			// failure to resolve, which has exactly one honest outcome.
			return cardAnswer(question,
				"the question could not be matched to a catalog query with the slots it needs: "+err.Error(),
				choicesForCard(topChoices(question))), nil
		}
		return Answer{}, err
	}
	a.Question = question
	if intent.Model != "" {
		a.Notes = append(a.Notes, "intent classified by "+intent.Model+" on duty alias "+local.AliasIntentFilling)
	}
	if intent.HadMargin {
		a.Notes = append(a.Notes, fmt.Sprintf("intent label margin %.3f", intent.Margin))
	}
	return a, nil
}

// intent is one resolved (or unresolved) classification.
type intent struct {
	Query     string
	Slots     map[string]string
	Margin    float64
	HadMargin bool
	Model     string
	// Card is non-nil exactly when the intent did NOT resolve.
	Card *Answer
}

// classify runs the two grammar-constrained duty calls.
func (s *Store) classify(ctx context.Context, question string) (intent, error) {
	names := CatalogNames()

	if s.duty == nil || s.advisory == nil {
		card := cardAnswer(question,
			"the local tier is not wired here, so intent cannot be classified — pick a query by name (the catalog is the reliability floor and is always available)",
			choicesForCard(topChoices(question)))
		return intent{Card: &card}, nil
	}

	runID, settle := s.advisory(ctx, "history-intent")
	if settle != nil {
		defer settle()
	}

	res, err := s.duty.Call(ctx, runID, local.DutyRequest{
		Alias:          local.AliasIntentFilling,
		System:         intentSystem,
		User:           intentPrompt(question, names),
		Schema:         IntentSchema(names),
		Name:           "history_intent",
		MaxTokens:      intentMaxTokens,
		Classification: true,
	})
	if err != nil {
		// An absent or refusing stack is not a failure of the question. The
		// floor answers.
		card := cardAnswer(question,
			"intent classification is unavailable ("+err.Error()+") — pick a query by name",
			choicesForCard(topChoices(question)))
		return intent{Card: &card}, nil
	}

	var picked struct {
		Reason  string `json:"reason"`
		Query   string `json:"query"`
		Abstain bool   `json:"abstain"`
	}
	if err := json.Unmarshal([]byte(res.Content), &picked); err != nil {
		card := cardAnswer(question,
			"the intent classifier did not return a usable answer — pick a query by name",
			choicesForCard(topChoices(question)))
		return intent{Card: &card}, nil
	}

	in := intent{Query: picked.Query, Model: res.Model, Slots: map[string]string{}}
	if m, ok := local.LabelMargin(res.Logprobs, []string{intentLabelField}); ok {
		in.Margin, in.HadMargin = m, true
	}

	if picked.Abstain || picked.Query == "" || picked.Query == abstainMember {
		card := cardAnswer(question, cardReason(picked.Reason,
			"the classifier abstained rather than guess a query"),
			choicesForCard(topChoices(question)))
		in.Card = &card
		return in, nil
	}
	q, ok := QueryByName(picked.Query)
	if !ok {
		card := cardAnswer(question, cardReason(picked.Reason,
			"the classifier named a query that is not in the catalog"),
			choicesForCard(topChoices(question)))
		in.Card = &card
		return in, nil
	}

	// The lexical no-match floor (P3-RW-18 D4-R2). The picked query must share
	// at least one content word with the question. It is a STRUCTURAL check on
	// relatedness, not a second classifier and not a confidence judgement: it
	// reads no logprob, invents no threshold, and applies whether the seat is
	// calibrated or not — so it is clean under S12.5, which forbids fabricating
	// a threshold for an uncalibrated seat, and it delivers S12.4's fixed
	// failure mode ("never a guess") on the one case a margin cannot catch.
	//
	// The walk is why it is lexical: the mismatch that sent "what got cancelled
	// recently and why?" to cost.budget_remainder came back with margin 8.575 —
	// HIGH and wrong. A margin floor would have passed it. Sharing a word is a
	// weak test on purpose; it catches only picks that are about something else
	// entirely, which is exactly the failure the reader hit.
	if !sharesQuestionVocabulary(question, q) {
		card := cardAnswer(question, cardReason(picked.Reason,
			"no catalog query matches the question's own words — here is what I do have"),
			choicesForCard(topChoices(question)))
		in.Card = &card
		return in, nil
	}

	// The S12.5 margin gate. `intent-filling@4B` is one of the calibrated
	// duties, so the threshold is real platform data. UNCALIBRATED is the
	// honest posture (the internal/local ReChecker precedent): no gate, the
	// answer stands, and the abstain member above still applies.
	if s.cal != nil && in.HadMargin {
		key := local.CalibrationKey{
			Duty:        local.AliasIntentFilling,
			ModelHash:   res.Seat.ModelHash(),
			EngineBuild: local.LlamaCppPin,
		}
		cal, found, err := s.cal.Calibration(ctx, key)
		if err != nil {
			return intent{}, fmt.Errorf("history: read intent calibration: %w", err)
		}
		if found && !cal.AcceptLocal(in.Margin) {
			card := cardAnswer(question, fmt.Sprintf(
				"the classifier's confidence was below the calibrated threshold for %s (margin %.3f) — which of these did you mean?",
				local.AliasIntentFilling, in.Margin),
				choicesForCard(nearby(q.Name)))
			in.Card = &card
			return in, nil
		}
	}

	if len(q.Slots) == 0 {
		return in, nil
	}

	// Second grammar: fill THIS query's typed slots.
	slotRes, err := s.duty.Call(ctx, runID, local.DutyRequest{
		Alias:          local.AliasIntentFilling,
		System:         slotSystem,
		User:           slotPrompt(question, q),
		Schema:         SlotSchema(q.Slots),
		Name:           "history_slots",
		MaxTokens:      slotMaxTokens,
		Classification: true,
	})
	if err != nil {
		card := cardAnswer(question,
			"the query needs values that could not be filled ("+err.Error()+") — supply them by name",
			choicesForCard([]string{q.Name}))
		in.Card = &card
		return in, nil
	}
	var filled map[string]any
	if err := json.Unmarshal([]byte(slotRes.Content), &filled); err != nil {
		card := cardAnswer(question, "the slot filler did not return a usable answer",
			choicesForCard([]string{q.Name}))
		in.Card = &card
		return in, nil
	}
	if ab, _ := filled["abstain"].(bool); ab {
		card := cardAnswer(question,
			"the question did not say what to filter on, and the filler abstained rather than invent a value",
			choicesForCard([]string{q.Name}))
		in.Card = &card
		return in, nil
	}
	for _, sl := range q.Slots {
		v, _ := filled[sl.Name].(string)
		if strings.TrimSpace(v) == "" {
			card := cardAnswer(question, fmt.Sprintf(
				"%s needs a value for %q (%s), and the question did not supply one", q.Name, sl.Name, sl.Type),
				choicesForCard([]string{q.Name}))
			in.Card = &card
			return in, nil
		}
		in.Slots[sl.Name] = v
	}
	return in, nil
}

// abstainMember is the sanctioned abstain spelling carried in the intent enum,
// so the model can decline inside the grammar rather than outside it.
const abstainMember = "abstain"

// IntentSchema is the grammar for choosing a catalog query: the required
// free-text `reason` FIRST in property order (so the model's reasoning is the
// leading token region and the label follows), then the query enum over the
// catalog names plus the abstain member, then the explicit abstain flag.
//
// It is built through internal/local's ordered-schema helper so property ORDER
// is preserved — a Go map marshals alphabetically, which would reorder the
// grammar (the F5 finding, confirmed live).
func IntentSchema(names []string) json.RawMessage {
	picks := append(append([]string(nil), names...), abstainMember)
	return local.BatterySchema([]local.LabelField{{Name: intentLabelField, Enum: picks}})
}

// SlotSchema is the grammar for filling one query's typed slots: `reason`
// first, one string field per slot, then the abstain member.
func SlotSchema(slots []Slot) json.RawMessage {
	fields := make([]local.LabelField, 0, len(slots))
	for _, sl := range slots {
		fields = append(fields, local.LabelField{Name: sl.Name})
	}
	return local.BatterySchema(fields)
}

const intentSystem = `You match a question about a platform's own history to ONE named query from a fixed catalog.
You do not answer the question. You do not write SQL. You only choose a query name from the list.
Fill the free-text reason FIRST, then choose.
If no catalog query answers the question, or you are unsure which one does, set abstain to true. Abstaining is correct and expected; guessing is not.`

func intentPrompt(question string, names []string) string {
	var b strings.Builder
	b.WriteString("THE QUESTION:\n")
	b.WriteString(question)
	b.WriteString("\n\nTHE CATALOG (name — what it answers):\n")
	for _, n := range names {
		if q, ok := QueryByName(n); ok {
			fmt.Fprintf(&b, "  %s [%s] — %s\n", q.Name, q.Category, q.Description)
		}
	}
	b.WriteString("\nChoose exactly one name from the list, or abstain.\n")
	return b.String()
}

const slotSystem = `You extract filter values from a question for a named database query.
Fill the free-text reason FIRST, then each value.
Copy values from the question. Never invent an id, a name, a date or a status the question does not contain.
If the question does not supply a value, set abstain to true rather than making one up.`

func slotPrompt(question string, q Query) string {
	var b strings.Builder
	b.WriteString("THE QUESTION:\n")
	b.WriteString(question)
	fmt.Fprintf(&b, "\n\nTHE QUERY: %s — %s\n\nVALUES NEEDED:\n", q.Name, q.Description)
	for _, sl := range q.Slots {
		fmt.Fprintf(&b, "  %s (type %s) — %s\n", sl.Name, sl.Type, sl.Desc)
	}
	return b.String()
}

// cardReason prefers the model's own stated reason, falling back to the
// structural one. A card that says why it appeared is the difference between a
// disambiguation and a shrug.
func cardReason(modelReason, fallback string) string {
	if r := strings.TrimSpace(modelReason); r != "" {
		return fallback + ": " + r
	}
	return fallback
}

// minContentToken is the shortest run of letters/digits treated as a content
// word. Structural, not a ⚙ (the §35/§38 precedent): its reason is that runs
// of three characters or fewer in English questions are overwhelmingly
// function words ("is", "the", "did", "was"), which match everything and
// therefore distinguish nothing.
const minContentToken = 4

// interrogativeStopwords are the question-shaped words long enough to pass
// minContentToken but carrying no subject matter. Structural, with the same
// standing as minContentToken: "what" appears in a great many questions and in
// a few catalog descriptions, so counting it as shared vocabulary would let the
// no-match floor pass a pick about something else entirely — which is the
// defect the floor exists to stop. The set is deliberately tiny; every member
// is a wh-word or an auxiliary, never a topic word.
var interrogativeStopwords = map[string]bool{
	"what": true, "when": true, "where": true, "which": true,
	"whose": true, "does": true, "did": true,
}

// contentTokens is the deterministic, model-free tokenizer both the shortlist
// and the no-match floor read a question through: lowercase, runs of [a-z0-9],
// keeping those at least minContentToken long. ONE definition, so the words the
// floor judges relatedness by are the same words the card's choices are found
// by — a card that said "no match" while offering entries matched on different
// words would be two different answers to one question.
func contentTokens(question string) map[string]bool {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(question), func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
	}) {
		if len(w) >= minContentToken {
			words[w] = true
		}
	}
	return words
}

// queryHay is the text a question's words are matched against: everything the
// catalog says about an entry in the reader's own language.
func queryHay(q Query) string {
	return strings.ToLower(q.Name + " " + string(q.Category) + " " + q.Description)
}

// sharesQuestionVocabulary reports whether the picked entry has anything to do
// with the words the asker used — at least one content token, interrogatives
// excluded, appearing in the entry's name, category or description.
//
// A question with no content tokens at all ("why?", "and now?") passes: there
// is nothing to judge relatedness by, and refusing every pick on a question the
// floor cannot read would turn a backstop into a gate.
func sharesQuestionVocabulary(question string, q Query) bool {
	hay := queryHay(q)
	judged := false
	for w := range contentTokens(question) {
		if interrogativeStopwords[w] {
			continue
		}
		judged = true
		if strings.Contains(hay, w) {
			return true
		}
	}
	return !judged
}

// topChoices picks the catalog entries to offer when nothing resolved. It is a
// deterministic, model-free shortlist: entries whose name, category or
// description shares a word with the question, then the first entries of each
// category so a card is never empty.
func topChoices(question string) []string {
	words := contentTokens(question)
	var hits []string
	seen := map[string]bool{}
	for _, q := range Catalog() {
		hay := queryHay(q)
		for w := range words {
			if strings.Contains(hay, w) {
				hits = append(hits, q.Name)
				seen[q.Name] = true
				break
			}
		}
		if len(hits) >= disambiguationChoices {
			break
		}
	}
	if len(hits) >= disambiguationChoices {
		return hits
	}
	// Pad with one entry per category, so every card offers a way forward.
	for _, cat := range Categories {
		for _, q := range Catalog() {
			if q.Category == cat && !seen[q.Name] {
				hits = append(hits, q.Name)
				seen[q.Name] = true
				break
			}
		}
		if len(hits) >= disambiguationChoices {
			break
		}
	}
	return hits
}

// nearby offers the chosen query plus its category siblings — the shape of a
// "which of these did you mean?" card when the model picked something but not
// confidently enough.
func nearby(name string) []string {
	q, ok := QueryByName(name)
	if !ok {
		return topChoices(name)
	}
	out := []string{q.Name}
	for _, c := range Catalog() {
		if c.Category == q.Category && c.Name != q.Name {
			out = append(out, c.Name)
		}
		if len(out) >= disambiguationChoices {
			break
		}
	}
	return out
}
