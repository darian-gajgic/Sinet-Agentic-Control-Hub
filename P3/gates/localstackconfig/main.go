// Command localstackconfig renders the llama-swap YAML for the demo world from
// TODAY'S committed manifest (P3-RW-11 R8/OQ4; Spec S12.2).
//
// It exists because the config is GENERATED, never hand-written or reused: the
// B4-5 smoke config on disk was rendered against that day's manifest and its
// pulled-seat set, and a stale copy would configure seats whose weights are not
// there (S12.3: a partial or absent GGUF must never be configured). This is the
// one-line program that lets a shell script get the same rendering the platform
// does, from internal/local's own generator — no second implementation of the
// YAML shape exists anywhere.
//
// It writes YAML to stdout and reads its inputs from the environment, the way
// every other SINET_LOCAL_* value travels (structural config, composition-root
// passthrough — the SINET_SRT_PATH precedent; CONVENTIONS §26 R28, no ⚙ key).
// The TTL/grace knobs ARE ⚙ and are read by dotted key from the registry's
// declared defaults.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "localstackconfig:", err)
		os.Exit(1)
	}
}

func run() error {
	reg := settings.New()
	ttlFast, err := reg.Int("local.ttl.fast_s")
	if err != nil {
		return fmt.Errorf("read ⚙ local.ttl.fast_s: %w", err)
	}
	ttlWorkhorse, err := reg.Int("local.ttl.workhorse_s")
	if err != nil {
		return fmt.Errorf("read ⚙ local.ttl.workhorse_s: %w", err)
	}
	grace, err := reg.Int("local.unload.term_grace_s")
	if err != nil {
		return fmt.Errorf("read ⚙ local.unload.term_grace_s: %w", err)
	}

	uuids, err := gpuUUIDs()
	if err != nil {
		return err
	}
	cfg, err := local.GenerateConfig(local.ConfigParams{
		ModelCacheDir: os.Getenv("SINET_LOCAL_MODEL_CACHE"),
		LlamaServer:   os.Getenv("SINET_LOCAL_LLAMA_SERVER"),
		GPUUUIDs:      uuids,
		TTLFastS:      ttlFast,
		TTLWorkhorseS: ttlWorkhorse,
		UnloadGraceS:  grace,
	})
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(cfg)
	return err
}

// gpuUUIDs takes the placement UUID from the environment when the caller
// resolved one (the script does, and says how), and otherwise live-reads it the
// way the platform does. Placement is by UUID, never index (S12.2), so an
// unresolvable UUID is a loud failure and never a fabricated value.
func gpuUUIDs() ([]string, error) {
	if u := os.Getenv("SINET_LOCAL_GPU_UUID"); u != "" {
		return []string{u}, nil
	}
	return local.GPUUUIDs(context.Background())
}
