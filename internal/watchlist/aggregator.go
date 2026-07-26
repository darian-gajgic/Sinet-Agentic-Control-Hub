package watchlist

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// aggregator.go — the T3 weekly aggregator-API jobs (Spec S14.6 ¶T3): the
// models.dev `api.json` diff and the price/model-list sources.
//
// Diffs are STRUCTURAL — added, removed and changed leaf keys — never free
// text. The baseline is a compact map of leaf path → value hash, so a changed
// value is detectable without keeping the whole document; when a document's
// digest exceeds the structural cap the baseline degrades HONESTLY to the
// whole-body hash and the finding says the diff is coarse, rather than
// pretending to a key-level answer it does not have.

// structuralDigestCap bounds the stored baseline. Structural, not ⚙.
const structuralDigestCap = 512 << 10

// diffKeyCap bounds how many changed key paths ride a finding payload
// (⚙ state.event_payload_cap is the hard gate above it); the total counts are
// always reported, so a truncated list never hides the scale of a change.
const diffKeyCap = 40

// StructuralDiff is the outcome of comparing two structural digests.
type StructuralDiff struct {
	Added   []string
	Removed []string
	Changed []string
	// Coarse is true when no key-level comparison was possible (the document
	// exceeded the digest cap, or there is no prior baseline of that kind).
	Coarse bool
	// TotalAdded/Removed/Changed are the untruncated counts.
	TotalAdded   int
	TotalRemoved int
	TotalChanged int
}

// Empty reports whether the diff found nothing.
func (d StructuralDiff) Empty() bool {
	return !d.Coarse && d.TotalAdded == 0 && d.TotalRemoved == 0 && d.TotalChanged == 0
}

// Summary renders the diff as one line.
func (d StructuralDiff) Summary() string {
	if d.Coarse {
		return "document changed (coarse: the structural digest exceeded its cap, so no key-level diff is available)"
	}
	return fmt.Sprintf("%d keys added, %d removed, %d changed", d.TotalAdded, d.TotalRemoved, d.TotalChanged)
}

// Detail renders the bounded key lists for the finding payload.
func (d StructuralDiff) Detail() string {
	if d.Coarse {
		return d.Summary()
	}
	var b strings.Builder
	b.WriteString(d.Summary())
	writeKeys(&b, "added", d.Added, d.TotalAdded)
	writeKeys(&b, "removed", d.Removed, d.TotalRemoved)
	writeKeys(&b, "changed", d.Changed, d.TotalChanged)
	return b.String()
}

func writeKeys(b *strings.Builder, label string, keys []string, total int) {
	if total == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s (%d):", label, total)
	for _, k := range keys {
		b.WriteString("\n  ")
		b.WriteString(k)
	}
	if total > len(keys) {
		fmt.Fprintf(b, "\n  … and %d more", total-len(keys))
	}
}

// StructuralBaseline builds the leaf-path → value-hash digest of a JSON
// document and marshals it for storage. It returns ok=false when the document
// is not JSON or the digest exceeds the cap; the caller then falls back to the
// whole-body hash.
func StructuralBaseline(body []byte) (string, bool) {
	digest, err := flattenJSON(body)
	if err != nil {
		return "", false
	}
	enc, err := json.Marshal(digest)
	if err != nil || len(enc) > structuralDigestCap {
		return "", false
	}
	return string(enc), true
}

// DiffStructural compares a stored baseline against a fresh document body.
func DiffStructural(baseline string, body []byte) StructuralDiff {
	fresh, err := flattenJSON(body)
	if err != nil || baseline == "" {
		return StructuralDiff{Coarse: true}
	}
	var prev map[string]string
	if err := json.Unmarshal([]byte(baseline), &prev); err != nil {
		return StructuralDiff{Coarse: true}
	}
	var d StructuralDiff
	for path, hash := range fresh {
		old, ok := prev[path]
		switch {
		case !ok:
			d.TotalAdded++
			if len(d.Added) < diffKeyCap {
				d.Added = append(d.Added, path)
			}
		case old != hash:
			d.TotalChanged++
			if len(d.Changed) < diffKeyCap {
				d.Changed = append(d.Changed, path)
			}
		}
	}
	for path := range prev {
		if _, ok := fresh[path]; !ok {
			d.TotalRemoved++
			if len(d.Removed) < diffKeyCap {
				d.Removed = append(d.Removed, path)
			}
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Strings(d.Changed)
	return d
}

// flattenJSON walks a decoded JSON document into leaf path → short value hash.
// Numbers are decoded with UseNumber so an integer never round-trips through a
// float and reads as a spurious change.
func flattenJSON(body []byte) (map[string]string, error) {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("watchlist: parse aggregator document: %w", err)
	}
	out := map[string]string{}
	walkJSON("$", doc, out)
	return out, nil
}

func walkJSON(path string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			out[path] = "{}"
			return
		}
		for k, child := range t {
			walkJSON(path+"."+k, child, out)
		}
	case []any:
		if len(t) == 0 {
			out[path] = "[]"
			return
		}
		for i, child := range t {
			walkJSON(path+"["+strconv.Itoa(i)+"]", child, out)
		}
	default:
		out[path] = shortHash(fmt.Sprint(v))
	}
}
