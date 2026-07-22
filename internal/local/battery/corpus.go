package battery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// corpus.go — the ~250K-token Sinet-domain corpus for the KL quant check (brief
// R8), assembled DETERMINISTICALLY from the repo's OWN text (Spec/Research/Docs
// — platform-owned, no licensing issue). Assembled-by-construction rather than
// committed as a 1 MB blob: the builder walks a fixed set of roots in sorted
// order and concatenates, so the corpus is reproducible + versioned with the
// harness (the measurement records the sha256 + token estimate). Token estimate
// = bytes/4 (the §17 structural estimate).

// Corpus is an assembled KL-check corpus.
type Corpus struct {
	Text         string   `json:"-"`
	ApproxTokens int      `json:"approx_tokens"`
	Bytes        int      `json:"bytes"`
	SHA256       string   `json:"sha256"`
	Sources      []string `json:"sources"`
}

// DefaultCorpusRoots are the platform-owned text roots the KL corpus draws from
// (relative to the repo root). Fixed = versioned.
func DefaultCorpusRoots() []string { return []string{"Spec", "Research", "Docs"} }

// AssembleCorpus walks the roots (relative to repoRoot) in sorted order,
// concatenating .md/.txt files until ~targetTokens is reached (bytes/4). The
// result is deterministic: same repo state ⇒ byte-identical corpus.
func AssembleCorpus(repoRoot string, roots []string, targetTokens int) (Corpus, error) {
	if targetTokens <= 0 {
		targetTokens = 250_000
	}
	var files []string
	for _, root := range roots {
		base := filepath.Join(repoRoot, root)
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // a missing root is skipped, not fatal
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".md" || ext == ".txt" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return Corpus{}, fmt.Errorf("battery: walk corpus root %s: %w", base, err)
		}
	}
	sort.Strings(files)

	var b strings.Builder
	var used []string
	budgetBytes := targetTokens * 4
	for _, f := range files {
		if b.Len() >= budgetBytes {
			break
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		b.Write(raw)
		b.WriteString("\n\n")
		rel, _ := filepath.Rel(repoRoot, f)
		used = append(used, rel)
	}
	text := b.String()
	sum := sha256.Sum256([]byte(text))
	return Corpus{
		Text:         text,
		ApproxTokens: len(text) / 4,
		Bytes:        len(text),
		SHA256:       hex.EncodeToString(sum[:]),
		Sources:      used,
	}, nil
}

// WriteCorpus materializes the assembled corpus text to a path (for the
// llama-perplexity -f argument). Scratch file, never committed.
func WriteCorpus(path string, c Corpus) error {
	if err := os.WriteFile(path, []byte(c.Text), 0o644); err != nil {
		return fmt.Errorf("battery: write corpus %s: %w", path, err)
	}
	return nil
}
