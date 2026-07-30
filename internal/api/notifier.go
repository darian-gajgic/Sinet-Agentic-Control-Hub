package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/push"
)

// notifier.go is the S15.6 re-nag notifier: "delivery owned by the notifier",
// with the cadences from ⚙ and the decisions from the inbox derivation that
// already exists.
//
// WHY IT LIVES IN internal/api (B6-9 OQ3(a), ratified). "Which cards are
// pending for identity X" and "how many" are the per-scope projector
// derivations in this package, and a second SQL derivation of the same answer
// in another package is the twin-maintained-copy hazard (§40-C D3) — one that
// would show up as the notifier pushing about a card the inbox does not list,
// or staying silent about one it does. So the evaluation CONSUMES
// rankedApprovals, the same function `GET /api/approvals` serves, and
// internal/push holds only what this package must not: the crypto, the
// subscriptions, and the audit rows.
//
// NO TICKERS — AND THE RULE IS ABOUT DUENESS, NOT LOOPS (§32). The shell drives
// this on an interval, exactly as it drives the watchdog sweep and the dead-man
// probe. What must never be a wall-clock countdown is WHETHER A CARD IS DUE,
// and it is not: dueness derives from the card's own ObservedTS, the ⚙ cadences
// read live at evaluation, and the last push for that card READ FROM THE EVENT
// LOG. There is no side table and nothing this process remembers, so a fresh
// process over the same database answers identically and a card born while the
// host was asleep is picked up on the next pass — which is exactly what the
// dead-man loop's own comment warns a ticker cannot do.

// PushSender is the outbound seam: one message, one subscription, one audited
// attempt. It exists so the whole notifier can be driven against a fake push
// service — the production implementation is *push.Sender, and nothing in this
// package composes an HTTP request to anywhere.
type PushSender interface {
	Send(ctx context.Context, sub push.Subscription, msg push.Message) push.Result
}

// The three G1 Def.3 cadence keys, by dotted key and never as constants. They
// are read LIVE at evaluation (the §32 setting: pattern), so a cadence the
// operator changes takes effect across the whole inbox — which is what the
// registry's own help text means by scoping them "inbox-wide". The SLA block
// stamped on a verify card at birth stays DISPLAY data.
const (
	keyCardPushHours     = "verification.card_push_hours"
	keySafetyRepingHours = "verification.safety_reping_hours"
	// keyCardRemindHours is READ so a composition defect is loud, and its
	// meaning at v0 is the inbox's OWN staleness marking (B6-9 OQ4(ii)): a card
	// past the reminder threshold is already flagged and re-ranked in the queue,
	// and no separate reminder CHANNEL is built. The notifier does not push at
	// 4 h; it pushes at card_push_hours.
	keyCardRemindHours = "verification.card_remind_hours"
)

// pushLookbackCadences bounds the push-history scan, and it is a bound that
// cannot change an answer: a card whose newest push is more than this many
// cadences old is due by arithmetic whether the row is read or not. Structural,
// with that as its reason.
const pushLookbackCadences = 4

// notifyIconPath is the self-hosted notification icon. It is a path on the
// platform's own origin — the composer prefixes the subscription's own recorded
// origin — so nothing about a notification reaches a third-party asset host
// (S01.8).
const notifyIconPath = "/icon-192.png"

// The two content-light notification titles. They are platform vocabulary and
// carry nothing from the card: what the decision IS lives behind the deep link,
// which is the S3.7 loop this channel exists to close (notified → glance →
// decide), and a title that summarised the card would put its content on a
// vendor relay's screen-lock preview.
const (
	titleApproval = "Sinet — a decision is waiting"
	titleSafety   = "Sinet — safety escalation waiting"
)

// EvaluatePush is one notifier pass: for every identity with an enrolled
// device, derive what is pending, decide what is DUE, and send exactly that.
//
// It is IDEMPOTENT under repetition. Two back-to-back passes send once, because
// the first pass's audit row is what the second one reads as "last pushed" and
// no cadence has elapsed between them. Nothing here is scheduled and nothing is
// remembered between calls.
func (s *Server) EvaluatePush(ctx context.Context) error {
	if s.push == nil || s.pushSender == nil || s.proj == nil {
		return nil // the channel is not wired in this process
	}
	pushAfter, reping, err := s.pushCadences()
	if err != nil {
		return err
	}
	owners, err := s.push.Owners(ctx)
	if err != nil {
		return err
	}
	now := s.clock()
	lookback := pushLookbackCadences * max(pushAfter, reping)

	var firstErr error
	for _, owner := range owners {
		if err := s.notifyOwner(ctx, owner, now, lookback, pushAfter, reping); err != nil {
			// One person's evaluation failing must not silence the household.
			s.logger.Error("push: evaluate", "owner", owner, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Server) notifyOwner(ctx context.Context, owner string, now time.Time, lookback, pushAfter, reping time.Duration) error {
	subs, err := s.push.List(ctx, owner)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	scope := ownerScope{UserID: owner, Operator: s.isOperatorID(ctx, owner)}
	items, _, err := s.rankedApprovals(ctx, scope)
	if err != nil {
		return err
	}
	// THE BADGE IS THE INBOX'S OWN LIST (B6-9 OQ5): the count of the ranked
	// derivation this identity's `/inbox` renders, so a glance at the badge and
	// a glance at the queue agree. It is taken BEFORE the read surface's page
	// bound, because a badge is a count of what is waiting and not a count of
	// what fits on a page — a capped badge would be a silent cap on a number.
	badge := len(items)

	last, err := s.push.LastPushed(ctx, owner, now.Add(-lookback))
	if err != nil {
		return err
	}
	for _, item := range items {
		msg, ok := s.dueMessage(item, badge, now, last[item.ID], pushAfter, reping)
		if !ok {
			continue
		}
		for _, sub := range subs {
			s.pushSender.Send(ctx, sub, msg)
		}
	}
	return nil
}

// dueMessage decides whether one card is due for this identity right now, and
// composes what would go out.
//
// A CARD IS PUSHED ONLY TO SOMEBODY WHO CAN ANSWER IT, and that is a reading
// recorded rather than assumed. S15.12 routes approvals "to the decision owner
// (D10)" and S15.11's whole subject is "decisions reach the RIGHT PERSON": the
// operator's ranked list legitimately carries every household member's card, so
// pushing all of them would wake the operator every 24 h, forever, for
// decisions the platform itself refuses to let them answer. The BADGE still
// counts the whole list (OQ5), because that is the number the inbox shows.
func (s *Server) dueMessage(item ApprovalItem, badge int, now, last time.Time, pushAfter, reping time.Duration) (push.Message, bool) {
	if !item.Answerable {
		return push.Message{}, false
	}
	// An EXPIRED card is never pushed again: the countdown it displayed has run
	// out, so a notification would send somebody to a decision they can no
	// longer make. An answered card is not here at all — the derivation reads
	// only what is still open.
	if item.ExpiryAt != nil && !now.Before(*item.ExpiryAt) {
		return push.Message{}, false
	}
	class := cardSLAClass(item.Card)
	cadence := pushAfter
	title := titleApproval
	if class == push.ClassSafety {
		cadence, title = reping, titleSafety
	}
	if !pushDue(class, item.ObservedTS, last, now, pushAfter, reping) {
		return push.Message{}, false
	}
	return push.Message{
		CardID: item.ID, Class: class, Title: title,
		Path:  inboxItemPath(item.ID),
		Badge: badge, IconPath: notifyIconPath,
		// The TTL is the class's own cadence: a message the push service could
		// not deliver before the next one is due has been superseded, and
		// delivering a stale nag after a fresh one is worse than dropping it.
		// It needs no ⚙ of its own because it is not an independent number.
		TTL: cadence,
	}, true
}

// pushDue is the whole re-nag SLA, as a pure function of stored state (G1 Def.3
// as superseded at close; S15.6). It is pure so it can be driven exhaustively
// without a database, and because dueness that reads a clock it also owns is
// the countdown §32 forbids.
//
//   - SAFETY pushes IMMEDIATELY on birth and re-pings every ⚙
//     verification.safety_reping_hours while the card is still open.
//   - APPROVAL pushes once the card has been pending ⚙
//     verification.card_push_hours, and every further card_push_hours while it
//     stays unanswered (B6-9 OQ4(i): the registry help reads as a first-push
//     threshold and S15.6's own word is "re-push", and a single never-repeated
//     push under a heading that says "re-nag" would under-deliver the letter).
//
// `last` zero means no push for this card has ever gone out.
func pushDue(class string, born, last, now time.Time, pushAfter, reping time.Duration) bool {
	if class == push.ClassSafety {
		if last.IsZero() {
			return true
		}
		return now.Sub(last) >= reping
	}
	if last.IsZero() {
		return now.Sub(born) >= pushAfter
	}
	return now.Sub(last) >= pushAfter
}

// cardSLAClass reads the card's OWN stamped SLA block (B6-9 OQ4): a card is
// safety-class iff its snapshot says so.
//
// Today that is exactly the S07.7 CatSafety route through internal/verify,
// which stamps {class, push_immediately, reping_every_hours} into the ask
// snapshot at card birth. Everything else — plain asks, effect approvals, the
// oversight kinds — carries no stamp and is approval-class. THE CLIENT INVENTS
// NO CLASS: a watchdog flag-now card is urgent in the watchdog's own alert
// vocabulary and carries no SLA stamp, so it stays approval-class at v0 and the
// producer-side stamp is the operator's to direct.
func cardSLAClass(card json.RawMessage) string {
	if len(card) == 0 {
		return push.ClassApproval
	}
	var snap struct {
		SLA struct {
			Class string `json:"class"`
		} `json:"sla"`
	}
	if err := json.Unmarshal(card, &snap); err != nil {
		return push.ClassApproval
	}
	if snap.SLA.Class == push.ClassSafety {
		return push.ClassSafety
	}
	return push.ClassApproval
}

// pushCadences reads the two live cadences. Both keys are declared (S18), so a
// read failure is a composition defect rather than a runtime condition: the
// pass fails loudly instead of notifying on an invented schedule.
func (s *Server) pushCadences() (pushAfter, reping time.Duration, err error) {
	if s.settings == nil {
		return 0, 0, fmt.Errorf("api: no settings registry wired: the notifier reads its cadences from ⚙ and will not invent them")
	}
	hours, err := s.settings.Int(keyCardPushHours)
	if err != nil {
		return 0, 0, fmt.Errorf("read ⚙ %s: %w", keyCardPushHours, err)
	}
	pushAfter = time.Duration(hours) * time.Hour
	if hours, err = s.settings.Int(keySafetyRepingHours); err != nil {
		return 0, 0, fmt.Errorf("read ⚙ %s: %w", keySafetyRepingHours, err)
	}
	reping = time.Duration(hours) * time.Hour
	// Read so a broken registry is loud here rather than at the inbox. Its v0
	// meaning is the queue's own staleness marking, not a push (OQ4(ii)).
	if _, err = s.settings.Int(keyCardRemindHours); err != nil {
		return 0, 0, fmt.Errorf("read ⚙ %s: %w", keyCardRemindHours, err)
	}
	return pushAfter, reping, nil
}

// isOperatorID answers the role bit for an identity the notifier is evaluating
// FOR, rather than for a request identity. The dev-posture fallback of
// isOperatorRead has no meaning here — a notifier pass has no request — so this
// reads the users row and nothing else.
func (s *Server) isOperatorID(ctx context.Context, userID string) bool {
	if s.sessions == nil {
		return false
	}
	u, err := s.sessions.User(ctx, userID)
	if err != nil {
		return false
	}
	return u.Role == auth.RoleOperator
}

// inboxItemPath composes the deep link for one card.
//
// IT MUST BYTE-MATCH WHAT THE SPA'S OWN hrefFor PRODUCES, because that is what
// makes a tapped notification land on the card it names. Card ids are
// `<kind>:<native id>` composites and native ids carry ':', '#' and a unit
// separator, so the encoding is the whole agreement — and Go's url.PathEscape
// is NOT that encoding (it leaves ':' and '@' alone and escapes a different
// set). encodeURIComponentJS below is the JavaScript rule, and a golden fixture
// ties the two languages together rather than leaving them to agree by luck.
func inboxItemPath(cardID string) string {
	return "/inbox/" + encodeURIComponentJS(cardID)
}

// encodeURIComponentJS implements JavaScript's encodeURIComponent exactly: every
// byte of the UTF-8 encoding is percent-escaped except the unreserved set
// A-Z a-z 0-9 and the seven marks - _ . ! ~ * ' ( ).
func encodeURIComponentJS(s string) string {
	const unreservedMarks = "-_.!~*'()"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			strings.IndexByte(unreservedMarks, c) >= 0:
			b.WriteByte(c)
		default:
			const hex = "0123456789ABCDEF"
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return b.String()
}

// rankedApprovals is the ONE per-identity inbox derivation, shared by the read
// surface and the notifier (OQ3's whole point). It returns the ranked list
// UNBOUNDED; the read surface applies its own page cap afterwards and says so
// with `truncated`, and the notifier counts the full list for the badge.
func (s *Server) rankedApprovals(ctx context.Context, scope ownerScope) ([]ApprovalItem, time.Duration, error) {
	expiry, stale, err := s.approvalHorizons()
	if err != nil {
		return nil, 0, err
	}
	snap, err := s.proj.inbox(ctx, scope)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.proj.approvalItems(ctx, scope, snap, expiry, stale)
	if err != nil {
		return nil, 0, err
	}
	conflicts, err := s.memoryConflictCards(ctx, scope)
	if err != nil {
		return nil, 0, err
	}
	items = append(items, conflicts...)
	// Risk-ranked (S15.6 "cards arrive ranked by risk"), then oldest-first
	// inside a tier so the queue drains in the order it filled.
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := tierRank[items[i].Tier], tierRank[items[j].Tier]
		if ri != rj {
			return ri < rj
		}
		if !items[i].ObservedTS.Equal(items[j].ObservedTS) {
			return items[i].ObservedTS.Before(items[j].ObservedTS)
		}
		return items[i].ID < items[j].ID
	})
	return items, expiry, nil
}
