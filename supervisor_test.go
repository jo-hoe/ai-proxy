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

// fakePatcher counts calls and captures the last credentials seen.
type fakePatcher struct {
	calls   atomic.Int32
	lastEP  atomic.Value // string
	lastCID atomic.Value // string
	lastRT  atomic.Value // string
	err     error
}

func (f *fakePatcher) PatchCredentials(_ context.Context, endpoint, clientID, rt string) error {
	f.calls.Add(1)
	f.lastEP.Store(endpoint)
	f.lastCID.Store(clientID)
	f.lastRT.Store(rt)
	return f.err
}

func TestPersistCredentialsIfChanged_patchesWhenNew(t *testing.T) {
	fp := &fakePatcher{}
	s := &Supervisor{patcher: fp}
	s.persistCredentialsIfChanged("ep-1", "cid-1", "rt-1")
	if got := fp.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
	if got := fp.lastRT.Load().(string); got != "rt-1" {
		t.Errorf("lastRT = %q, want rt-1", got)
	}
	if got := fp.lastEP.Load().(string); got != "ep-1" {
		t.Errorf("lastEP = %q, want ep-1", got)
	}
	// After a successful patch the persisted triple is updated.
	s.mu.RLock()
	persistedRT, persistedEP, persistedCID := s.lastPersistedRT, s.lastPersistedEP, s.lastPersistedCID
	s.mu.RUnlock()
	if persistedRT != "rt-1" || persistedEP != "ep-1" || persistedCID != "cid-1" {
		t.Errorf("persisted = (%q,%q,%q), want (ep-1,cid-1,rt-1)", persistedEP, persistedCID, persistedRT)
	}
}

func TestPersistCredentialsIfChanged_skipsWhenUnchanged(t *testing.T) {
	fp := &fakePatcher{}
	s := &Supervisor{patcher: fp, lastPersistedEP: "ep-1", lastPersistedCID: "cid-1", lastPersistedRT: "rt-1"}
	s.persistCredentialsIfChanged("ep-1", "cid-1", "rt-1")
	if got := fp.calls.Load(); got != 0 {
		t.Errorf("calls = %d, want 0 (unchanged credentials should not be patched)", got)
	}
}

func TestPersistCredentialsIfChanged_patchesWhenEndpointChangesButRTSame(t *testing.T) {
	// A secretless push sets the RT first; later the endpoint/client-id must
	// still be persisted even if the RT itself has not changed.
	fp := &fakePatcher{}
	s := &Supervisor{patcher: fp, lastPersistedRT: "rt-1"}
	s.persistCredentialsIfChanged("ep-1", "cid-1", "rt-1")
	if got := fp.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 (new endpoint/client-id should trigger patch)", got)
	}
}

func TestPersistCredentialsIfChanged_nilPatcher(t *testing.T) {
	s := &Supervisor{}
	// Must not panic when patcher is nil (Docker Compose / bare metal).
	s.persistCredentialsIfChanged("ep-1", "cid-1", "rt-1")
}

func TestPersistCredentialsIfChanged_emptyToken(t *testing.T) {
	fp := &fakePatcher{}
	s := &Supervisor{patcher: fp}
	s.persistCredentialsIfChanged("ep-1", "cid-1", "")
	if got := fp.calls.Load(); got != 0 {
		t.Errorf("calls = %d, want 0 (empty token should not be patched)", got)
	}
}

func TestPersistCredentialsIfChanged_patchFailureDoesNotUpdateState(t *testing.T) {
	fp := &fakePatcher{err: errors.New("k8s API down")}
	s := &Supervisor{patcher: fp}
	s.persistCredentialsIfChanged("ep-1", "cid-1", "rt-1")
	if got := fp.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
	// Persisted state should remain empty — next attempt should retry.
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
		appVersion:  clientVersionPrefix + defaultClientVersion,
		accessToken: "tok-123",
	}
	s.reverseProxy = s.buildReverseProxy()

	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if want := clientVersionPrefix + defaultClientVersion; gotAppVersion != want {
		t.Errorf("client version header = %q, want %q", gotAppVersion, want)
	}
	if gotAuth != "Bearer tok-123" {
		t.Errorf("Authorization = %q, want Bearer tok-123", gotAuth)
	}
}
