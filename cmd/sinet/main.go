// Command sinet is the Sinet platform's single release artifact: one static
// Go binary invoked in dedicated modes (control, broker, portpool), per Spec
// S01.5. Mode dispatch lives in internal/cli.
package main

import (
	"os"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
