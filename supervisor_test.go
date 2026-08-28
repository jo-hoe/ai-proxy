package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestReverseProxy_InjectsClientVersionHeader(t *testing.T) {
	var gotAppVersion, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAppVersion = r.Header.Get(clientVersionHeader)
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream: %v", err)
	}
	s := &Supervisor{
		upstream:    u,
		appVersion:  clientVersionPrefix + "1.4.5",
		accessToken: "tok-123",
	}
	s.reverseProxy = s.buildReverseProxy()

	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if want := clientVersionPrefix + "1.4.5"; gotAppVersion != want {
		t.Errorf("client version header = %q, want %q", gotAppVersion, want)
	}
	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", gotAuth)
	}
}
