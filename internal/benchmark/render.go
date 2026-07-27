package benchmark

import "strings"

// render.go is BENCH-REG §3.2's ONE uniform presentation template: "both
// deliverables rendered through one uniform presentation template: formatting
// and scaffolding tells stripped; length never truncated (the length confound is
// reported alongside, not 'corrected' at this n)".
//
// Two things this template deliberately is NOT:
//
//   - It is not a de-identifier. Expert daily users detect platform style near
//     perfectly (P-T11-1), and presentation normalization is not established
//     shipped practice anywhere. §5 says so plainly: the template is the
//     MITIGATION, the per-vote guess is the MEASUREMENT. Nothing here is allowed
//     to imply the pair is blind because it was rendered.
//   - It is not a summarizer, truncator or rewriter. It removes scaffolding and
//     normalizes format; it never removes, shortens or reorders content. A
//     rendered arm keeps every content token of its input, in order — proven by
//     test, because "length never truncated" is frozen text.

// Render applies the uniform presentation template. It is a pure function: the
// same input always renders the same way, so neither arm can be advantaged by
// when or where it was rendered.
func Render(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = stripFrontMatter(s)

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blanks := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		line = normalizeBullet(line)
		if strings.TrimSpace(line) == "" {
			blanks++
			// A run of blank lines is scaffolding rhythm, not content: one
			// separator is kept so structure survives, the rest is dropped.
			if blanks > 1 || len(out) == 0 {
				continue
			}
			out = append(out, "")
			continue
		}
		blanks = 0
		out = append(out, line)
	}
	// Drop a trailing separator so the two arms end identically.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// stripFrontMatter removes a leading `---` delimited metadata block. Sinet's own
// artifacts carry front matter (the S06 intake artifact renderer emits it), so
// its presence or absence is a scaffolding tell rather than content — and the
// block is metadata about the deliverable, never the deliverable. A document
// whose opening `---` is never closed is left untouched: that is a horizontal
// rule, not front matter.
func stripFrontMatter(s string) string {
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return s
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return s
	}
	after := rest[end+len("\n---"):]
	if nl := strings.IndexByte(after, '\n'); nl >= 0 {
		return after[nl+1:]
	}
	return ""
}

// normalizeBullet renders every unordered-list marker as "-". Which glyph a
// producer favours is a house-style tell that says nothing about the work.
func normalizeBullet(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	for _, marker := range []string{"* ", "+ "} {
		if strings.HasPrefix(trimmed, marker) {
			return indent + "- " + trimmed[len(marker):]
		}
	}
	return line
}

// RenderedPair is both arms through the template, plus the §3.2 length
// confound. Position assignment lives on the pair row, not here: this type is
// arm-keyed and never leaves the package's own record path.
type RenderedPair struct {
	Platform string
	Direct   string
	// LengthPlatform / LengthDirect are the rendered character lengths. They
	// are REPORTED alongside the pair and never used to trim either arm — at
	// this n the length confound is stated, not corrected (§3.2).
	LengthPlatform int
	LengthDirect   int
}

// RenderPair renders both arms through the one template.
func RenderPair(platform, direct string) RenderedPair {
	p, d := Render(platform), Render(direct)
	return RenderedPair{Platform: p, Direct: d, LengthPlatform: len(p), LengthDirect: len(d)}
}

// ArtifactRef is the stable reference a §14 record carries for one rendered
// blind artifact. The bodies live in the pair row (they are small text artifacts
// OF the practice, inside the S02.9 durable set by construction); the EVENT
// carries only the ref, per the refs-not-blobs rule (P-T07-5).
func ArtifactRef(pairID string, side Side) string {
	return "benchmark://pair/" + pairID + "/" + strings.ToLower(string(side))
}
