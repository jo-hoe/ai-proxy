//go:build darwin

package main

import "github.com/jo-hoe/ai-proxy/internal/wincred"

// newStore returns the macOS Keychain store.
func newStore() wincred.Store { return wincred.KeychainStore{} }
