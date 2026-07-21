package flexera

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Zone aliases the generated server variable enum for clearer auth helper APIs.
type Zone = ServerUrlFlexeraOneAPIZoneVariable

const (
	ZoneNAM  Zone = ServerUrlFlexeraOneAPIZoneVariableCom
	ZoneEU   Zone = ServerUrlFlexeraOneAPIZoneVariableEu
	ZoneAPAC Zone = ServerUrlFlexeraOneAPIZoneVariableAu
	// ZoneTest targets the flexeratest.com staging environment. The test
	// host (flexeratest.com) does not fit the production flexera.{zone}
	// template, so this zone bypasses the generated server URL constructor
	// in favor of literal API/login base URLs.
	ZoneTest    Zone = "test"
	DefaultZone Zone = ServerUrlFlexeraOneAPIZoneVariableDefault
)

// Literal hosts for the test/staging environment that does not fit the
// flexera.{zone} template.
const (
	testAPIBaseURL   = "https://api.flexeratest.com"
	testLoginBaseURL = "https://login.flexeratest.com"
)

// AuthHelperConfig configures the unified auth helper.
type AuthHelperConfig struct {
	Zone         Zone
	HTTPClient   HttpRequestDoer
	APIBaseURL   string
	LoginBaseURL string
}

// OAuthConfig configures OAuth-backed access token acquisition for the unified client.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
}

// AuthHelper provides ergonomic constructors and token helpers for the unified client.
type AuthHelper struct {
	apiBaseURL   string
	loginBaseURL string
	httpClient   HttpRequestDoer
}

// NewAuthHelper creates an auth helper for the given Flexera zone.
func NewAuthHelper(config AuthHelperConfig) (*AuthHelper, error) {
	zone := normalizeZone(config.Zone)
	if err := validateZone(zone); err != nil {
		return nil, err
	}

	apiBaseURL := strings.TrimSpace(config.APIBaseURL)
	if apiBaseURL == "" {
		resolvedAPIBaseURL, err := APIBaseURLForZone(zone)
		if err != nil {
			return nil, err
		}
		apiBaseURL = resolvedAPIBaseURL
	}

	loginBaseURL := strings.TrimSpace(config.LoginBaseURL)
	if loginBaseURL == "" {
		loginBaseURL = LoginBaseURLForZone(zone)
	}

	return &AuthHelper{
		apiBaseURL:   strings.TrimRight(apiBaseURL, "/"),
		loginBaseURL: strings.TrimRight(loginBaseURL, "/"),
		httpClient:   config.HTTPClient,
	}, nil
}

// APIBaseURL returns the resolved API base URL for the helper.
func (h *AuthHelper) APIBaseURL() string {
	return h.apiBaseURL
}

// LoginBaseURL returns the resolved login base URL for the helper.
func (h *AuthHelper) LoginBaseURL() string {
	return h.loginBaseURL
}

// APIBaseURLForZone returns the zoned API base URL used by the unified client.
func APIBaseURLForZone(zone Zone) (string, error) {
	zone = normalizeZone(zone)
	if err := validateZone(zone); err != nil {
		return "", err
	}
	if zone == ZoneTest {
		return testAPIBaseURL, nil
	}
	return NewServerUrlFlexeraOneAPI(zone)
}

// LoginBaseURLForZone returns the zoned login base URL used for token acquisition.
func LoginBaseURLForZone(zone Zone) string {
	zone = normalizeZone(zone)
	if zone == ZoneTest {
		return testLoginBaseURL
	}
	return fmt.Sprintf("https://login.flexera.%s", zone)
}

// NewServerUrlFlexeraLogin returns the zoned login server URL.
func NewServerUrlFlexeraLogin(zone Zone) string {
	return LoginBaseURLForZone(zone)
}

// BearerTokenRequestEditor adds a static bearer token to outgoing requests.
func BearerTokenRequestEditor(accessToken string) RequestEditorFn {
	token := strings.TrimSpace(accessToken)
	return func(ctx context.Context, req *http.Request) error {
		if token == "" {
			return errors.New("access token is required")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// WithBearerToken configures the unified client to use a static bearer token.
func WithBearerToken(accessToken string) ClientOption {
	return WithRequestEditorFn(BearerTokenRequestEditor(accessToken))
}

// WithSharedTokenSource returns a ClientOption that wires the unified
// client's HTTP doer to the OAuth2Doer underlying src.
//
// IMPORTANT: This option REPLACES c.Client (sets it to src.Doer()) and
// MUST be the FIRST doer-mutating option in the chain. Wrapper options
// such as WithRetryingHTTPClient and WithHTTPDebug compose by wrapping
// c.Client; they must come AFTER this option. Installing this option
// after a wrapper silently discards the wrapper.
// NewClientWithResponsesForAuth enforces the correct order; direct
// callers must mirror it.
//
// This is the SDK seam for sharing one OAuth token mint across
// multiple generated client packages (unified API + Optima) within one
// process, eliminating duplicate /oidc/token round trips and keeping
// refresh behaviour consistent.
func WithSharedTokenSource(src *SharedTokenSource) ClientOption {
	return func(c *Client) error {
		if src == nil || src.Doer() == nil {
			return errors.New("WithSharedTokenSource: nil source or empty doer")
		}
		c.Client = src.Doer()
		return nil
	}
}

// NewTokenClient builds an unauthenticated client pointed at the Flexera login host.
func (h *AuthHelper) NewTokenClient(opts ...ClientOption) (*ClientWithResponses, error) {
	return NewClientWithResponses(h.loginBaseURL, h.baseClientOptions(opts...)...)
}

// NewStaticTokenClient creates a unified API client that uses a static bearer token.
func (h *AuthHelper) NewStaticTokenClient(accessToken string, opts ...ClientOption) (*ClientWithResponses, error) {
	allOpts := append(h.baseClientOptions(opts...), WithBearerToken(accessToken))
	return NewClientWithResponses(h.apiBaseURL, allOpts...)
}

// NewOAuth2Doer constructs an OAuth-backed HTTP doer for the unified API client.
func (h *AuthHelper) NewOAuth2Doer(config OAuthConfig) (*OAuth2Doer, error) {
	if err := validateOAuthConfig(config); err != nil {
		return nil, err
	}

	underlying := h.httpClient
	if underlying == nil {
		underlying = &http.Client{}
	}

	tokenURL, err := url.JoinPath(h.loginBaseURL, "/oidc/token")
	if err != nil {
		return nil, fmt.Errorf("build token URL: %w", err)
	}

	return NewOAuth2Doer(
		strings.TrimSpace(config.ClientID),
		strings.TrimSpace(config.ClientSecret),
		strings.TrimSpace(config.RefreshToken),
		tokenURL,
		underlying,
	), nil
}

// ValidateOAuth2Credentials verifies that the OAuth configuration can obtain a token.
func (h *AuthHelper) ValidateOAuth2Credentials(config OAuthConfig) error {
	doer, err := h.NewOAuth2Doer(config)
	if err != nil {
		return err
	}
	return doer.ValidateCredentials()
}

// NewOAuthClient creates a unified API client with automatic token
// acquisition/refresh.
//
// Option ordering: the OAuth doer is installed via WithHTTPClient as the
// FIRST option, so caller-supplied wrapper options (e.g.
// WithRetryingHTTPClient, WithHTTPDebug) wrap the OAuth doer rather than
// being overwritten by it. This composes with
// NewClientWithResponsesForAuth which prepends WithRetryingHTTPClient to
// caller opts — final chain is
// [WithHTTPClient(oauthDoer), WithRetryingHTTPClient(...), caller-opts...].
func (h *AuthHelper) NewOAuthClient(config OAuthConfig, opts ...ClientOption) (*Client, error) {
	doer, err := h.NewOAuth2Doer(config)
	if err != nil {
		return nil, err
	}

	allOpts := append([]ClientOption{WithHTTPClient(doer)}, opts...)
	return NewClient(h.apiBaseURL, allOpts...)
}

// NewOAuthClientWithResponses creates a response-aware unified API client
// with automatic token acquisition/refresh. See NewOAuthClient for
// option-ordering semantics.
func (h *AuthHelper) NewOAuthClientWithResponses(config OAuthConfig, opts ...ClientOption) (*ClientWithResponses, error) {
	doer, err := h.NewOAuth2Doer(config)
	if err != nil {
		return nil, err
	}

	allOpts := append([]ClientOption{WithHTTPClient(doer)}, opts...)
	return NewClientWithResponses(h.apiBaseURL, allOpts...)
}

// NewProjectResolverWithOAuth constructs a ProjectResolver backed by an
// OAuth-authenticated unified Client.
func (h *AuthHelper) NewProjectResolverWithOAuth(config OAuthConfig, opts ...ProjectResolverOption) (*ProjectResolver, error) {
	client, err := h.NewOAuthClientWithResponses(config)
	if err != nil {
		return nil, err
	}
	return NewProjectResolver(client, opts...)
}

// NewProjectResolverWithStaticToken constructs a ProjectResolver backed by a
// unified Client that uses a static bearer token.
func (h *AuthHelper) NewProjectResolverWithStaticToken(accessToken string, opts ...ProjectResolverOption) (*ProjectResolver, error) {
	client, err := h.NewStaticTokenClient(accessToken)
	if err != nil {
		return nil, err
	}
	return NewProjectResolver(client, opts...)
}

// TokenWithClientCredentials calls POST /oidc/token using the client_credentials grant.
func (h *AuthHelper) TokenWithClientCredentials(ctx context.Context, clientID, clientSecret string, reqEditors ...RequestEditorFn) (*AuthTokenResponseBody, error) {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("client_id and client_secret are required")
	}

	client, err := h.NewTokenClient()
	if err != nil {
		return nil, err
	}

	body := AuthTokenTokenFormdataRequestBody{
		GrantType:    AuthTokenRequestBodyGrantTypeClientCredentials,
		ClientId:     &clientID,
		ClientSecret: &clientSecret,
	}

	response, err := client.AuthTokenTokenWithFormdataBodyWithResponse(ctx, body, reqEditors...)
	if err != nil {
		return nil, err
	}

	return unwrapTokenResponse(response)
}

// TokenWithRefreshToken calls POST /oidc/token using the refresh_token grant.
func (h *AuthHelper) TokenWithRefreshToken(ctx context.Context, refreshToken string, reqEditors ...RequestEditorFn) (*AuthTokenResponseBody, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errors.New("refresh_token is required")
	}

	client, err := h.NewTokenClient()
	if err != nil {
		return nil, err
	}

	body := AuthTokenTokenFormdataRequestBody{
		GrantType:    AuthTokenRequestBodyGrantTypeRefreshToken,
		RefreshToken: &refreshToken,
	}

	response, err := client.AuthTokenTokenWithFormdataBodyWithResponse(ctx, body, reqEditors...)
	if err != nil {
		return nil, err
	}

	return unwrapTokenResponse(response)
}

func (h *AuthHelper) baseClientOptions(opts ...ClientOption) []ClientOption {
	if h.httpClient == nil {
		return append([]ClientOption{}, opts...)
	}

	base := []ClientOption{WithHTTPClient(h.httpClient)}
	return append(base, opts...)
}

func unwrapTokenResponse(response *AuthTokenTokenResponse) (*AuthTokenResponseBody, error) {
	if response == nil {
		return nil, errors.New("token response is nil")
	}
	if response.JSON200 != nil {
		return response.JSON200, nil
	}
	return nil, fmt.Errorf("token request failed with status %d: %s", response.StatusCode(), strings.TrimSpace(string(response.Body)))
}

func normalizeZone(zone Zone) Zone {
	if zone == "" {
		return DefaultZone
	}
	return zone
}

// validateZone delegates to IsCanonicalZone (zones.go) so the accept set
// is defined in exactly one place.
func validateZone(zone Zone) error {
	if IsCanonicalZone(zone) {
		return nil
	}
	return fmt.Errorf("unsupported zone %q", zone)
}

func validateOAuthConfig(config OAuthConfig) error {
	if strings.TrimSpace(config.RefreshToken) != "" {
		return nil
	}
	if strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.ClientSecret) == "" {
		return errors.New("either refresh_token or both client_id and client_secret are required")
	}
	return nil
}
