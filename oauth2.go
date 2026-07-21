package flexera

// Generic OAuth2 HTTP Doer
// - Supports client_credentials and refresh_token grant types
// - Supports token caching and automatic refresh
// - Uses standard library only
// - Aligns with Flexera One Authorization (https://developer.flexera.com/docs/page/authorization)

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenResponse represents the response from the OAuth2 token endpoint
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// OAuth2Doer implements HttpRequestDoer with OAuth2 authentication.
//
// Concurrency: mu serializes accessToken/expiry mutation across Do,
// Refresh, and Invalidate. The unified-API Do path and the
// SharedTokenSource (Optima) Token/Invalidate path can both reach the
// same doer concurrently; mu prevents torn reads on the Authorization
// header.
type OAuth2Doer struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	TokenURL     string

	mu          sync.Mutex
	accessToken string
	expiry      time.Time

	underlying HttpRequestDoer
}

// NewOAuth2Doer creates a new OAuth2Doer.
// If refreshToken is provided, it uses refresh_token grant type.
// Otherwise, it uses client_credentials grant type.
func NewOAuth2Doer(clientID, clientSecret, refreshToken, tokenURL string, underlying HttpRequestDoer) *OAuth2Doer {
	if underlying == nil {
		underlying = &http.Client{}
	}
	return &OAuth2Doer{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
		TokenURL:     tokenURL,
		underlying:   underlying,
	}
}

// Do performs the HTTP request with OAuth2 authentication.
func (o *OAuth2Doer) Do(req *http.Request) (*http.Response, error) {
	o.mu.Lock()
	if o.accessToken == "" || time.Now().After(o.expiry) {
		if err := o.refreshTokenLocked(); err != nil {
			o.mu.Unlock()
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
	}
	token := o.accessToken
	o.mu.Unlock()

	// Clone the request and add Authorization header
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token)

	// Perform the request
	return o.underlying.Do(req)
}

// Refresh forces a /oidc/token round trip using the configured grant
// (refresh_token if RefreshToken is set, otherwise client_credentials).
// ctx is accepted for forward compatibility; the underlying refresh uses
// context.Background today (a future revision can plumb ctx through
// without breaking this signature).
//
// Refresh is the public entry point that SharedTokenSource.Token uses;
// the mutex prevents racing with concurrent Do or Invalidate.
func (o *OAuth2Doer) Refresh(ctx context.Context) error {
	_ = ctx // reserved for future cancellation plumbing
	o.mu.Lock()
	defer o.mu.Unlock()
	o.accessToken = ""
	o.expiry = time.Time{}
	return o.refreshTokenLocked()
}

// Invalidate clears the cached access token so the next Do (or Refresh)
// call performs a fresh /oidc/token round-trip. Intended for callers
// that observe a server-side revocation (e.g. a 401 after a recent
// successful refresh) and want to force re-auth without waiting for
// natural expiry. Mutex-serialized with Do and Refresh.
func (o *OAuth2Doer) Invalidate() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.accessToken = ""
	o.expiry = time.Time{}
}

// refreshTokenLocked obtains a new access token. Caller MUST hold o.mu.
func (o *OAuth2Doer) refreshTokenLocked() error {
	data := url.Values{}

	if o.RefreshToken != "" {
		// Refresh token flow - use only the refresh token
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", o.RefreshToken)
	} else if o.ClientID != "" && o.ClientSecret != "" {
		// Client credentials flow - use client ID and secret
		data.Set("grant_type", "client_credentials")
		data.Set("client_id", o.ClientID)
		data.Set("client_secret", o.ClientSecret)
	} else {
		return fmt.Errorf("either refresh_token or both client_id and client_secret must be provided")
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", o.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.underlying.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	o.accessToken = tokenResp.AccessToken
	o.expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Update refresh token if provided
	if tokenResp.RefreshToken != "" {
		o.RefreshToken = tokenResp.RefreshToken
	}

	return nil
}

// ValidateCredentials tests the OAuth2 credentials by attempting to obtain an access token.
// Retained as a thin Background-context wrapper around Refresh for callers that pre-date the
// ctx-aware API.
func (o *OAuth2Doer) ValidateCredentials() error {
	if err := o.Refresh(context.Background()); err != nil {
		return fmt.Errorf("credential validation failed: %w", err)
	}
	return nil
}

// AccessToken returns the current access token.
func (o *OAuth2Doer) AccessToken() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.accessToken
}

// TokenExpiry returns the token expiry time.
func (o *OAuth2Doer) TokenExpiry() time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.expiry
}
