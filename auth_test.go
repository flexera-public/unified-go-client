package flexera

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewAuthHelper_DefaultURLs(t *testing.T) {
	helper, err := NewAuthHelper(AuthHelperConfig{Zone: ZoneEU})
	if err != nil {
		t.Fatal(err)
	}

	if got := helper.APIBaseURL(); got != "https://api.flexera.eu" {
		t.Fatalf("unexpected API base URL: %s", got)
	}
	if got := helper.LoginBaseURL(); got != "https://login.flexera.eu" {
		t.Fatalf("unexpected login base URL: %s", got)
	}
}

func TestBearerTokenRequestEditor(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	editor := BearerTokenRequestEditor("test-token")
	if err := editor(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("unexpected Authorization header: %s", got)
	}
}

func TestTokenWithClientCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oidc/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/x-www-form-urlencoded") {
			t.Fatalf("unexpected content type: %s", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "client_credentials" {
			t.Fatalf("unexpected grant_type: %s", got)
		}
		if got := r.Form.Get("client_id"); got != "abc" {
			t.Fatalf("unexpected client_id: %s", got)
		}
		if got := r.Form.Get("client_secret"); got != "xyz" {
			t.Fatalf("unexpected client_secret: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-123","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()

	helper, err := NewAuthHelper(AuthHelperConfig{LoginBaseURL: server.URL, APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := helper.TokenWithClientCredentials(context.Background(), "abc", "xyz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "token-123" {
		t.Fatalf("unexpected access token: %s", resp.AccessToken)
	}
}

func TestTokenWithRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("unexpected grant_type: %s", got)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-123" {
			t.Fatalf("unexpected refresh_token: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-456","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-789"}`)
	}))
	defer server.Close()

	helper, err := NewAuthHelper(AuthHelperConfig{LoginBaseURL: server.URL, APIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := helper.TokenWithRefreshToken(context.Background(), "refresh-123")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "token-456" {
		t.Fatalf("unexpected access token: %s", resp.AccessToken)
	}
}

func TestNewOAuth2DoerUsesLoginTokenURL(t *testing.T) {
	helper, err := NewAuthHelper(AuthHelperConfig{Zone: ZoneAPAC})
	if err != nil {
		t.Fatal(err)
	}

	doer, err := helper.NewOAuth2Doer(OAuthConfig{ClientID: "abc", ClientSecret: "xyz"})
	if err != nil {
		t.Fatal(err)
	}

	if got := doer.TokenURL; got != "https://login.flexera.au/oidc/token" {
		t.Fatalf("unexpected token URL: %s", got)
	}
}

func TestNewOAuthClientWithResponsesUsesOAuthDoer(t *testing.T) {
	helper, err := NewAuthHelper(AuthHelperConfig{Zone: ZoneNAM})
	if err != nil {
		t.Fatal(err)
	}

	client, err := helper.NewOAuthClientWithResponses(OAuthConfig{ClientID: "abc", ClientSecret: "xyz"})
	if err != nil {
		t.Fatal(err)
	}

	baseClient, ok := client.ClientInterface.(*Client)
	if !ok {
		t.Fatalf("unexpected client type: %T", client.ClientInterface)
	}
	if _, ok := baseClient.Client.(*OAuth2Doer); !ok {
		t.Fatalf("expected OAuth2 doer, got %T", baseClient.Client)
	}
	if got := strings.TrimRight(baseClient.Server, "/"); got != "https://api.flexera.com" {
		t.Fatalf("unexpected server: %s", got)
	}
}

func TestValidateOAuthConfigRejectsIncompleteClientCredentials(t *testing.T) {
	err := validateOAuthConfig(OAuthConfig{ClientID: "abc"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoginBaseURLForZone(t *testing.T) {
	got := LoginBaseURLForZone(ZoneNAM)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "login.flexera.com" {
		t.Fatalf("unexpected host: %s", parsed.Host)
	}
}

func TestZoneTestURLs(t *testing.T) {
	if got := LoginBaseURLForZone(ZoneTest); got != "https://login.flexeratest.com" {
		t.Fatalf("unexpected test login URL: %s", got)
	}
	got, err := APIBaseURLForZone(ZoneTest)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.flexeratest.com" {
		t.Fatalf("unexpected test API URL: %s", got)
	}

	helper, err := NewAuthHelper(AuthHelperConfig{Zone: ZoneTest})
	if err != nil {
		t.Fatal(err)
	}
	if helper.APIBaseURL() != "https://api.flexeratest.com" {
		t.Fatalf("helper APIBaseURL wrong: %s", helper.APIBaseURL())
	}
	if helper.LoginBaseURL() != "https://login.flexeratest.com" {
		t.Fatalf("helper LoginBaseURL wrong: %s", helper.LoginBaseURL())
	}
}

func TestNewClientWithResponsesForAuth_RetryWrapsOAuthDoer(t *testing.T) {
	cli, err := NewClientWithResponsesForAuth(ClientAuth{
		Zone:         ZoneNAM,
		ClientID:     "abc",
		ClientSecret: "xyz",
	})
	if err != nil {
		t.Fatal(err)
	}
	base, ok := cli.ClientInterface.(*Client)
	if !ok {
		t.Fatalf("unexpected client type: %T", cli.ClientInterface)
	}
	rd, ok := base.Client.(*retryingDoer)
	if !ok {
		t.Fatalf("expected *retryingDoer wrapping OAuth doer, got %T", base.Client)
	}
	if _, ok := rd.next.(*OAuth2Doer); !ok {
		t.Fatalf("expected retryingDoer.next = *OAuth2Doer, got %T", rd.next)
	}
}

func TestNewClientWithResponsesForAuth_UsesSharedTokenSource(t *testing.T) {
	helper, err := NewAuthHelper(AuthHelperConfig{Zone: ZoneNAM})
	if err != nil {
		t.Fatal(err)
	}
	doer, err := helper.NewOAuth2Doer(OAuthConfig{ClientID: "abc", ClientSecret: "xyz"})
	if err != nil {
		t.Fatal(err)
	}
	src := NewSharedTokenSource(doer)
	cli, err := NewClientWithResponsesForAuth(ClientAuth{
		Zone:              ZoneNAM,
		SharedTokenSource: src,
	})
	if err != nil {
		t.Fatal(err)
	}
	base, ok := cli.ClientInterface.(*Client)
	if !ok {
		t.Fatalf("unexpected client type: %T", cli.ClientInterface)
	}
	rd, ok := base.Client.(*retryingDoer)
	if !ok {
		t.Fatalf("expected *retryingDoer, got %T", base.Client)
	}
	// Pointer-equal: the same OAuth doer instance is shared with the
	// caller's source; no fresh mint occurred.
	if rd.next != src.Doer() {
		t.Fatalf("expected retryingDoer.next to be the SHARED OAuth doer; got a different instance")
	}
}
