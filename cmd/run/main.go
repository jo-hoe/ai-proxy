//go:build windows

// run builds (if needed) and starts the proxy container on Windows.
package main

import (
	"fmt"
	"os"

	"github.com/jo-hoe/ai-proxy/internal/wincred"
)

func main() {
	if err := run(os.Args[1:], wincred.WindowsStore{}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
