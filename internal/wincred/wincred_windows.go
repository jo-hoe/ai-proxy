//go:build windows

package wincred

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	advapi32     = syscall.NewLazyDLL("advapi32.dll")
	procCredEnum = advapi32.NewProc("CredEnumerateW")
	procCredFree = advapi32.NewProc("CredFree")
)

// credentialW mirrors the Win32 CREDENTIALW struct layout.
type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        [2]uint32
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// WindowsStore implements Store using the Win32 Credential Manager API.
type WindowsStore struct{}

// FindByPrefix returns all credentials whose target starts with prefix.
// The wildcard suffix is appended automatically (e.g. "foo*").
func (WindowsStore) FindByPrefix(prefix string) ([]Credential, error) {
	prefixPtr, err := syscall.UTF16PtrFromString(prefix + "*")
	if err != nil {
		return nil, fmt.Errorf("wincred: encode prefix: %w", err)
	}

	var count uint32
	// credArrayPtr is a *[]*credentialW managed by advapi32; freed via CredFree.
	var credArrayPtr *[1 << 16]*credentialW

	r, _, e := procCredEnum.Call(
		uintptr(unsafe.Pointer(prefixPtr)),
		0,
		uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&credArrayPtr)),
	)
	if r == 0 {
		if e == syscall.ERROR_NOT_FOUND {
			return nil, nil
		}
		return nil, fmt.Errorf("wincred: CredEnumerateW: %w", e)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credArrayPtr)))

	results := make([]Credential, 0, count)
	for _, cred := range credArrayPtr[:count] {
		target := syscall.UTF16ToString(
			(*[1 << 14]uint16)(unsafe.Pointer(cred.TargetName))[:],
		)
		token := ""
		if cred.CredentialBlobSize > 0 {
			token = string((*[1 << 20]byte)(unsafe.Pointer(cred.CredentialBlob))[:cred.CredentialBlobSize])
		}
		results = append(results, Credential{Target: target, Token: token})
	}
	return results, nil
}
