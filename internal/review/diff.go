package review

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// The unified line diff is computed HOST-SIDE with `git diff` between
// revision pins (Spec S13.2) — one diff authority for both the review
// surface and the anchor diff-map, so the map can never disagree with the
// rendered diff. git is an OS-mechanism adoption (components.lock entry;
// the S13.5 git topology is its heavier consumer at B4-2); its absence is
// a loud error, never a silent fallback.

// gitDiff runs `git diff --no-index` over two contents and returns the
// unified diff body ("" when identical). Exit status 1 means "differs" and
// is not an error.
//
// `name` is the LOGICAL path the two contents belong to, and it is what the
// returned diff's file headers name. Without it the headers named the
// throwaway temp files git was handed — a random directory per call — which
// made the served text carry a host path AND differ on every read of the
// same immutable revision pair. A unified diff's headers are the file
// identity every consumer reads (the S15.8 widget parses them, and the
// anchor model keys on file paths), so naming the temp file was naming the
// wrong thing. An empty name leaves the output as git wrote it.
func gitDiff(name, oldContent, newContent string) (string, error) {
	dir, err := os.MkdirTemp("", "sinet-review-diff-*")
	if err != nil {
		return "", fmt.Errorf("review: diff tmp: %w", err)
	}
	defer os.RemoveAll(dir)
	oldPath := filepath.Join(dir, "old")
	newPath := filepath.Join(dir, "new")
	if err := os.WriteFile(oldPath, []byte(oldContent), 0o600); err != nil {
		return "", fmt.Errorf("review: diff tmp write: %w", err)
	}
	if err := os.WriteFile(newPath, []byte(newContent), 0o600); err != nil {
		return "", fmt.Errorf("review: diff tmp write: %w", err)
	}
	// --no-color/--unified pinned so parsing never depends on host config;
	// -c core.quotepath=false keeps non-ASCII paths literal.
	cmd := exec.Command("git", "-c", "core.quotepath=false", "diff",
		"--no-index", "--no-color", "--unified=3", "--", oldPath, newPath)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", fmt.Errorf("review: git diff: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		// Exit 1 = the files differ: the expected outcome, not an error.
	}
	return relabel(out.String(), oldPath, newPath, name), nil
}

// relabel rewrites the temp paths in a unified diff's file headers to the
// logical path the contents came from. Only the paths git was handed are
// replaced — by exact string, not by pattern — so no diff BODY line can be
// touched: a temp path cannot appear in content that was written to that temp
// path moments earlier.
func relabel(unified, oldPath, newPath, name string) string {
	if unified == "" || name == "" {
		return unified
	}
	// git prints the paths under the a/ b/ prefixes with the leading separator
	// stripped; the bare absolute forms are replaced too, because which one a
	// git version emits for --no-index outside a repository is not something to
	// depend on.
	rel := func(p string) string { return strings.TrimPrefix(filepath.ToSlash(p), "/") }
	return strings.NewReplacer(
		"a/"+rel(oldPath), "a/"+name,
		"b/"+rel(newPath), "b/"+name,
		filepath.ToSlash(oldPath), name,
		filepath.ToSlash(newPath), name,
	).Replace(unified)
}

// hunk is one parsed @@ block.
type hunk struct {
	oldStart, oldLines int
	newStart, newLines int
}

// parseHunks extracts the hunk headers from a unified diff body.
func parseHunks(unified string) ([]hunk, error) {
	var hs []hunk
	for _, line := range strings.Split(unified, "\n") {
		if !strings.HasPrefix(line, "@@ ") {
			continue
		}
		// @@ -oldStart[,oldLines] +newStart[,newLines] @@ ...
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("review: malformed hunk header %q", line)
		}
		o, err := parseRange(strings.TrimPrefix(fields[1], "-"))
		if err != nil {
			return nil, fmt.Errorf("review: hunk header %q: %w", line, err)
		}
		n, err := parseRange(strings.TrimPrefix(fields[2], "+"))
		if err != nil {
			return nil, fmt.Errorf("review: hunk header %q: %w", line, err)
		}
		hs = append(hs, hunk{oldStart: o[0], oldLines: o[1], newStart: n[0], newLines: n[1]})
	}
	return hs, nil
}

func parseRange(s string) ([2]int, error) {
	start, count := s, "1"
	if i := strings.IndexByte(s, ','); i >= 0 {
		start, count = s[:i], s[i+1:]
	}
	a, err := strconv.Atoi(start)
	if err != nil {
		return [2]int{}, err
	}
	b, err := strconv.Atoi(count)
	if err != nil {
		return [2]int{}, err
	}
	return [2]int{a, b}, nil
}

// lineMap maps an old-side line number to its new-side position through
// the parsed hunks (the S13.3 ladder step 1: Sinet's closed world always
// has the diff). Return values: (newLine, true) when the line is outside
// every hunk (unchanged — a direct correspondence), or (bestGuess, false)
// when the line falls INSIDE a hunk (modified/deleted region — the mapped
// position is the hunk's new-side start plus the offset into the hunk,
// clamped; text confirmation decides from there).
func lineMap(hs []hunk, oldLine int) (int, bool) {
	delta := 0
	for _, h := range hs {
		oldEnd := h.oldStart + h.oldLines // first line past the hunk (old side)
		if oldLine < h.oldStart {
			break
		}
		if oldLine < oldEnd || (h.oldLines == 0 && oldLine == h.oldStart) {
			// Inside a changed region: best-guess position keeps the
			// offset into the hunk, clamped to the hunk's new side.
			off := oldLine - h.oldStart
			guess := h.newStart + off
			if h.newLines > 0 {
				max := h.newStart + h.newLines - 1
				if guess > max {
					guess = max
				}
				if guess < h.newStart {
					guess = h.newStart
				}
			} else {
				guess = h.newStart
			}
			if guess < 1 {
				guess = 1
			}
			return guess, false
		}
		delta += h.newLines - h.oldLines
	}
	mapped := oldLine + delta
	if mapped < 1 {
		mapped = 1
	}
	return mapped, true
}

// splitLines splits content into lines without a trailing phantom line.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
