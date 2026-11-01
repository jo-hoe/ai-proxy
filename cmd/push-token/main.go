//go:build windows || darwin

// push-token extracts a refresh token from the OS credential store (Windows
// Credential Manager or the macOS Keychain) and posts it to the proxy
// management API.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:], newStore()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
