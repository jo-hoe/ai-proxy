package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// supervisorIface is the subset of Supervisor used by the API.
// Defining it here enables test mocking without a framework.
type supervisorIface interface {
	UpdateToken(endpoint, clientID, refreshToken string) error
	Status() ProxyStatus
}

// API is the HTTP management server.
type API struct {
	mux *http.ServeMux
	sup supervisorIface
}

// newAPI constructs an API wired to the given supervisor.
func newAPI(sup supervisorIface) *API {
	a := &API{mux: http.NewServeMux(), sup: sup}
	a.mux.HandleFunc("POST /token", a.handlePostToken)
	a.mux.HandleFunc("GET /status", a.handleGetStatus)
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

// handlePostToken accepts a refresh token, client ID, and OIDC endpoint via
// form fields, validates via OIDC exchange, and hot-swaps the access token.
func (a *API) handlePostToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "cannot parse form")
		return
	}
	endpoint := r.FormValue("endpoint")
	if endpoint == "" {
		writeError(w, http.StatusBadRequest, "form field 'endpoint' is required")
		return
	}
	clientID := r.FormValue("client_id")
	if clientID == "" {
		writeError(w, http.StatusBadRequest, "form field 'client_id' is required")
		return
	}
	token := r.FormValue("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "form field 'token' is required")
		return
	}
	if err := a.sup.UpdateToken(endpoint, clientID, token); err != nil {
		log.Printf("POST /token: %v", err)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGetStatus returns the current proxy status.
func (a *API) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.sup.Status())
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
