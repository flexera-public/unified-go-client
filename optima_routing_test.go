package flexera

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOptimaRoutingDoer_ReroutesOptimaHostedPaths(t *testing.T) {
	var gotHost, gotPath string
	optimaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer optimaSrv.Close()

	cli, err := NewClientWithResponses("https://api.flexera.com/", WithOptimaRouting(optimaSrv.URL))
	if err != nil {
		t.Fatal(err)
	}

	wantHost := strings.TrimPrefix(strings.TrimPrefix(optimaSrv.URL, "http://"), "https://")
	for _, path := range []string{
		"/bill-analysis/orgs/1105/custom/dimensions",
		"/analytics/orgs/1105/billing_centers",
		"/recommendations/orgs/1105/recommendations",
	} {
		gotHost, gotPath = "", ""
		resp, err := cli.ClientInterface.(*Client).Client.Do(mustRequest(t, "GET", "https://api.flexera.com"+path))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if gotHost != wantHost {
			t.Fatalf("path %q: expected rerouted to Optima host %q, got %q", path, wantHost, gotHost)
		}
		if gotPath != path {
			t.Fatalf("path %q: expected path unchanged, got %q", path, gotPath)
		}
	}
}

func TestOptimaRoutingDoer_LeavesUnrelatedRecommendationSingularAlone(t *testing.T) {
	var gotHost string
	gatewaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer gatewaySrv.Close()

	// /recommendation (singular) is the unrelated Flexera risk
	// misconfiguration API and must stay on the gateway host.
	doer := &optimaRoutingDoer{next: http.DefaultClient, optimaHost: mustURL(t, "https://api.optima.flexeraeng.com")}
	resp, err := doer.Do(mustRequest(t, "GET", gatewaySrv.URL+"/recommendation/v1/orgs/1105/misconfiguration/list"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	wantHost := strings.TrimPrefix(strings.TrimPrefix(gatewaySrv.URL, "http://"), "https://")
	if gotHost != wantHost {
		t.Fatalf("expected /recommendation (singular) untouched (host %q), got %q", wantHost, gotHost)
	}
}

func TestOptimaRoutingDoer_LeavesOtherPathsAlone(t *testing.T) {
	var gotHost string
	gatewaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer gatewaySrv.Close()

	doer := &optimaRoutingDoer{next: http.DefaultClient, optimaHost: mustURL(t, "https://api.optima.flexeraeng.com")}
	resp, err := doer.Do(mustRequest(t, "GET", gatewaySrv.URL+"/iam/v1/orgs/1105"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	wantHost := strings.TrimPrefix(strings.TrimPrefix(gatewaySrv.URL, "http://"), "https://")
	if gotHost != wantHost {
		t.Fatalf("expected non-Optima-hosted request untouched (host %q), got %q", wantHost, gotHost)
	}
}

func TestWithOptimaRouting_EmptyIsNoop(t *testing.T) {
	c := &Client{Client: http.DefaultClient}
	if err := WithOptimaRouting("")(c); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Client.(*optimaRoutingDoer); ok {
		t.Fatal("expected empty optimaBaseURL to be a no-op")
	}
}

func mustRequest(t *testing.T, method, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func mustURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
