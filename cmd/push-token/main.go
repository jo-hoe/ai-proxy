//go:build windows

// push-token extracts a refresh token from Windows Credential Manager and
// posts it to the proxy management API.
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
