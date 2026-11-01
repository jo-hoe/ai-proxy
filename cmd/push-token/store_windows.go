//go:build windows

package main

import "github.com/jo-hoe/ai-proxy/internal/wincred"

// newStore returns the Windows Credential Manager store.
func newStore() wincred.Store { return wincred.WindowsStore{} }
