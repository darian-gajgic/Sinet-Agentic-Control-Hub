package api

// export_test.go exposes package internals to the external api_test package for
// assertions that must be made against the REAL value rather than a copy.

// TerminalRunStatesForTest exposes the S02.3 terminal-state mirror the
// failed-pair derivation filters on, so a test can cross-check it against
// run.IsTerminal — the one definition of record — without this package importing
// internal/run for a predicate it only needs inside SQL-shaped code.
func TerminalRunStatesForTest() []string { return terminalRunStates }
