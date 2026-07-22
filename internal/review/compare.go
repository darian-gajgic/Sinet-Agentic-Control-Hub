package review

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Per-type comparison behavior (Spec S13.2): every deliverable type has a
// defined comparison at v0 — NO type is refused; a type without a rich
// surface gets the honest fallback, visibly labeled (the 2.1 honesty
// posture). Behavior is normative here; RENDERING (diff widget, the
// 2-up/swipe/onion image trio, metadata cards) is Spec S15's.

// Comparison surfaces.
const (
	SurfaceLineDiff      = "line-diff"           // code/text: host-side git diff
	SurfaceExtractedText = "extracted-text-diff" // PDF (and text-extractable fallback)
	SurfaceImagePair     = "image-pair"          // the two-revision surface the S15 trio renders
	SurfaceBinaryCards   = "binary-cards"        // metadata card per side + hash verdict
)

// Comparison is the host-side comparison of two revisions of one
// deliverable. Any revision pair is diffable on demand — revision-over-
// revision navigation is a schema capability, not a UI nicety (Spec
// S13.1).
type Comparison struct {
	DeliverableID string `json:"deliverable_id"`
	Type          string `json:"type"`
	OldN          int    `json:"old_n"` // 0 = pre-task base (S13.5 materializes it, B4-2)
	NewN          int    `json:"new_n"`
	Surface       string `json:"surface"`
	// Fallback marks the labeled honest-fallback surface (Spec S13.2 "any
	// other" row); Label carries the visible labeling text.
	Fallback bool   `json:"fallback,omitempty"`
	Label    string `json:"label,omitempty"`
	// Unified is the host-side unified diff for diff surfaces.
	Unified string `json:"unified,omitempty"`
	// OldObjects/NewObjects are the per-side object refs (image/binary
	// surfaces — the metadata cards' data).
	OldObjects []ObjectRef `json:"old_objects,omitempty"`
	NewObjects []ObjectRef `json:"new_objects,omitempty"`
	// Changed is the by-hash verdict for object surfaces.
	Changed bool `json:"changed,omitempty"`
	// PixelDiff is the optional server-side aid for images (nil seam at
	// v0; Spec S13.2).
	PixelDiff *PixelDiffResult `json:"pixel_diff,omitempty"`
}

// PixelDiffAid is the optional S13.2 image pixel-diff seam. v0 wires nil —
// the two-revision surface alone feeds the S15 trio; a future
// implementation slots in without schema change.
type PixelDiffAid interface {
	PixelDiff(ctx context.Context, oldPath, newPath string) (PixelDiffResult, error)
}

// PixelDiffResult is a server-side pixel-diff aid product.
type PixelDiffResult struct {
	DiffObjectSHA string  `json:"diff_object_sha,omitempty"`
	ChangedRatio  float64 `json:"changed_ratio,omitempty"`
}

// Compare compares two revisions of a deliverable per its type. oldN 0 is
// the pre-task base: empty until the S13.5 git topology materializes
// branch bases (B4-2) — revision 1 then presents against project HEAD at
// branch base (Spec S13.1).
func (s *Store) Compare(ctx context.Context, deliverableID string, oldN, newN int) (Comparison, error) {
	d, err := s.Deliverable(ctx, deliverableID)
	if err != nil {
		return Comparison{}, err
	}
	if newN < 1 || oldN < 0 || oldN == newN {
		return Comparison{}, fmt.Errorf("%w: compare needs distinct revisions (old %d, new %d)", ErrBadInput, oldN, newN)
	}
	newRev, err := s.RevisionAt(ctx, deliverableID, newN)
	if err != nil {
		return Comparison{}, err
	}
	var oldRev Revision
	if oldN > 0 {
		oldRev, err = s.RevisionAt(ctx, deliverableID, oldN)
		if err != nil {
			return Comparison{}, err
		}
	}
	out := Comparison{DeliverableID: deliverableID, Type: d.Type, OldN: oldN, NewN: newN}

	switch {
	case isLineDiffType(d.Type) && newRev.PinKind == "content":
		out.Surface = SurfaceLineDiff
		out.Unified, err = s.contentDiff(ctx, deliverableID, oldN, newN)
		return out, err

	case strings.EqualFold(d.Type, "pdf"):
		// Extracted-text diff ONLY at v0 (G2 Def.13); the pixel-overlay
		// lane is post-v0. Extraction failure degrades to the metadata
		// cards, labeled — never a refusal.
		out.Surface = SurfaceExtractedText
		out.Label = "extracted-text diff (v0 PDF surface; pixel overlay is post-v0)"
		oldText, oldErr := s.revisionPDFText(oldN, oldRev)
		newText, newErr := s.revisionPDFText(newN, newRev)
		if oldErr != nil || newErr != nil {
			out.Surface = SurfaceBinaryCards
			out.Fallback = true
			out.Label = fmt.Sprintf("PDF text extraction unavailable (%v): metadata cards", firstErr(oldErr, newErr))
			out.OldObjects, out.NewObjects = oldRev.Objects, newRev.Objects
			out.Changed = objectsChanged(oldRev.Objects, newRev.Objects)
			return out, nil
		}
		out.Unified, err = gitDiff(oldText, newText)
		return out, err

	case isImageType(d.Type):
		// The two-revision surface as data; 2-up/swipe/onion is S15
		// rendering. The optional pixel-diff aid rides the seam.
		out.Surface = SurfaceImagePair
		out.OldObjects, out.NewObjects = oldRev.Objects, newRev.Objects
		out.Changed = objectsChanged(oldRev.Objects, newRev.Objects)
		if s.PixelDiff != nil && len(oldRev.Objects) == 1 && len(newRev.Objects) == 1 && out.Changed {
			res, err := s.PixelDiff.PixelDiff(ctx,
				s.ObjectPath(oldRev.Objects[0].SHA256), s.ObjectPath(newRev.Objects[0].SHA256))
			if err != nil {
				// The aid is optional; its failure never blocks the
				// surface — recorded in the label, not fatal.
				out.Label = fmt.Sprintf("pixel-diff aid unavailable: %v", err)
			} else {
				out.PixelDiff = &res
			}
		}
		return out, nil

	case newRev.PinKind == "content":
		// Any other text-extractable type: the plain extracted-text diff,
		// labeled as the fallback surface (Spec S13.2).
		out.Surface = SurfaceExtractedText
		out.Fallback = true
		out.Label = fmt.Sprintf("fallback surface: plain text diff (no rich comparison for type %q at v0)", d.Type)
		out.Unified, err = s.contentDiff(ctx, deliverableID, oldN, newN)
		return out, err

	default:
		// Binaries / opaque: metadata card per side + changed/unchanged by
		// hash; download to inspect (Spec S13.2; G2 Def.14).
		out.Surface = SurfaceBinaryCards
		if !isBinaryType(d.Type) {
			out.Fallback = true
			out.Label = fmt.Sprintf("fallback surface: metadata cards (no rich comparison for type %q at v0)", d.Type)
		}
		out.OldObjects, out.NewObjects = oldRev.Objects, newRev.Objects
		out.Changed = objectsChanged(oldRev.Objects, newRev.Objects)
		return out, nil
	}
}

// contentDiff renders the host-side unified line diff between two content
// revisions (`git diff` between revision pins, Spec S13.2), per file with
// headers when a revision carries several.
func (s *Store) contentDiff(ctx context.Context, deliverableID string, oldN, newN int) (string, error) {
	newFiles, err := s.RevisionFiles(ctx, deliverableID, newN)
	if err != nil {
		return "", err
	}
	oldFiles := map[string]string{}
	if oldN > 0 {
		oldFiles, err = s.RevisionFiles(ctx, deliverableID, oldN)
		if err != nil {
			return "", err
		}
	}
	names := map[string]bool{}
	for n := range oldFiles {
		names[n] = true
	}
	for n := range newFiles {
		names[n] = true
	}
	ordered := make([]string, 0, len(names))
	for n := range names {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)
	var sb strings.Builder
	for _, name := range ordered {
		u, err := gitDiff(oldFiles[name], newFiles[name])
		if err != nil {
			return "", err
		}
		if u == "" {
			continue
		}
		if len(ordered) > 1 {
			fmt.Fprintf(&sb, "=== %s ===\n", name)
		}
		sb.WriteString(u)
	}
	return sb.String(), nil
}

func (s *Store) revisionPDFText(n int, rev Revision) (string, error) {
	if n == 0 {
		return "", nil // pre-task base: empty side
	}
	if len(rev.Objects) == 0 {
		return "", fmt.Errorf("revision %d pins no PDF object", n)
	}
	var sb strings.Builder
	for _, ref := range rev.Objects {
		if _, err := s.readObject(ref.SHA256); err != nil {
			return "", err
		}
		text, err := ExtractPDFText(s.ObjectPath(ref.SHA256))
		if err != nil {
			return "", fmt.Errorf("extract %s: %w", ref.Name, err)
		}
		if len(rev.Objects) > 1 {
			fmt.Fprintf(&sb, "=== %s ===\n", ref.Name)
		}
		sb.WriteString(text)
	}
	return sb.String(), nil
}

func objectsChanged(oldRefs, newRefs []ObjectRef) bool {
	return !sameRefs(oldRefs, newRefs)
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func isLineDiffType(t string) bool {
	switch strings.ToLower(t) {
	case "code", "text", "markdown":
		return true
	}
	return false
}

func isImageType(t string) bool {
	switch strings.ToLower(t) {
	case "image", "png", "jpeg", "jpg", "gif", "webp", "svg":
		return true
	}
	return false
}

func isBinaryType(t string) bool {
	switch strings.ToLower(t) {
	case "binary", "archive", "executable":
		return true
	}
	return false
}
