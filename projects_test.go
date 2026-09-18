package flexera

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestProjectResolver(t *testing.T, handler http.HandlerFunc, clientOpts ...ClientOption) (*ProjectResolver, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	allOpts := append([]ClientOption{
		WithHTTPClient(server.Client()),
		WithBearerToken(testUserJWT(42)),
	}, clientOpts...)
	client, err := NewClientWithResponses(server.URL, allOpts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resolver, err := NewProjectResolver(client)
	if err != nil {
		t.Fatalf("NewProjectResolver: %v", err)
	}
	return resolver, server
}

func testUserJWT(userID int) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"sub":"u-%d"}`, userID)))
	return header + "." + payload + ".sig"
}

func TestListProjects_Success(t *testing.T) {
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/grs/users/42/projects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Version"); got != "2.0" {
			t.Fatalf("expected default X-Api-Version 2.0, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]`)
	})

	projects, err := resolver.ListProjects(context.Background(), 42)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 || projects[0].ID != 1 || projects[1].Name != "beta" {
		t.Fatalf("unexpected projects: %+v", projects)
	}
}

func TestListProjects_AppliesClientEditors(t *testing.T) {
	var gotAuth string
	token := testUserJWT(42)
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `[]`)
	}, WithBearerToken(token))

	_, err := resolver.ListProjects(context.Background(), 1)
	if err != nil && !errors.Is(err, nil) {
		t.Fatalf("ListProjects: %v", err)
	}
	if gotAuth != "Bearer "+token {
		t.Fatalf("expected bearer header from client editor, got %q", gotAuth)
	}
}

func TestListProjects_PerCallEditorCanOverrideAPIVersion(t *testing.T) {
	var gotVersion string
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("X-Api-Version")
		_, _ = io.WriteString(w, `[]`)
	})

	editor := func(_ context.Context, req *http.Request) error {
		req.Header.Set("X-Api-Version", "3.0")
		return nil
	}
	if _, err := resolver.ListProjects(context.Background(), 1, editor); err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if gotVersion != "3.0" {
		t.Fatalf("expected per-call editor to set X-Api-Version, got %q", gotVersion)
	}
}

func TestListProjects_UsesConfiguredTokenSource(t *testing.T) {
	var gotAuth string
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `[]`)
	})
	resolver, err := NewProjectResolver(resolver.client, WithProjectResolverTokenSource(func(context.Context) (string, error) {
		return testUserJWT(42), nil
	}))
	if err != nil {
		t.Fatalf("NewProjectResolver: %v", err)
	}

	if _, err := resolver.ListProjects(context.Background(), 1); err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if gotAuth != "Bearer "+testUserJWT(42) {
		t.Fatalf("expected bearer header from configured token source, got %q", gotAuth)
	}
}

func TestListProjects_RejectsEmptyConfiguredToken(t *testing.T) {
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	resolver, err := NewProjectResolver(resolver.client, WithProjectResolverTokenSource(func(context.Context) (string, error) {
		return "", nil
	}))
	if err != nil {
		t.Fatalf("NewProjectResolver: %v", err)
	}

	_, err = resolver.ListProjects(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "empty access token") {
		t.Fatalf("expected empty token error, got %v", err)
	}
}

func TestListProjects_NonOKError(t *testing.T) {
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `forbidden`)
	})

	_, err := resolver.ListProjects(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestResolveSingleProjectID(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantID     int64
		wantErr    error
		wantErrAs  bool
		ambiguous  bool
		expectFail bool
	}{
		{name: "single", body: `[{"id":7,"name":"only"}]`, wantID: 7},
		{name: "empty", body: `[]`, wantErr: ErrNoProjects, expectFail: true},
		{name: "multiple", body: `[{"id":1},{"id":2}]`, wantErr: ErrAmbiguousProject, expectFail: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			})

			id, err := resolver.ResolveSingleProjectID(context.Background(), 99)
			if tc.expectFail {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.wantID {
				t.Fatalf("expected id %d, got %d", tc.wantID, id)
			}
		})
	}
}

func TestAutoDetectProjectID_AmbiguousFlag(t *testing.T) {
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"id":11},{"id":22}]`)
	})

	id, ambiguous, err := resolver.AutoDetectProjectID(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 11 {
		t.Fatalf("expected first id 11, got %d", id)
	}
	if !ambiguous {
		t.Fatal("expected ambiguous=true with multiple projects")
	}
}

func TestProjectResolver_CachesResults(t *testing.T) {
	var calls int32
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = io.WriteString(w, `[{"id":1,"name":"x"}]`)
	})

	for i := 0; i < 3; i++ {
		if _, err := resolver.ListProjects(context.Background(), 5); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 upstream call, got %d", got)
	}

	resolver.Invalidate(5)
	if _, err := resolver.ListProjects(context.Background(), 5); err != nil {
		t.Fatalf("after invalidate: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 upstream calls after invalidate, got %d", got)
	}
}

func TestProjectResolver_CacheTTLExpiry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = io.WriteString(w, `[{"id":1}]`)
	}))
	defer server.Close()

	client, err := NewClientWithResponses(server.URL, WithHTTPClient(server.Client()), WithBearerToken(testUserJWT(1)))
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewProjectResolver(client, WithProjectsCacheTTL(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.ListProjects(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	if _, err := resolver.ListProjects(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected TTL expiry to trigger refetch, got %d calls", got)
	}
}

func TestGetProjectByName_NotFound(t *testing.T) {
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"id":1,"name":"alpha"}]`)
	})

	_, err := resolver.GetProjectByName(context.Background(), 1, "missing")
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestGetProjectByID_FindsMatch(t *testing.T) {
	resolver, _ := newTestProjectResolver(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"id":1,"name":"alpha"},{"id":2,"name":"beta"}]`)
	})

	p, err := resolver.GetProjectByID(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "beta" {
		t.Fatalf("expected beta, got %q", p.Name)
	}
}

type captureDoer struct {
	lastURL string
}

func (d *captureDoer) Do(req *http.Request) (*http.Response, error) {
	d.lastURL = req.URL.String()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`[]`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestNewProjectResolver_UsesGRSHostForProjectLookup(t *testing.T) {
	apiBaseURL, err := APIBaseURLForZone(ZoneNAM)
	if err != nil {
		t.Fatalf("APIBaseURLForZone: %v", err)
	}
	doer := &captureDoer{}
	client, err := NewClientWithResponses(apiBaseURL, WithHTTPClient(doer), WithBearerToken(testUserJWT(121041)))
	if err != nil {
		t.Fatalf("NewClientWithResponses: %v", err)
	}
	resolver, err := NewProjectResolver(client)
	if err != nil {
		t.Fatalf("NewProjectResolver: %v", err)
	}

	if _, err := resolver.ListProjects(context.Background(), 42); err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	wantPrefix := GrsBaseURLForZone(ZoneNAM) + "/grs/users/121041/projects"
	if !strings.HasPrefix(doer.lastURL, wantPrefix) {
		t.Fatalf("expected resolver call to use GRS host %q, got %q", wantPrefix, doer.lastURL)
	}
}

type tokenAwareDoer struct {
	token string
}

func (d *tokenAwareDoer) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`[]`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

func (d *tokenAwareDoer) AccessToken() string {
	return d.token
}

func (d *tokenAwareDoer) Refresh(ctx context.Context) error {
	if strings.TrimSpace(d.token) == "" {
		d.token = testUserJWT(99)
	}
	return nil
}

func TestAccessTokenFromClient_UnwrapsRetryingDoer(t *testing.T) {
	raw := &tokenAwareDoer{token: testUserJWT(77)}
	client := &Client{
		Client: NewRetryingHTTPClient(raw, RetryConfig{
			MaxRetries:    0,
			BaseDelay:     time.Millisecond,
			MaxDelay:      time.Millisecond,
			MaxRetryAfter: time.Second,
			RetryMethods:  map[string]bool{http.MethodGet: true},
		}),
	}

	token, err := accessTokenFromClient(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("accessTokenFromClient: %v", err)
	}
	if token != raw.token {
		t.Fatalf("expected token %q, got %q", raw.token, token)
	}
}
