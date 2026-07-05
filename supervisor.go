package main

import (
	"context"
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

const rotationInterval = 50 * time.Minute
const secretPatchTimeout = 10 * time.Second

// secretPatcher is the subset of *SecretPatcher used by Supervisor. Enables
// nil safety (nil implementations swallow calls) and test injection.
type secretPatcher interface {
	PatchRefreshToken(ctx context.Context, refreshToken string) error
}

// ProxyStatus is the current state of the supervisor.
type ProxyStatus struct {
	Running         bool      `json:"running"`
	TokenExpiresAt  time.Time `json:"token_expires_at,omitempty"`
	LastRefreshedAt time.Time `json:"last_refreshed_at,omitempty"`
	LastRotatedAt   time.Time `json:"last_rotated_at,omitempty"`
	UptimeSeconds   float64   `json:"uptime_seconds"`
	RotationError   string    `json:"rotation_error,omitempty"`
}

// Supervisor manages OIDC token rotation and proxies requests to the upstream
// LLM API, injecting the current access token on every request.
type Supervisor struct {
	cfg          *Config
	oidc         *OIDCClient
	proxyPort    int
	upstream     *url.URL
	reverseProxy *httputil.ReverseProxy
	patcher      secretPatcher // nil when secret persistence is disabled

	mu           sync.RWMutex
	oidcEndpoint string
	clientID     string
	accessToken  string
	refreshToken string
	tokenResult  *TokenResult
	startedAt    time.Time
	stopCh       chan struct{}
	stopped      bool

	lastRotationErr string
	lastRotationAt  time.Time
	tokenStale      bool   // set when rotation gets invalid_grant; cleared by UpdateToken
	lastPersistedRT string // last refresh token successfully patched into the k8s Secret
}

// newSupervisor constructs a Supervisor. Call UpdateToken to activate.
func newSupervisor(cfg *Config, proxyPort string) (*Supervisor, error) {
	port, _ := strconv.Atoi(proxyPort)
	if port == 0 {
		port = cfg.Proxy.Port
	}

	if cfg.Proxy.UpstreamURL == "" {
		return nil, fmt.Errorf("proxy.upstream_url is required in config.yaml")
	}

	upstream, err := url.Parse(cfg.Proxy.UpstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy.upstream_url %q: %w", cfg.Proxy.UpstreamURL, err)
	}

	s := &Supervisor{
		cfg:       cfg,
		oidc:      NewOIDCClient(),
		proxyPort: port,
		upstream:  upstream,
		stopCh:    make(chan struct{}),
	}
	// Optional k8s Secret write-back. NewSecretPatcher returns (nil, nil)
	// when the feature is disabled or we're not running in a cluster.
	patcher, err := NewSecretPatcher()
	if err != nil {
		slog.Warn("supervisor: secret patcher init failed, persistence disabled", "err", err)
	} else if patcher != nil {
		s.patcher = patcher
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
	s.setToken(tr, endpoint, clientID, refreshToken)
	s.tokenStale = false
	s.lastRotationErr = ""
	first := s.startedAt.IsZero()
	if first {
		s.startedAt = time.Now()
	}
	// Snapshot the RT under the lock; patch outside the lock (network I/O).
	rtToPersist := s.refreshToken
	s.mu.Unlock()
	if first {
		go s.rotationLoop()
	}
	s.persistIfChanged(rtToPersist)
	slog.Info("supervisor: token updated", "expires_at", tr.ExpiresAt)
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
		Running: s.accessToken != "",
	}
	if s.tokenResult != nil {
		st.TokenExpiresAt = s.tokenResult.ExpiresAt
		st.LastRefreshedAt = s.tokenResult.ExpiresAt.Add(
			-time.Duration(s.tokenResult.ExpiresIn) * time.Second,
		)
	}
	if !s.startedAt.IsZero() {
		st.UptimeSeconds = time.Since(s.startedAt).Seconds()
	}
	st.RotationError = s.lastRotationErr
	st.LastRotatedAt = s.lastRotationAt
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
		s.refreshToken = refreshToken
	}
}

// rotationLoop proactively refreshes the token before it expires.
// When the refresh token becomes invalid (invalid_grant) it marks the token as
// stale and pauses rotation. Rotation resumes automatically once UpdateToken
// is called with a fresh token.
func (s *Supervisor) rotationLoop() {
	ticker := time.NewTicker(rotationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.RLock()
			stale := s.tokenStale
			s.mu.RUnlock()
			if stale {
				slog.Warn("supervisor: token is stale — skipping rotation until a new token is pushed via POST /token")
				continue
			}
			s.rotate()
		}
	}
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
	slog.Info("supervisor: refresh token persisted to k8s Secret")
}

// isInvalidGrant reports whether the error is an OIDC invalid_grant response.
func isInvalidGrant(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid_grant")
}
