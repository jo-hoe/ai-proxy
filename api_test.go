package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockSupervisor implements supervisorIface for testing.
type mockSupervisor struct {
	updateErr  error
	statusResp ProxyStatus
}

func (m *mockSupervisor) UpdateToken(_, _, _ string) error { return m.updateErr }
func (m *mockSupervisor) Status() ProxyStatus              { return m.statusResp }

func TestAPI_PostToken_Success(t *testing.T) {
	api := newAPI(&mockSupervisor{})

	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("endpoint=https://oidc.example.com/token&client_id=my-client&token=my-refresh-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAPI_PostToken_MissingToken(t *testing.T) {
	api := newAPI(&mockSupervisor{})

	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("endpoint=https://oidc.example.com/token&client_id=my-client"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAPI_PostToken_MissingClientID(t *testing.T) {
	api := newAPI(&mockSupervisor{})

	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("endpoint=https://oidc.example.com/token&token=my-refresh-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAPI_PostToken_MissingEndpoint(t *testing.T) {
	api := newAPI(&mockSupervisor{})

	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("client_id=my-client&token=my-refresh-token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAPI_PostToken_InvalidToken(t *testing.T) {
	api := newAPI(&mockSupervisor{updateErr: errors.New("oidc exchange: HTTP 401")})

	req := httptest.NewRequest(http.MethodPost, "/token",
		strings.NewReader("endpoint=https://oidc.example.com/token&client_id=my-client&token=bad"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
}

func TestAPI_GetStatus_Running(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	api := newAPI(&mockSupervisor{
		statusResp: ProxyStatus{
			Running:         true,
			TokenExpiresAt:  now.Add(time.Hour),
			LastRefreshedAt: now,
			UptimeSeconds:   42.5,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var body ProxyStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Running {
		t.Error("running should be true")
	}
	if body.UptimeSeconds != 42.5 {
		t.Errorf("uptime = %f, want 42.5", body.UptimeSeconds)
	}
}

func TestAPI_GetStatus_NotRunning(t *testing.T) {
	api := newAPI(&mockSupervisor{statusResp: ProxyStatus{Running: false}})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var body ProxyStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Running {
		t.Error("running should be false")
	}
}
