//go:build windows

// get-token extracts a refresh token from Windows Credential Manager and
// writes it to a file suitable for mounting into the proxy container.
package main

import (
	"fmt"
	"os"

	"github.com/oidc-proxy/oidc-proxy/internal/wincred"
)

func main() {
	if err := run(os.Args[1:], wincred.WindowsStore{}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
