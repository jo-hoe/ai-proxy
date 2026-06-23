package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// tokenResponse is the OIDC token endpoint response shape.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// TokenResult holds the result of a successful token exchange.
type TokenResult struct {
	AccessToken  string
	RefreshToken string // empty when the server does not rotate the refresh token
	ExpiresIn    int
	ExpiresAt    time.Time
}

// OIDCClient performs refresh-token grants against any OIDC endpoint.
type OIDCClient struct {
	http *http.Client
}

// NewOIDCClient returns a reusable OIDC client.
func NewOIDCClient() *OIDCClient {
	return &OIDCClient{http: &http.Client{Timeout: 15 * time.Second}}
}

// Exchange performs a refresh-token grant against endpoint and returns new tokens.
// clientID identifies the OAuth 2.0 application to the OIDC server.
func (c *OIDCClient) Exchange(endpoint, clientID, refreshToken string) (*TokenResult, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}
	resp, err := c.http.Post(
		endpoint,
		"application/x-www-form-urlencoded",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("oidc exchange: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("oidc exchange: reading body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc exchange: HTTP %d: %s", resp.StatusCode, raw)
	}

	var tr tokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return nil, fmt.Errorf("oidc exchange: parse response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("oidc exchange: empty access_token in response")
	}

	expiresIn := tr.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	return &TokenResult{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresIn:    expiresIn,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}
