package sandbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// run_launch.go is the `sinet run-launch` mode — the S11.8 run launcher: the
// fixed, non-agent-reachable ExecStart of the root-installed template
// sinet-run@<run_id>.service (ExecStart=/usr/lib/sinet/run-launch %i, Shape B).
// It runs UNPRIVILEGED: it reads the sinet-owned spool record the control
// plane wrote, schema-validates it, composes the S11.1 stack, and execs bwrap.
// The privileged command line is fixed in the shipped unit; run-launch is
// handed only DATA (the job record), never properties — that is precisely why
// the name-scoped polkit grant is property-safe (S11.8: polkit cannot
// constrain transient-unit properties).
//
// The same composition core (Compose) powers the dev in-process path
// (confiner.go). At B1 the host unit is not installed (no host changes); this
// mode is the faithful host entry, exercised directly by tests and usable by
// the operator once the template unit is installed at a gate.

// JobRecord is the S11.8 spool record: the compiled confinement plus the
// engine spawn. It is DATA only. Written by the control plane to a sinet-owned
// spool file (/run/sinet/jobs/<run_id>.json under systemd), read here.
type JobRecord struct {
	Class        string   `json:"class"`
	Argv         []string `json:"argv"`
	Env          []string `json:"env"`
	Workspace    string   `json:"workspace,omitempty"`
	ROConfig     []string `json:"ro_config,omitempty"`
	RWExchange   []string `json:"rw_exchange,omitempty"`
	EnginePrefix string   `json:"engine_prefix,omitempty"`
}

// RunLaunch executes one `sinet run-launch --job <path>` invocation: read +
// validate the spool record, compose the sandbox, exec bwrap with the engine
// inside it. It returns the sandboxed process's exit code (or 1 on a
// composition/spawn failure) so systemd (or a dev caller) sees the true
// disposition.
func RunLaunch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sinet run-launch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	job := fs.String("job", "", "path to the confinement/spawn spool record (S11.8)")
	printOnly := fs.Bool("print", false, "compose and print the sandbox invocation without running it (inspection aid)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *job == "" {
		fmt.Fprintln(stderr, "sinet run-launch: --job <path> is required (S11.8 spool record)")
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sinet run-launch: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	blob, err := os.ReadFile(*job)
	if err != nil {
		fmt.Fprintf(stderr, "sinet run-launch: read job: %v\n", err)
		return 1
	}
	var rec JobRecord
	dec := json.NewDecoder(bytes.NewReader(blob))
	dec.DisallowUnknownFields() // schema-validate: reject any unexpected field
	if err := dec.Decode(&rec); err != nil {
		fmt.Fprintf(stderr, "sinet run-launch: invalid job record: %v\n", err)
		return 1
	}

	caps := Probe()
	class := Class(rec.Class)
	sp := Spawn{
		Argv: rec.Argv, Env: rec.Env, Workspace: rec.Workspace,
		ROConfig: rec.ROConfig, RWExchange: rec.RWExchange, EnginePrefix: rec.EnginePrefix,
	}
	// Egress for the srt path: the host launcher has no settings registry, so
	// the per-class domain allow-list (C0 single host / C2 registries) is
	// carried by the compiled spool record when the DEFERRED host egress
	// substrate lands at B2 — until then the class profile's mode is enough
	// (C1 deny-all; C0/C2 an empty allow-list, inert without the substrate).
	prof, err := Profile(class)
	if err != nil {
		fmt.Fprintf(stderr, "sinet run-launch: %v\n", err)
		return 1
	}
	// The srt config file (srt path only) lives beside the spool record.
	plan, err := BuildPlan(caps, class, sp, EgressPolicy{Mode: prof.Network}, filepath.Dir(*job))
	if err != nil {
		fmt.Fprintf(stderr, "sinet run-launch: compose: %v\n", err)
		return 1
	}
	defer plan.Cleanup()

	if *printOnly {
		// Inspection aid: show the exact composed sandbox invocation without
		// running it (the operator can audit a job's confinement + which path).
		fmt.Fprintf(stdout, "path=%s class=%s landlock-abi=%d\n", plan.Path, rec.Class, caps.LandlockABI)
		fmt.Fprintf(stdout, "%s %s\n", plan.Bin, strings.Join(plan.Argv, " "))
		return 0
	}

	// Host evidence: which composition path is active (srt primary / native
	// fallback) is recorded to the unit's journal.
	fmt.Fprintf(stderr, "sinet run-launch: composed via %s path (S11.1 srt primary / S16.3 native fallback)\n", plan.Path)
	cmd := exec.Command(plan.Bin, plan.Argv...)
	cmd.Env = plan.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = plan.ExtraFiles
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintf(stderr, "sinet run-launch: run: %v\n", err)
		return 1
	}
	return 0
}
