package local

import "fmt"

// alias.go — Spec S12.4 duty-alias resolution (brief R15). A duty alias is a
// named capability slot mapping to a swappable seat; workers, templates, and
// platform callers only ever address aliases (S12.4). The alias→seat map is
// settings-backed (⚙ local.alias, TypeMap), read by dotted key AT CALL TIME
// (CONVENTIONS §6 map-family discipline) so a swap gate change (S12.10, B4-7)
// is picked up without a rebuild. The map is READ-ONLY data to this packet —
// no swap machinery is built here (R15).

// ResolvedSeat is a duty alias resolved to its serving seat.
type ResolvedSeat struct {
	Alias string
	Seat  SeatRecord
}

// Registry resolves duty aliases over ⚙ local.alias + the committed manifest.
type Registry struct {
	settings Settings
}

// NewRegistry returns an alias registry reading ⚙ through settings.
func NewRegistry(settings Settings) *Registry { return &Registry{settings: settings} }

// Resolve maps a duty alias to its serving seat via ⚙ local.alias + the
// manifest (R15). Read at call time. The embedder alias refuses with the
// post-gate reason (G2 Def.8, §8 reading 9) — its row stays in the registry
// as S12.4 data but nothing is served. An unknown alias is a loud error; an
// alias whose ⚙ target has no manifest seat is a loud config error.
func (r *Registry) Resolve(alias string) (ResolvedSeat, error) {
	a := normalizeAlias(alias)
	if a == AliasEmbedder {
		return ResolvedSeat{}, ErrEmbedderPostGate
	}
	m, err := r.settings.StringMap(keyAlias)
	if err != nil {
		return ResolvedSeat{}, fmt.Errorf("local: read ⚙ %s: %w", keyAlias, err)
	}
	target, ok := m[a]
	if !ok {
		return ResolvedSeat{}, fmt.Errorf("%w: %q (not in ⚙ %s)", ErrUnknownAlias, alias, keyAlias)
	}
	seat, ok := seatByKey(target)
	if !ok {
		return ResolvedSeat{}, fmt.Errorf("%w: alias %q → %q", ErrNoSeat, alias, target)
	}
	return ResolvedSeat{Alias: a, Seat: seat}, nil
}

// SeatFor is Resolve returning just the seat (the common case).
func (r *Registry) SeatFor(alias string) (SeatRecord, error) {
	rs, err := r.Resolve(alias)
	if err != nil {
		return SeatRecord{}, err
	}
	return rs.Seat, nil
}
