package flexera

import (
	"fmt"
	"strings"
)

// ClientAuth describes everything needed to build an authenticated unified
// client: the zone (or explicit base URLs) plus one of a static access token,
// a shared OAuth token source, client-credentials, or a refresh token. It
// lets any consumer (CLI, Terraform provider, web app) construct a
// ready-to-use client without re-implementing the auth/retry wiring.
type ClientAuth struct {
	Zone         Zone
	HTTPClient   HttpRequestDoer
	APIBaseURL   string
	LoginBaseURL string

	AccessToken  string
	ClientID     string
	ClientSecret string
	RefreshToken string

	// SharedTokenSource, when non-nil, supplies the OAuth2 doer that
	// NewClientWithResponsesForAuth installs as the client's HTTP doer
	// (in lieu of constructing a fresh one). The (ClientID,
	// ClientSecret, RefreshToken) fields are still consulted by
	// Validate, but the caller is responsible for ensuring the source
	// was constructed with credentials equivalent to those fields.
	SharedTokenSource *SharedTokenSource
}

// hasAccessToken reports whether a static bearer token was supplied.
func (a ClientAuth) hasAccessToken() bool { return strings.TrimSpace(a.AccessToken) != "" }

// Validate ensures a usable authentication method was provided.
func (a ClientAuth) Validate() error {
	if a.hasAccessToken() {
		return nil
	}
	if a.SharedTokenSource != nil && a.SharedTokenSource.Doer() != nil {
		return nil
	}
	if strings.TrimSpace(a.RefreshToken) != "" {
		return nil
	}
	if strings.TrimSpace(a.ClientID) != "" && strings.TrimSpace(a.ClientSecret) != "" {
		return nil
	}
	if strings.TrimSpace(a.ClientID) != "" || strings.TrimSpace(a.ClientSecret) != "" {
		return fmt.Errorf("both client ID and client secret are required")
	}
	return fmt.Errorf("authentication is required: provide an access token, shared token source, client ID + secret, or a refresh token")
}

// NewClientWithResponsesForAuth builds an authenticated *ClientWithResponses
// from ClientAuth. It applies the default retrying HTTP client, then any
// extra options (e.g. WithBaseURL for a non-gateway host). Auth selection
// precedence:
//
//  1. Static AccessToken (highest — caller already has a bearer string)
//  2. SharedTokenSource (process-shared OAuth doer)
//  3. OAuth (RefreshToken, or ClientID + ClientSecret)
func NewClientWithResponsesForAuth(a ClientAuth, opts ...ClientOption) (*ClientWithResponses, error) {
	helper, err := NewAuthHelper(AuthHelperConfig{
		Zone:         a.Zone,
		HTTPClient:   a.HTTPClient,
		APIBaseURL:   a.APIBaseURL,
		LoginBaseURL: a.LoginBaseURL,
	})
	if err != nil {
		return nil, err
	}
	all := append([]ClientOption{WithRetryingHTTPClient(DefaultRetryConfig())}, opts...)
	if a.hasAccessToken() {
		return helper.NewStaticTokenClient(a.AccessToken, all...)
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if a.SharedTokenSource != nil && a.SharedTokenSource.Doer() != nil {
		// Install the shared OAuth doer FIRST (mirrors NewOAuthClient's
		// ordering) so retry/debug wrappers compose correctly.
		sharedOpts := append([]ClientOption{WithSharedTokenSource(a.SharedTokenSource)}, all...)
		return NewClientWithResponses(helper.APIBaseURL(), sharedOpts...)
	}
	return helper.NewOAuthClientWithResponses(OAuthConfig{
		ClientID:     a.ClientID,
		ClientSecret: a.ClientSecret,
		RefreshToken: a.RefreshToken,
	}, all...)
}
