package broker

// placed.go — the SECRET-FREE placement read (Spec S11.5).
//
// The control plane needs one fact at startup: which auth profiles a person
// has an engine credential placed under, so a lane document can become a
// provider entry that person actually holds. That is a question about a KIND,
// never about a secret, and S11.5 keeps the decision path holding zero secrets.
//
// So this read does four things it must never do otherwise:
//   - it reads the record's PLAINTEXT `kind` field only, the same field the
//     store's own kindOf reads to derive a signing posture;
//   - it never decrypts — resolve and Client.Resolve are out of bounds here;
//   - it never creates the master key, which is why OpenStore is not used: that
//     MkdirAlls, Chmods and GENERATES master.key when absent, so a control
//     plane calling it would change the host on every start with nothing
//     commissioned, and would hold a decrypting Store it has no business
//     holding;
//   - it never writes anything.
//
// A record that will not parse is a credential that is not placed, never an
// error that stops a control plane from starting: the failure an operator can
// act on is "this lane is not commissioned", not a platform that refuses to run
// because one file in a directory is corrupt.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// recordExt is the on-disk suffix of one stored credential (store.go's
// recordPath). It is the file-layout mechanic, not an operator choice.
const recordExt = ".cred"

// StoreRoot reports the per-person credential-store root under a state dir. It
// mirrors the default `sinet broker` computes for itself (mode.go), and is the
// ONE place that default lives: the daemon, the control plane and the key
// ceremony must all name the same directory, or a placed key is invisible to
// whichever of them looked somewhere else.
//
// A broker started with an explicit --store-dir points somewhere this
// derivation will never look; that limitation is recorded rather than papered
// over (CONVENTIONS §65).
func StoreRoot(stateDir string) string {
	return filepath.Join(stateDir, "broker-store")
}

// StorePeople lists the people who have a store under root, sorted. A root that
// does not exist is not an error: it is a host where nobody has placed
// anything.
func StorePeople(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if validProfile(e.Name()) != nil {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// PlacedEngineCreds reports which auth profiles one person's store holds an
// ENGINE-CRED under, without decrypting anything, creating anything or writing
// anything.
//
// Only KindEngineCred counts: the S11.5 destination constraint says an engine
// credential is the one kind delivered to an engine, so a signing-key or a
// git-ssh-key sitting under a lane's profile name commissions nothing. That
// constraint lives here, in the package that defines the kinds, rather than
// being re-decided by every caller.
//
// A person with no store is not an error: it is a person with nothing placed.
func PlacedEngineCreds(root, user string) (map[string]bool, error) {
	if err := validProfile(user); err != nil {
		return nil, err
	}
	dir := filepath.Join(root, user)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	placed := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, recordExt) {
			continue
		}
		blob, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var rec profileRecord
		if json.Unmarshal(blob, &rec) != nil || rec.Kind != KindEngineCred {
			continue
		}
		placed[strings.TrimSuffix(name, recordExt)] = true
	}
	return placed, nil
}
