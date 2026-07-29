package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
)

// title.go — S15.7's "first-message auto-titling runs as a local-tier duty".
//
// THE SEAT IS `utility`, AND IT IS AN EXISTING ROW (OQ2(a)). S12.4's utility
// entry covers "summaries … drafting", its outputs are "human-gated downstream"
// and "this seat never decides" — which is exactly a session label: a person
// reads it, and renames it if they disagree (RenameSession). NO NEW ALIAS IS
// INVENTED: ⚙ local.alias is registry data with ten declared rows, and adding
// an eleventh would be an S00.9 amendment plus an S18 sweep, not an executor's
// act. The alias resolves through the landed registry AT CALL TIME, so an
// operator retarget is honored without this code knowing (S12.4 R15).
//
// FREE-TEXT DRAFTING, NO MARGIN GATE. Classification:false — no schema, no
// logprobs, no forced-label abstain, and no calibrated margin check, because
// `utility` is honestly UNCALIBRATED by design (S12.5) and a gate over an
// uncalibrated seat would be a number pretending to mean something. What the
// model returns is a suggestion; the human override outranks it permanently.
//
// DEGRADATION IS TOTAL AND MECHANICAL. An absent stack (ErrStackAbsent), a
// stopped-admissions switch, a non-servable seat, a refusal, a blank answer —
// every one of them lands the same deterministic fallback: a truncation of the
// person's own first message. That is a MECHANICAL derivation of something the
// person actually wrote, never a fabricated model output, and it is why a
// session is always honestly named even with no local tier at all. NOTHING HERE
// CAN FAIL A TURN: the caller ignores the outcome, and a session nobody named
// is a cosmetic absence while a turn that will not start is a broken surface.

// titleMaxTokens caps the drafting call. A title is one short line; a per-duty
// length cap is the caller's structural constant (S18 ratifies no key —
// local.DutyRequest.MaxTokens is set by every duty site the same way).
const titleMaxTokens = 48

// titleSystem/titleUser shape the drafting call. The message text is the
// UNTRUSTED region and it is framed as data to be summarized, never as
// instructions — the standing "model output is untrusted input" invariant
// applied at the input end (§29 reading; S12.4).
const titleSystem = "You label conversations for a personal dashboard. Reply with one short title of at most eight words. No quotes, no punctuation at the end, no preamble."

// titleFromFirstMessage names a session from the message that opened its FIRST
// turn. It is called after BeginTurn commits and its result is deliberately
// discarded by the caller.
func (s *Store) titleFromFirstMessage(ctx context.Context, owner, sessionID, text string) {
	sess, err := s.Session(ctx, owner, sessionID)
	if err != nil {
		return
	}
	// Only the first turn titles, and a session that already carries a title —
	// from the duty, from the mechanical fallback, or from a human — is never
	// re-titled. The human case is the one that matters: a rename the assistant
	// could undo is not an override.
	if sess.TitleSource != TitleUnset {
		return
	}
	var turns int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM chat_turns WHERE session_id = ? AND user_id = ?`,
		sessionID, owner).Scan(&turns); err != nil || turns != 1 {
		return
	}

	title, source := mechanicalTitle(text), TitleMechanical
	if drafted, ok := s.draftTitle(ctx, text); ok {
		title, source = drafted, TitleDuty
	}
	s.applyTitle(ctx, owner, sessionID, title, source)
}

// draftTitle runs the one duty call. It returns ok=false on every degradation,
// and the caller falls back — there is no path here that invents a title.
func (s *Store) draftTitle(ctx context.Context, text string) (string, bool) {
	if s.duty == nil || s.advisory == nil {
		return "", false
	}
	// Every local call is metered on a consuming run (S12.1 R18), and naming a
	// session has no run of its own, so it rides a short-lived platform advisory
	// run — the history-intent / contradiction-screen / narrator precedent.
	runID, settle := s.advisory(ctx, "chat-title")
	defer settle()
	if runID == "" {
		return "", false
	}
	res, err := s.duty.Call(ctx, runID, local.DutyRequest{
		Alias:     local.AliasUtility,
		System:    titleSystem,
		User:      "Conversation opener:\n\n" + truncate(text, 600),
		MaxTokens: titleMaxTokens,
		// Free-text drafting: no schema, no logprobs, no abstain belt.
		Classification: false,
	})
	if err != nil {
		return "", false
	}
	return cleanTitle(res.Content)
}

// applyTitle writes the label. The WHERE clause still requires title_source to
// be unset, so a human rename that committed while the duty was thinking wins
// the race — the check above is the fast path, this is the guarantee.
func (s *Store) applyTitle(ctx context.Context, owner, sessionID, title, source string) {
	if title == "" {
		return
	}
	payload, err := json.Marshal(struct {
		Session string `json:"session"`
		Source  string `json:"source"`
	}{sessionID, source})
	if err != nil {
		return
	}
	_ = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE chat_sessions SET title = ?, title_source = ?, updated_ts = ?
			  WHERE session_id = ? AND user_id = ? AND title_source = ''`,
			title, source, s.stamp(), sessionID, owner)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			// Somebody named it first. Nothing changed, so nothing is recorded.
			return nil
		}
		_, err = s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: owner, Type: EventSessionRenamed,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
}

// mechanicalTitle is the deterministic fallback: the first line of what the
// person wrote, collapsed and truncated on a word boundary. It derives from
// their own words and reproduces exactly, which is what makes it honest to show
// without saying a model wrote it.
func mechanicalTitle(text string) string {
	line := strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if line == "" {
		return ""
	}
	if utf8.RuneCountInString(line) <= MaxTitleRunes {
		return line
	}
	// Cut on a rune boundary, then back off to the last space so the label ends
	// at a word rather than mid-syllable.
	cut := string([]rune(line)[:MaxTitleRunes])
	if i := strings.LastIndex(cut, " "); i > MaxTitleRunes/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

// cleanTitle normalizes a drafted title and rejects an unusable one. Model
// output is UNTRUSTED INPUT (§29 reading): it is stripped of the wrapping a
// model habitually adds, collapsed to one line, and bounded — and if what
// survives is empty, the answer is "no title", not a best effort.
func cleanTitle(raw string) (string, bool) {
	t := strings.TrimSpace(raw)
	// A reasoning seat may emit a <think> block ahead of the answer; the duty
	// layer disables that phase only for classification calls, so take the tail.
	if i := strings.LastIndex(t, "</think>"); i >= 0 {
		t = strings.TrimSpace(t[i+len("</think>"):])
	}
	if i := strings.IndexAny(t, "\r\n"); i >= 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(strings.Join(strings.Fields(t), " "))
	t = strings.Trim(t, `"'`+"`")
	t = strings.TrimRight(t, " .;:,")
	t = strings.TrimSpace(t)
	if t == "" {
		return "", false
	}
	if utf8.RuneCountInString(t) > MaxTitleRunes {
		t = strings.TrimSpace(string([]rune(t)[:MaxTitleRunes]))
	}
	for _, r := range t {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	return t, true
}

// truncate bounds the prompt's untrusted region on a rune boundary.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// TitleSeatWired reports whether the titling duty has both a caller and a
// metering run. It exists so a surface can state the honest reason a session is
// mechanically named rather than leaving a person to wonder.
func (s *Store) TitleSeatWired() bool { return s != nil && s.duty != nil && s.advisory != nil }
