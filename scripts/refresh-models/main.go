// refresh-models queries the ai-proxy upstream for each known provider's model
// catalogue and rewrites the corresponding tables in docs/providers-and-models.md.
// It also probes a candidate list of vendor prefixes and flags any that newly
// appear, since the upstream exposes no cross-provider catalogue endpoint.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
