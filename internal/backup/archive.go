package backup

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// The S13.10 archive layer: `tar | zstd` (the age layer wraps this, S13.10).
// The tar holds the text-first `.dump` of the durable set plus the platform's
// git-versioned file stores (worker templates [S08], knowledge/memory files
// [S09]) as text-first exports. NO binary DB file and NO secret ever enters —
// the pipeline reads platform.db (via the dump) + the file stores only, never
// the broker's credential store (secrets are a broker duty, deliberately
// decoupled; S13.10/G2 D2.5).

// NamedDir is one git-versioned file-store root to export text-first.
type NamedDir struct {
	Name string // logical name (its path prefix inside the archive)
	Dir  string // the store's working-tree root on the host
}

// archiveManifest rides inside every archive so the restore drill can rebuild
// without out-of-band knowledge.
type archiveManifest struct {
	UserVersion int    `json:"user_version"`
	CreatedTS   string `json:"created_ts"`
	DumpPath    string `json:"dump_path"`
}

const (
	manifestName = "manifest.json"
	dumpName     = "platform.dump.sql"
	storePrefix  = "filestores/"
)

// buildArchive builds the tar|zstd payload: the manifest, the text `.dump`, and
// each file-store's text-first export (its working-tree files, excluding .git).
// Deterministic file order (sorted) so archives diff cleanly.
func buildArchive(dumpSQL string, userVersion int, stores []NamedDir, now time.Time) ([]byte, error) {
	var zbuf bytes.Buffer
	zw, err := zstd.NewWriter(&zbuf)
	if err != nil {
		return nil, fmt.Errorf("backup: zstd writer: %w", err)
	}
	tw := tar.NewWriter(zw)

	man := archiveManifest{UserVersion: userVersion, CreatedTS: now.UTC().Format(time.RFC3339Nano), DumpPath: dumpName}
	manJSON, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeTarFile(tw, manifestName, manJSON); err != nil {
		return nil, err
	}
	if err := writeTarFile(tw, dumpName, []byte(dumpSQL)); err != nil {
		return nil, err
	}
	for _, store := range stores {
		if err := addStore(tw, store); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("backup: close tar: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("backup: close zstd: %w", err)
	}
	return zbuf.Bytes(), nil
}

// addStore adds a git-versioned file store's working-tree files (text-first
// export; .git excluded) under filestores/<name>/... in sorted order.
func addStore(tw *tar.Writer, store NamedDir) error {
	var rels []string
	err := filepath.WalkDir(store.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(store.Dir, path)
		if err != nil {
			return err
		}
		rels = append(rels, rel)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil // a not-yet-created store is empty, not an error
		}
		return fmt.Errorf("backup: walk store %q: %w", store.Name, err)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(store.Dir, rel))
		if err != nil {
			return fmt.Errorf("backup: read store file %q: %w", rel, err)
		}
		name := storePrefix + store.Name + "/" + filepath.ToSlash(rel)
		if err := writeTarFile(tw, name, body); err != nil {
			return err
		}
	}
	return nil
}

func writeTarFile(tw *tar.Writer, name string, body []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("backup: tar header %q: %w", name, err)
	}
	if _, err := tw.Write(body); err != nil {
		return fmt.Errorf("backup: tar write %q: %w", name, err)
	}
	return nil
}

// unpackedArchive is what the restore drill needs from a decrypted archive.
type unpackedArchive struct {
	DumpSQL     string
	UserVersion int
	// Stores maps "<store>/<rel>" to file content — the file-store export; the
	// drill's proof is the DB rebuild, but the stores travel for a full
	// restore (S13.10).
	Stores map[string]string
}

// unpackArchive reverses buildArchive: zstd-decompress then untar. The age
// layer is already peeled by the caller (the restore drill decrypts first).
func unpackArchive(zstdTar []byte) (unpackedArchive, error) {
	zr, err := zstd.NewReader(bytes.NewReader(zstdTar))
	if err != nil {
		return unpackedArchive{}, fmt.Errorf("backup: zstd reader: %w", err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	out := unpackedArchive{Stores: map[string]string{}}
	var man archiveManifest
	var haveMan bool
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return unpackedArchive{}, fmt.Errorf("backup: tar read: %w", err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return unpackedArchive{}, err
		}
		switch {
		case hdr.Name == manifestName:
			if err := json.Unmarshal(body, &man); err != nil {
				return unpackedArchive{}, fmt.Errorf("backup: parse manifest: %w", err)
			}
			haveMan = true
		case hdr.Name == dumpName:
			out.DumpSQL = string(body)
		case strings.HasPrefix(hdr.Name, storePrefix):
			out.Stores[strings.TrimPrefix(hdr.Name, storePrefix)] = string(body)
		}
	}
	if !haveMan {
		return unpackedArchive{}, fmt.Errorf("backup: archive has no manifest")
	}
	if out.DumpSQL == "" {
		return unpackedArchive{}, fmt.Errorf("backup: archive has no dump")
	}
	out.UserVersion = man.UserVersion
	return out, nil
}
