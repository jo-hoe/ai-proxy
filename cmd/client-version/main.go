// client-version bumps or verifies the proxy's client version across every file
// that pins it. config.go's defaultClientVersion const is the source of truth;
// the chart values, sample config, and generated chart README mirror it.
//
//	go run ./cmd/client-version -set 1.4.8   # rewrite all locations
//	go run ./cmd/client-version -check       # fail if any location disagrees
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
