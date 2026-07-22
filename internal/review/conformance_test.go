package review_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exactly ONE drain path (Spec S13.4; FC-v1 §2), pinned structurally: the
// SQL that stamps consumption exists in drain.go alone, review-comment
// writes exist only inside this package, and no other package touches the
// review_comments table — the whole repo source is scanned (the §17/§18
// conformance-scan precedent).
func TestOneDrainPathConformance(t *testing.T) {
	root := repoRoot(t)

	var consumeSites, commentWriteSites []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(raw)
		rel, _ := filepath.Rel(root, path)
		if strings.Contains(src, "status = 'consumed'") {
			consumeSites = append(consumeSites, rel)
		}
		for _, stmt := range []string{"INSERT INTO review_comments", "UPDATE review_comments", "DELETE FROM review_comments"} {
			if strings.Contains(src, stmt) {
				commentWriteSites = append(commentWriteSites, rel+" ("+stmt+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(consumeSites) != 1 || !strings.HasSuffix(consumeSites[0], filepath.Join("internal", "review", "drain.go")) {
		t.Fatalf("consumption must be written by THE drain alone (Spec S13.4): %v", consumeSites)
	}
	for _, site := range commentWriteSites {
		if !strings.Contains(site, filepath.Join("internal", "review")+string(filepath.Separator)) {
			t.Fatalf("review_comments written outside internal/review: %v", commentWriteSites)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above the test binary")
		}
		dir = parent
	}
}
