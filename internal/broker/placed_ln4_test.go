package broker

// placed_ln4_test.go — P3-LN-4 / T7 (S11.5, R3).
//
// The control plane asks the store ONE question at startup: "is a credential
// placed under this profile, and is it an engine-cred?" That is a question
// about a KIND, never about a secret, and the reader that answers it must be
// able to answer it on a host where nothing has ever been placed without
// changing that host.
//
// $0, and no secret: every record here is written by hand with an EMPTY
// ciphertext. The reader never decrypts, so it never needs one.

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// writeRecord materialises one credential record by hand — no store is opened,
// so no master key is ever created and nothing here holds key material.
func writeRecord(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("store dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// dirSnapshot records every path under root with its mode, size and mtime.
type dirSnapshot map[string]string

func snapshotDir(t *testing.T, root string) dirSnapshot {
	t.Helper()
	out := dirSnapshot{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		out[rel] = info.Mode().String() + " " + info.ModTime().UTC().Format("2006-01-02T15:04:05.000000000Z") +
			" " + strconv.FormatInt(info.Size(), 10)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return out
}

// T7 · the presence read is secret-free and side-effect-free.
func TestLN4PresenceReadIsSecretFreeAndSideEffectFree(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "me")
	writeRecord(t, dir, "zai-coding-plan.cred", `{"kind":"engine-cred","nonce":"","ct":""}`)

	before := snapshotDir(t, root)
	if len(before) < 2 {
		t.Fatalf("the snapshot saw %d paths — the before/after comparison would be vacuous", len(before))
	}

	placed, err := PlacedEngineCreds(root, "me")
	if err != nil {
		t.Fatalf("PlacedEngineCreds: %v", err)
	}
	if !placed["zai-coding-plan"] {
		t.Errorf("the placed engine-cred was not reported: %v", placed)
	}

	// The master key is what OpenStore would have created. A read must never.
	if _, err := os.Stat(filepath.Join(dir, "master.key")); !os.IsNotExist(err) {
		t.Errorf("reading the store created master.key (stat err = %v). A commissioning probe that "+
			"generated a key would change the host on every start with nothing commissioned, and would "+
			"hold a decrypting key it has no business holding (S11.5).", err)
	}
	after := snapshotDir(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("the store changed under a read.\nbefore: %v\nafter:  %v", before, after)
	}

	// Secret-free BY TYPE: the exported reader answers a question about a kind,
	// so there is no signature on it that could carry credential material.
	if got := reflect.TypeOf(PlacedEngineCreds).Out(0); got.String() != "map[string]bool" {
		t.Errorf("PlacedEngineCreds returns %s — the decision path holds zero secrets (S11.5), so the "+
			"reader must answer with a placement fact and never with stored material", got)
	}
	if got := reflect.TypeOf(StorePeople).Out(0); got.String() != "[]string" {
		t.Errorf("StorePeople returns %s, want the person names alone", got)
	}

	// An absent store root is not an error: it is a host with nothing placed.
	missing := filepath.Join(root, "no-such-root")
	people, err := StorePeople(missing)
	if err != nil {
		t.Errorf("StorePeople(absent root) = %v, want a clean empty result", err)
	}
	if len(people) != 0 {
		t.Errorf("StorePeople(absent root) = %v, want none", people)
	}
	gone, err := PlacedEngineCreds(missing, "me")
	if err != nil {
		t.Errorf("PlacedEngineCreds(absent root) = %v, want a clean empty result", err)
	}
	if len(gone) != 0 {
		t.Errorf("PlacedEngineCreds(absent root) = %v, want none", gone)
	}
	if none, err := PlacedEngineCreds(root, "nobody"); err != nil || len(none) != 0 {
		t.Errorf("PlacedEngineCreds(person with no store) = %v, %v — a person who has placed nothing is "+
			"not an error", none, err)
	}
}

// T7 (second half) · only an ENGINE-CRED counts, and a corrupt record is a lane
// that is not commissioned rather than a control plane that will not start.
func TestLN4PlacedEngineCredsIgnoresEverythingElse(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "me")
	writeRecord(t, dir, "good.cred", `{"kind":"engine-cred","nonce":"","ct":""}`)
	writeRecord(t, dir, "signer.cred", `{"kind":"signing-key","nonce":"","ct":""}`)
	writeRecord(t, dir, "git.cred", `{"kind":"git-ssh-key","nonce":"","ct":""}`)
	writeRecord(t, dir, "junk.cred", `{"kind":"not-a-kind","nonce":"","ct":""}`)
	writeRecord(t, dir, "broken.cred", `{"kind":`)
	writeRecord(t, dir, "notarecord.json", `{"kind":"engine-cred"}`)
	writeRecord(t, dir, "master.key", "0123456789abcdef0123456789abcdef")

	placed, err := PlacedEngineCreds(root, "me")
	if err != nil {
		t.Fatalf("PlacedEngineCreds: %v — a corrupt record must not stop a control plane from starting", err)
	}
	if len(placed) != 1 || !placed["good"] {
		t.Errorf("placed = %v, want exactly the engine-cred profile. Only an engine-cred commissions a "+
			"lane: the S11.5 destination constraint is what stops a signing key sitting under a lane's "+
			"profile name from being delivered to an engine.", placed)
	}
}

// T7 (third half) · the people are the store directories, sorted.
func TestLN4StorePeopleListsStoreDirectories(t *testing.T) {
	root := t.TempDir()
	for _, who := range []string{"sinep", "alice", "bob"} {
		writeRecord(t, filepath.Join(root, who), "x.cred", `{"kind":"engine-cred","nonce":"","ct":""}`)
	}
	// A loose file in the root is not a person.
	writeRecord(t, root, "stray.txt", "not a store")

	people, err := StorePeople(root)
	if err != nil {
		t.Fatalf("StorePeople: %v", err)
	}
	want := []string{"alice", "bob", "sinep"}
	if !reflect.DeepEqual(people, want) {
		t.Errorf("people = %v, want %v (sorted, directories only — a loose file in the store root is not "+
			"a person, and the order is what makes a startup log line comparable between two runs)", people, want)
	}
}

// T7 (fourth half) · the store root is derived in exactly ONE place, and it is
// the same default `sinet broker` computes for itself (mode.go).
func TestLN4StoreRootMirrorsTheBrokerDefault(t *testing.T) {
	if got, want := StoreRoot("/var/lib/sinet"), filepath.Join("/var/lib/sinet", "broker-store"); got != want {
		t.Errorf("StoreRoot = %q, want %q — the control plane and the broker daemon must look at the same "+
			"directory, or a placed key is invisible to commissioning", got, want)
	}
}
