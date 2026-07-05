package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// fakePatcher counts calls and captures the last refresh token seen.
type fakePatcher struct {
	calls    atomic.Int32
	lastRT   atomic.Value // string
	err      error
}

func (f *fakePatcher) PatchRefreshToken(_ context.Context, rt string) error {
	f.calls.Add(1)
	f.lastRT.Store(rt)
	return f.err
}

func TestPersistIfChanged_patchesWhenNew(t *testing.T) {
	fp := &fakePatcher{}
	s := &Supervisor{patcher: fp}
	s.persistIfChanged("rt-1")
	if got := fp.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
	if got := fp.lastRT.Load().(string); got != "rt-1" {
		t.Errorf("lastRT = %q, want rt-1", got)
	}
	// After a successful patch lastPersistedRT is updated.
	s.mu.RLock()
	persisted := s.lastPersistedRT
	s.mu.RUnlock()
	if persisted != "rt-1" {
		t.Errorf("lastPersistedRT = %q, want rt-1", persisted)
	}
}

func TestPersistIfChanged_skipsWhenUnchanged(t *testing.T) {
	fp := &fakePatcher{}
	s := &Supervisor{patcher: fp, lastPersistedRT: "rt-1"}
	s.persistIfChanged("rt-1")
	if got := fp.calls.Load(); got != 0 {
		t.Errorf("calls = %d, want 0 (unchanged token should not be patched)", got)
	}
}

func TestPersistIfChanged_nilPatcher(t *testing.T) {
	s := &Supervisor{}
	// Must not panic when patcher is nil (Docker Compose / bare metal).
	s.persistIfChanged("rt-1")
}

func TestPersistIfChanged_emptyToken(t *testing.T) {
	fp := &fakePatcher{}
	s := &Supervisor{patcher: fp}
	s.persistIfChanged("")
	if got := fp.calls.Load(); got != 0 {
		t.Errorf("calls = %d, want 0 (empty token should not be patched)", got)
	}
}

func TestPersistIfChanged_patchFailureDoesNotUpdateState(t *testing.T) {
	fp := &fakePatcher{err: errors.New("k8s API down")}
	s := &Supervisor{patcher: fp}
	s.persistIfChanged("rt-1")
	if got := fp.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
	// lastPersistedRT should remain empty — next attempt should retry.
	s.mu.RLock()
	persisted := s.lastPersistedRT
	s.mu.RUnlock()
	if persisted != "" {
		t.Errorf("lastPersistedRT = %q, want empty after failed patch", persisted)
	}
}
