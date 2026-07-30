package review

import (
	"strconv"
	"strings"
)

// The S13.3 anchor model. Anchors are re-validated SERVER-SIDE on every
// render and every port — client- or agent-supplied positions are never
// trusted as placement authority (FC-v1 §2). Validation and porting are
// pure functions of immutable revision contents, so recorded placements
// are stable and re-validation must agree.

// validateBirth validates an as-claimed anchor against the revision it was
// made on (the port-free half of the ladder): line_text at the claimed
// position → exact; line_text within ±drift of the claimed position →
// drifted (the validated position returned); otherwise → file (the quote
// stays visible on the comment row; render location = file strip or the
// synthetic strip). A claimed file absent from the revision degrades to
// file too — born-orphan does not exist, the original-revision link is the
// row itself.
func validateBirth(files map[string]string, a AnchorRecord, drift int) (AnchorStatus, AnchorRecord) {
	content, ok := files[a.FilePath]
	if !ok {
		return AnchorFile, AnchorRecord{}
	}
	lines := splitLines(content)
	if at(lines, a.LineNo) == a.LineText {
		return AnchorExact, a
	}
	if n, ok := searchNear(lines, a.LineNo, a.LineText, drift); ok {
		found := a
		found.LineNo = n
		return AnchorDrifted, found
	}
	return AnchorFile, AnchorRecord{}
}

// portAnchor ports a comment's anchor onto a target revision — the 5-step
// re-anchoring ladder (Spec S13.3; FC-v1 §2):
//
//  1. map (side, line_no) through the known source→target diff,
//  2. exact line_text match at the mapped position,
//  3. line_text search within ⚙ review.anchor_drift_lines of the mapped
//     position,
//  4. degrade to a file-level placement (original quote stays visible),
//  5. ORPHAN — explicit, quote and original-revision link kept, still
//     renders (synthetic strip) and still drains as file-level (P-T12-2).
//
// srcFiles is the content the anchor's line lives in (the anchor-side
// content of the view the comment was made on); dstFiles the target
// revision. A nil srcFiles (source content unavailable — e.g. an old-side
// anchor of revision 1, whose old side is the pre-task base that only the
// S13.5 git topology can materialize) skips the map and degrades honestly
// from step 3's search-at-claimed-position.
func portAnchor(srcFiles, dstFiles map[string]string, a AnchorRecord, drift int) (AnchorStatus, AnchorRecord) {
	if a.LineNo <= 0 || a.FilePath == "" {
		// A born file-level comment ports as file-level while its file (or
		// the deliverable) exists; there is no position to track.
		if a.FilePath == "" {
			return AnchorFile, AnchorRecord{}
		}
		if _, ok := dstFiles[a.FilePath]; ok {
			return AnchorFile, AnchorRecord{FilePath: a.FilePath}
		}
		return AnchorOrphan, AnchorRecord{}
	}
	dstContent, ok := dstFiles[a.FilePath]
	if !ok {
		// The anchored file is gone from the target revision: no live
		// location anywhere — the explicit ORPHAN state (step 5).
		return AnchorOrphan, AnchorRecord{}
	}
	dstLines := splitLines(dstContent)

	mapped := a.LineNo
	if srcFiles != nil {
		if srcContent, ok := srcFiles[a.FilePath]; ok {
			unified, err := gitDiff(a.FilePath, srcContent, dstContent)
			if err == nil {
				if hs, herr := parseHunks(unified); herr == nil {
					mapped, _ = lineMap(hs, a.LineNo)
				}
			}
			// A diff failure falls through with mapped = the claimed
			// position: the text steps below still decide, never a silent
			// drop.
		}
	}

	ported := a
	ported.Side = SideNew // a ported placement points into the target revision
	if at(dstLines, mapped) == a.LineText {
		ported.LineNo = mapped
		if mapped == a.LineNo {
			// The quote confirms at the anchored line number itself.
			return AnchorExact, ported
		}
		return AnchorMapped, ported
	}
	if n, ok := searchNear(dstLines, mapped, a.LineText, drift); ok {
		ported.LineNo = n
		return AnchorDrifted, ported
	}
	// Step 4: the file still exists — file-level, quote kept visible.
	return AnchorFile, AnchorRecord{FilePath: a.FilePath}
}

// at returns the 1-based line or "" out of range.
func at(lines []string, n int) string {
	if n < 1 || n > len(lines) {
		return ""
	}
	return lines[n-1]
}

// searchNear looks for an exact line_text match within ±drift lines of
// pos, preferring the closest hit (ties break toward the earlier line —
// deterministic). Empty quotes never match: a blank line is no identity.
func searchNear(lines []string, pos int, text string, drift int) (int, bool) {
	if strings.TrimSpace(text) == "" {
		return 0, false
	}
	for d := 0; d <= drift; d++ {
		if at(lines, pos-d) == text {
			return pos - d, true
		}
		if d > 0 && at(lines, pos+d) == text {
			return pos + d, true
		}
	}
	return 0, false
}

// ParseFindingAnchor interprets a verification finding's opaque anchor
// string (Spec S07.5 emits "file:line / section / claim id" shapes; the
// anchor mechanics are S13's). A "path:NNN" tail that names a file of the
// revision parses to a positional anchor on the new side (the quote
// selector is filled from the revision content, making the stored record
// self-verifying); anything else records as a file-level comment with the
// original string kept verbatim in origin_anchor — findings are never
// refused for anchor shape (P-T12-2).
func ParseFindingAnchor(files map[string]string, raw string) (AnchorRecord, bool) {
	i := strings.LastIndexByte(raw, ':')
	if i <= 0 || i == len(raw)-1 {
		return AnchorRecord{}, false
	}
	path, numPart := raw[:i], raw[i+1:]
	n, err := strconv.Atoi(numPart)
	if err != nil || n < 1 {
		return AnchorRecord{}, false
	}
	content, ok := files[path]
	if !ok {
		return AnchorRecord{}, false
	}
	line := at(splitLines(content), n)
	if line == "" {
		return AnchorRecord{}, false
	}
	return AnchorRecord{FilePath: path, Side: SideNew, LineNo: n, LineText: line}, true
}
