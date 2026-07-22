package review

import (
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDF text extraction for the S13.2 extracted-text diff — the ONLY v0 PDF
// comparison surface (G2 Def.13: MIT/BSD-licensed extraction; AGPL
// extractors excluded as defaults). github.com/ledongthuc/pdf is the
// adopted extractor (BSD-3-Clause, the rsc.io/pdf lineage; pin + funeral
// plan in components.lock).

// ExtractPDFText returns the plain reading text of a PDF file.
func ExtractPDFText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("review: open pdf: %w", err)
	}
	defer f.Close()
	text, err := extractPlainText(r)
	if err != nil {
		return "", fmt.Errorf("review: extract pdf text: %w", err)
	}
	return text, nil
}

func extractPlainText(r *pdf.Reader) (_ string, err error) {
	// The extractor panics on some malformed structures (it predates
	// errors-everywhere); a panic is converted to an error so a corrupt
	// PDF degrades to the labeled metadata-card fallback, never a crash
	// (Spec S13.2: no type refused, honest fallback).
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("pdf extractor panic: %v", rec)
		}
	}()
	reader, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if _, err := io.Copy(&sb, reader); err != nil {
		return "", err
	}
	return sb.String(), nil
}
