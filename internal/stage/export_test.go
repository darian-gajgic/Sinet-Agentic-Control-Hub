package stage

// export_test.go — test-only surface. The per-duty length caps stay unexported
// structural constants (S18 ratifies no ⚙ key); the live tripwire needs the
// phrase cap to assert the real call lands UNDER it with headroom, and reading
// the number from the same constant the caller sends is what makes that
// assertion honest (PH-1 F4).

// PhraseMaxTokens is the phrase duty's length cap, exposed to the live leg.
const PhraseMaxTokens = phraseMaxTokens

// HelpMaxTokens is the 13.5 help drafting cap, exposed to the live leg.
const HelpMaxTokens = helpMaxTokens
