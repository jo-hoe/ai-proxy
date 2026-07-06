package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const secretPatchTimeout = 10 * time.Second

// secretPatcher is the subset of *SecretPatcher used by Supervisor. Enables
// nil safety (nil implementations swallow calls) and test injection.
type secretPatcher interface {
	PatchRefreshToken(ctx context.Context, refreshToken string) error
}

// ProxyStatus is the current state of the supervisor.
type ProxyStatus struct {
	Ready          bool      `json:"ready"`
	TokenExpiresAt time.Time `json:"token_expires_at,omitzero"`
	LastRefreshedAt time.Time `json:"last_refreshed_at,omitzero"`
	LastRotatedAt  time.Time `json:"last_rotated_at,omitzero"`
	NextRotationAt time.Time `json:"next_rotation_at,omitzero"`
	RotationError  string    `json:"rotation_error,omitempty"`
}

// Supervisor manages OIDC token rotation and proxies requests to the upstream
// LLM API, injecting the current access token on every request.
type Supervisor struct {
	cfg            *Config
	oidc           *OIDCClient
	proxyPort      int
	upstream       *url.URL
	reverseProxy   *httputil.ReverseProxy
	patcher        secretPatcher // nil when secret persistence is disabled
	persistSecret  string        // name of the k8s Secret being patched, for logging
	rotationMargin time.Duration

	mu           sync.RWMutex
	oidcEndpoint string
	clientID     string
	accessToken  string
	refreshToken string
	tokenResult  *TokenResult
	startedAt    time.Time
	stopCh       chan struct{}
	resetCh      chan time.Time // signals rotationLoop to reschedule with a new expiry
	stopped      bool

	lastRotationErr string
	lastRotationAt  time.Time
	nextRotationAt  time.Time // when the next proactive rotation is scheduled
	tokenStale      bool   // set when rotation gets invalid_grant; cleared by UpdateToken
	lastPersistedRT string // last refresh token successfully patched into the k8s Secret
}

// newSupervisor constructs a Supervisor. Call UpdateToken to activate.
func newSupervisor(cfg *Config, proxyPort string) (*Supervisor, error) {
	port, err := strconv.Atoi(proxyPort)
	if err != nil || port == 0 {
		if err != nil {
			slog.Warn("supervisor: invalid proxy port string, falling back to config value", "raw", proxyPort, "err", err)
		}
		port = cfg.Proxy.Port
	}

	if cfg.Proxy.UpstreamURL == "" {
		return nil, errors.New("proxy.upstream_url is required in config.yaml")
	}

	upstream, err := url.Parse(cfg.Proxy.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy.upstream_url %q: %w", cfg.Proxy.UpstreamURL, err)
	}

	s := &Supervisor{
		cfg:            cfg,
		oidc:           NewOIDCClient(),
		proxyPort:      port,
		upstream:       upstream,
		rotationMargin: cfg.Proxy.RotationMargin,
		stopCh:         make(chan struct{}),
		resetCh:        make(chan time.Time, 1),
	}
	// Optional k8s Secret write-back. NewSecretPatcher returns (nil, nil)
	// when the feature is disabled or we're not running in a cluster.
	patcher, err := NewSecretPatcher()
	if err != nil {
		slog.Warn("supervisor: secret patcher init failed, persistence disabled", "err", err)
	} else if patcher != nil {
		s.patcher = patcher
		s.persistSecret = patcher.secret
		slog.Info("supervisor: k8s Secret persistence enabled", "secret", patcher.secret, "namespace", patcher.namespace)
	}
	s.reverseProxy = s.buildReverseProxy()
	return s, nil
}

// buildReverseProxy creates a reverse proxy that rewrites the host and injects
// the current access token on every request.
func (s *Supervisor) buildReverseProxy() *httputil.ReverseProxy {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(s.upstream)
			pr.Out.Host = s.upstream.Host
			s.mu.RLock()
			token := s.accessToken
			s.mu.RUnlock()
			pr.Out.Header.Set("Authorization", "Bearer "+token)
			slog.Debug("proxy request",
				"method", pr.In.Method,
				"path", pr.In.URL.Path,
				"upstream", pr.Out.URL.String(),
			)
		},
	}
	rp.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode == http.StatusUnauthorized {
			slog.Warn("upstream returned 401 — token may be expired or invalid",
				"method", resp.Request.Method,
				"path", resp.Request.URL.Path,
			)
		}
		slog.Debug("proxy response",
			"method", resp.Request.Method,
			"path", resp.Request.URL.Path,
			"status", resp.StatusCode,
		)
		return nil
	}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy error", "method", r.Method, "path", r.URL.Path, "err", err)
		http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
	}
	return rp
}

// ServeHTTP implements http.Handler — the supervisor is the proxy.
func (s *Supervisor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	running := s.accessToken != ""
	s.mu.RUnlock()
	if !running {
		slog.Warn("proxy not ready: no token", "method", r.Method, "path", r.URL.Path)
		http.Error(w, `{"error":"proxy not ready: no token"}`, http.StatusServiceUnavailable)
		return
	}
	s.reverseProxy.ServeHTTP(w, r)
}

// UpdateToken validates a new refresh token and hot-swaps the access token
// with zero downtime. Also starts the rotation loop on first call.
func (s *Supervisor) UpdateToken(endpoint, clientID, refreshToken string) error {
	tr, err := s.oidc.Exchange(endpoint, clientID, refreshToken)
	if err != nil {
		return fmt.Errorf("token update: %w", err)
	}
	s.mu.Lock()
	wasStale := s.tokenStale
	s.setToken(tr, endpoint, clientID, refreshToken)
	s.tokenStale = false
	s.lastRotationErr = ""
	first := s.startedAt.IsZero()
	if first {
		s.startedAt = time.Now()
	}
	nextAt := time.Now().Add(s.nextRotation(tr.ExpiresAt))
	s.nextRotationAt = nextAt
	rtToPersist := s.refreshToken
	s.mu.Unlock()

	if wasStale {
		slog.Info("supervisor: recovered from stale token state")
	}
	if first {
		go s.rotationLoop(tr.ExpiresAt)
	} else {
		// Drain any pending reset then send the latest expiry. The channel is
		// buffered(1) so the loop always sees the most recent value.
		select {
		case <-s.resetCh:
		default:
		}
		s.resetCh <- tr.ExpiresAt
	}
	s.persistIfChanged(rtToPersist)
	slog.Info("supervisor: token updated", "expires_at", tr.ExpiresAt, "next_rotation_at", nextAt)
	return nil
}

// Healthy reports whether the proxy is ready to serve requests.
// Returns false when no token has been loaded or the refresh token is stale.
func (s *Supervisor) Healthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accessToken != "" && !s.tokenStale
}

// Status returns a snapshot of supervisor state.
func (s *Supervisor) Status() ProxyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := ProxyStatus{
		Ready: s.accessToken != "" && !s.tokenStale,
	}
	if s.tokenResult != nil {
		st.TokenExpiresAt = s.tokenResult.ExpiresAt
		st.LastRefreshedAt = s.tokenResult.ExpiresAt.Add(
			-time.Duration(s.tokenResult.ExpiresIn) * time.Second,
		)
	}
	st.RotationError = s.lastRotationErr
	st.LastRotatedAt = s.lastRotationAt
	st.NextRotationAt = s.nextRotationAt
	return st
}

// stop shuts down the rotation loop.
func (s *Supervisor) stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()
	close(s.stopCh)
}

// setToken atomically updates the OIDC credentials and access token.
// Must be called with s.mu held for writing.
func (s *Supervisor) setToken(tr *TokenResult, endpoint, clientID, refreshToken string) {
	s.accessToken = tr.AccessToken
	s.oidcEndpoint = endpoint
	s.clientID = clientID
	s.tokenResult = tr
	if tr.RefreshToken != "" {
		s.refreshToken = tr.RefreshToken
	} else {
		slog.Debug("supervisor: server did not rotate refresh token; reusing existing")
		s.refreshToken = refreshToken
	}
}

// rotationLoop proactively refreshes the token before it expires.
// It fires rotationMargin before the token's expiry, so a short-lived token
// received via POST /token is rotated promptly rather than after a fixed 50m.
// When the refresh token becomes invalid (invalid_grant) it marks the token as
// stale and pauses rotation. Rotation resumes automatically once UpdateToken
// is called with a fresh token.
func (s *Supervisor) rotationLoop(firstExpiry time.Time) {
	timer := time.NewTimer(s.nextRotation(firstExpiry))
	defer timer.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case expiresAt := <-s.resetCh:
			// A new token was pushed via POST /token mid-loop; reschedule.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			d := s.nextRotation(expiresAt)
			nextAt := time.Now().Add(d)
			s.mu.Lock()
			s.nextRotationAt = nextAt
			s.mu.Unlock()
			timer.Reset(d)
			slog.Info("supervisor: rotation rescheduled", "next_rotation_at", nextAt)
		case <-timer.C:
			s.mu.RLock()
			stale := s.tokenStale
			s.mu.RUnlock()
			if stale {
				slog.Warn("supervisor: token is stale — skipping rotation until a new token is pushed via POST /token")
				// Retry in one margin interval rather than spinning.
				timer.Reset(s.rotationMargin)
				continue
			}
			slog.Debug("supervisor: starting scheduled token rotation")
			s.rotate()
			s.mu.RLock()
			expiresAt := s.tokenResult.ExpiresAt
			s.mu.RUnlock()
			d := s.nextRotation(expiresAt)
			nextAt := time.Now().Add(d)
			s.mu.Lock()
			s.nextRotationAt = nextAt
			s.mu.Unlock()
			timer.Reset(d)
			slog.Info("supervisor: next rotation scheduled", "next_rotation_at", nextAt)
		}
	}
}

// nextRotation returns how long to wait before the next rotation attempt.
// It targets s.rotationMargin before expiry, with a minimum of zero.
func (s *Supervisor) nextRotation(expiresAt time.Time) time.Duration {
	d := time.Until(expiresAt) - s.rotationMargin
	if d < 0 {
		return 0
	}
	return d
}

func (s *Supervisor) rotate() {
	s.mu.RLock()
	ep := s.oidcEndpoint
	cid := s.clientID
	rt := s.refreshToken
	s.mu.RUnlock()

	tr, err := s.oidc.Exchange(ep, cid, rt)
	if err != nil {
		slog.Error("supervisor: token rotation failed", "err", err)
		s.mu.Lock()
		s.lastRotationErr = err.Error()
		if isInvalidGrant(err) {
			s.tokenStale = true
			slog.Warn("supervisor: refresh token is invalid or expired — proxy will continue serving with the current access token until a new refresh token is pushed via POST /token")
		}
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.setToken(tr, ep, cid, rt)
	s.lastRotationErr = ""
	s.lastRotationAt = time.Now()
	rtToPersist := s.refreshToken
	s.mu.Unlock()
	s.persistIfChanged(rtToPersist)
	slog.Info("supervisor: token rotated", "expires_at", tr.ExpiresAt)
}

// persistIfChanged patches the k8s Secret when the refresh token differs
// from the last successfully persisted value. Best-effort: patch failures
// are logged but don't affect the in-memory token or the rotation loop.
func (s *Supervisor) persistIfChanged(refreshToken string) {
	if s.patcher == nil || refreshToken == "" {
		return
	}
	s.mu.RLock()
	unchanged := refreshToken == s.lastPersistedRT
	s.mu.RUnlock()
	if unchanged {
		slog.Debug("supervisor: refresh token unchanged, skipping persist")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), secretPatchTimeout)
	defer cancel()
	if err := s.patcher.PatchRefreshToken(ctx, refreshToken); err != nil {
		slog.Warn("supervisor: failed to persist refresh token to k8s Secret", "err", err)
		return
	}
	s.mu.Lock()
	s.lastPersistedRT = refreshToken
	s.mu.Unlock()
	slog.Info("supervisor: refresh token persisted to k8s Secret", "secret", s.persistSecret)
}

// isInvalidGrant reports whether the error is an OIDC invalid_grant response.
func isInvalidGrant(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid_grant")
}
