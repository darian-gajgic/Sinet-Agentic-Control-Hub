package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Snapshot/restore primitives for the S13.10 durable-set pipeline. They live
// in internal/storage because the driver seam is here (Spec S01.3/CONVENTIONS
// §6: internal/storage is the modernc.org/sqlite driver's only importer). The
// backup package orchestrates the pipeline (VACUUM -> dump -> tar|zstd|age ->
// push) through these primitives and never touches the driver itself.

// The 11.3 exporter boundary, BY CONSTRUCTION (Spec S14.9 ¶3): "the snapshot
// exporter reads an allowlisted view containing only the keep-forever set; raw
// trace payloads are structurally unreachable from it."
//
// The dump therefore reads run_events THROUGH the run_events_export view
// (migration 0015), never from the table. The view keeps every ROW — event_seq
// must stay gap-free, an S02.9 invariant CheckRestored asserts — and replaces
// every non-keep-forever payload BODY with a marker, at ANY age. Keep-forever
// status is the whole test, which is also the only boundary a PER-USER horizon
// can express as one view.
//
// This supersedes the B4-3 placeholder, which blanked every payload at or below
// a scalar seq (including keep-forever ones) and was wired to 0, so nothing was
// stripped at all. S13.10 asks for exactly this ("raw trace payload bodies
// excluded; full traces stay local"); S02.9's horizon phrasing is satisfied a
// fortiori, since excluding every raw trace body necessarily excludes those
// past the horizon.
const (
	runEventsTable      = "run_events"
	runEventsExportView = "run_events_export"
)

// VacuumInto writes a transactionally-consistent copy of the live database to
// dest (Spec S02.9: the durable-set snapshot shape — `VACUUM INTO` a temp file,
// consistent against the live DB, never a file-copy of the live sidecars).
// dest must not already exist. VACUUM cannot run inside a transaction, so it
// runs on the pool outside WriteTx.
func (d *DB) VacuumInto(ctx context.Context, dest string) error {
	// dest is a platform-controlled temp path; single-quote-escape defensively
	// (VACUUM INTO takes a string literal, not a bound parameter).
	escaped := strings.ReplaceAll(dest, "'", "''")
	if _, err := d.sql.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return fmt.Errorf("storage: VACUUM INTO %q: %w", dest, err)
	}
	return nil
}

// DumpFrom serializes a snapshot database file (a VacuumInto copy) to a
// text-first `.dump` (Spec S13.10: diffable, deleted-content-purged). Statement
// order is tables -> data -> indexes/triggers/views (the standard .dump order,
// so a load never trips a not-yet-existing table). run_events data is read
// through the run_events_export view, so a raw trace payload body cannot reach
// the dump at any age (the 11.3 boundary above). Returns the dump SQL and the
// source user_version.
func DumpFrom(ctx context.Context, srcPath string) (string, int, error) {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(0)")
	db, err := sql.Open("sqlite", "file:"+srcPath+"?"+q.Encode())
	if err != nil {
		return "", 0, fmt.Errorf("storage: open snapshot source %q: %w", srcPath, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	var userVersion int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); err != nil {
		return "", 0, fmt.Errorf("storage: read user_version: %w", err)
	}

	type object struct{ typ, name, sql string }
	var all []object
	rows, err := db.QueryContext(ctx,
		`SELECT type, name, sql FROM sqlite_master
		 WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return "", 0, fmt.Errorf("storage: read schema: %w", err)
	}
	for rows.Next() {
		var o object
		if err := rows.Scan(&o.typ, &o.name, &o.sql); err != nil {
			rows.Close()
			return "", 0, err
		}
		all = append(all, o)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", 0, err
	}
	rows.Close()

	// Classify: regular tables (dump their data), FTS5 virtual tables (emit the
	// CREATE, which auto-builds the shadow tables, then rebuild the index from
	// their external content — never dump the shadow tables), and post objects
	// (indexes/triggers/views, emitted after data). Shadow tables (named
	// <virtual>_*) are skipped entirely — the virtual table owns them.
	virtualNames := map[string]bool{}
	for _, o := range all {
		if o.typ == "table" && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(o.sql)), "CREATE VIRTUAL TABLE") {
			virtualNames[o.name] = true
		}
	}
	var tables, virtuals, post []object
	for _, o := range all {
		shadow, unexpected := shadowClass(o.name, virtualNames)
		if unexpected {
			return "", 0, fmt.Errorf("storage: table %q matches an FTS virtual-table prefix but is not a recognized shadow table — refusing to silently drop it from the dump (S13.10)", o.name)
		}
		if shadow {
			continue
		}
		switch {
		case o.typ == "table" && virtualNames[o.name]:
			virtuals = append(virtuals, o)
		case o.typ == "table":
			tables = append(tables, o)
		default:
			post = append(post, o)
		}
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].name < tables[j].name })
	sort.Slice(virtuals, func(i, j int) bool { return virtuals[i].name < virtuals[j].name })

	var b strings.Builder
	for _, t := range tables {
		b.WriteString(t.sql)
		b.WriteString(";\n")
	}
	for _, v := range virtuals {
		b.WriteString(v.sql)
		b.WriteString(";\n")
	}
	exportViewPresent := false
	for _, o := range post {
		if o.typ == "view" && o.name == runEventsExportView {
			exportViewPresent = true
		}
	}
	for _, t := range tables {
		source := t.name
		if t.name == runEventsTable {
			if !exportViewPresent {
				// Fail LOUD. A dump that silently fell back to the raw table
				// would ship raw trace payload bodies off the host — the exact
				// thing S14.9 ¶3 makes structurally unreachable (the F9
				// never-a-silent-drop precedent, inverted: never a silent leak).
				return "", 0, fmt.Errorf("storage: %s is missing from the snapshot — refusing to dump run_events without the S14.9 11.3 export boundary", runEventsExportView)
			}
			source = runEventsExportView
		}
		// A VIEW has no rowid; run_events orders by its own event_seq, which is
		// the sole ordering authority anyway (S14.1).
		order := "rowid"
		if source != t.name {
			order = "event_seq"
		}
		if err := dumpTableData(ctx, db, t.name, source, order, &b); err != nil {
			return "", 0, err
		}
	}
	// Rebuild each external-content FTS index from its now-loaded content table
	// (the shadow tables were never dumped).
	for _, v := range virtuals {
		fmt.Fprintf(&b, `INSERT INTO "%s"("%s") VALUES('rebuild');`+"\n", v.name, v.name)
	}
	for _, o := range post {
		b.WriteString(o.sql)
		b.WriteString(";\n")
	}
	return b.String(), userVersion, nil
}

// fts5ShadowSuffixes are the exact shadow tables an FTS5 virtual table creates
// (external-content omits _content; the full set is enumerated so an
// unexpected <virtual>_* table is never silently dropped).
var fts5ShadowSuffixes = map[string]bool{
	"data": true, "idx": true, "docsize": true, "config": true, "content": true,
}

// shadowClass classifies a table name against the FTS5 virtual tables: a
// recognized shadow (skip — the virtual table's CREATE rebuilds it), an
// UNEXPECTED prefix match (fail loud — never a silent drop, F9), or neither.
func shadowClass(name string, virtualNames map[string]bool) (shadow, unexpected bool) {
	for vt := range virtualNames {
		if name == vt {
			return false, false // the virtual table itself
		}
		if strings.HasPrefix(name, vt+"_") {
			if fts5ShadowSuffixes[name[len(vt)+1:]] {
				return true, false
			}
			return false, true // matches an FTS prefix but is not a known shadow
		}
	}
	return false, false
}

// dumpTableData emits INSERTs INTO table, reading from source — the same name
// for every table but run_events, which is read through its export view (the
// 11.3 boundary). Reading through the view also keeps GENERATED columns out of
// the INSERT: migration 0015 added two VIRTUAL generated columns to run_events,
// which SELECT * would return and no INSERT may assign.
func dumpTableData(ctx context.Context, db *sql.DB, table, source, orderBy string, b *strings.Builder) error {
	rows, err := db.QueryContext(ctx, `SELECT * FROM "`+source+`" ORDER BY "`+orderBy+`"`)
	if err != nil {
		return fmt.Errorf("storage: read %s: %w", source, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	colList := make([]string, len(cols))
	for i, c := range cols {
		colList[i] = `"` + c + `"`
	}
	prefix := `INSERT INTO "` + table + `" (` + strings.Join(colList, ", ") + `) VALUES (`
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		b.WriteString(prefix)
		for i, v := range vals {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(sqlLiteral(v))
		}
		b.WriteString(");\n")
	}
	return rows.Err()
}

// sqlLiteral formats a scanned value as a SQL literal (portable across the
// dump round-trip; the platform-internal consistency posture, not cross-system
// canonicality).
func sqlLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case []byte:
		return "X'" + toHex(x) + "'"
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", x), "'", "''") + "'"
	}
}

func toHex(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}

// RebuildFromDump creates a fresh database at path from a text SQL dump and
// sets its user_version — the S13.10 restore-drill rebuild (Spec S13.10:
// "rebuild the DB from the dump"). It loads in WAL mode with foreign_keys OFF
// (the dump's statement order is not FK-topological, the standard .dump
// posture); the caller then re-opens via Open, which runs the S02.1
// integrity_check under WAL, and asserts the S02.9 invariants over the result.
// The LIVE database is never touched — the rebuild lands in a scratch location.
func RebuildFromDump(ctx context.Context, path, dumpSQL string, userVersion int) error {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(0)")
	q.Add("_pragma", "journal_mode(wal)")
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return fmt.Errorf("storage: open rebuild target %q: %w", path, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, dumpSQL); err != nil {
		return fmt.Errorf("storage: load dump: %w", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", userVersion)); err != nil {
		return fmt.Errorf("storage: set rebuilt user_version: %w", err)
	}
	return nil
}

// RestoreCheck is the S02.9 invariant result over a rebuilt database (the
// restore drill's assertions, Spec S13.10/S02.9): integrity, referential
// (durable-set) consistency, and a gap-free global event_seq.
type RestoreCheck struct {
	IntegrityOK     bool
	ForeignKeysOK   bool
	EventSeqGapFree bool
	RunEventCount   int64
	UserVersion     int
}

// AllOK reports whether every S02.9 invariant held.
func (c RestoreCheck) AllOK() bool {
	return c.IntegrityOK && c.ForeignKeysOK && c.EventSeqGapFree
}

// CheckRestored opens a rebuilt database at path and asserts the S02.9
// invariants (Spec S13.10 restore drill): PRAGMA integrity_check ok, PRAGMA
// foreign_key_check empty (durable-set referential consistency), and a gap-free
// global event_seq in run_events (count == max-min+1 when non-empty). It never
// mutates the DB and is independent of the live database.
func CheckRestored(ctx context.Context, path string) (RestoreCheck, error) {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return RestoreCheck{}, fmt.Errorf("storage: open restored %q: %w", path, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	var c RestoreCheck
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return RestoreCheck{}, fmt.Errorf("storage: restored integrity_check: %w", err)
	}
	c.IntegrityOK = integrity == "ok"

	fkRows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return RestoreCheck{}, fmt.Errorf("storage: restored foreign_key_check: %w", err)
	}
	c.ForeignKeysOK = !fkRows.Next()
	fkRows.Close()

	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&c.UserVersion); err != nil {
		return RestoreCheck{}, err
	}

	var count, span sql.NullInt64
	if err := db.QueryRowContext(ctx,
		"SELECT count(*), max(event_seq)-min(event_seq)+1 FROM run_events").Scan(&count, &span); err != nil {
		return RestoreCheck{}, fmt.Errorf("storage: restored event_seq scan: %w", err)
	}
	c.RunEventCount = count.Int64
	// Gap-free: an empty log is trivially gap-free; otherwise the count must
	// equal the span of event_seq.
	c.EventSeqGapFree = count.Int64 == 0 || (span.Valid && count.Int64 == span.Int64)
	return c, nil
}
