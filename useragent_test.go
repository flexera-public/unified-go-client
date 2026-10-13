package flexera

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
)

func uaCapture(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func doGet(t *testing.T, d HttpRequestDoer, url string, ua string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	resp, err := d.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ua != "" && req.Header.Get("User-Agent") != ua {
		t.Fatalf("caller request mutated")
	}
}

func TestDefaultUserAgentFormat(t *testing.T) {
	ua := DefaultUserAgent()
	re := regexp.MustCompile(`^flexera-unified-go-client/\S+ \(.*go\d.*; \w+/\w+\)$`)
	if !re.MatchString(ua) {
		t.Fatalf("unexpected UA %q", ua)
	}
}

func TestBuildUserAgent(t *testing.T) {
	bi := &debug.BuildInfo{
		Main: debug.Module{Path: modulePath, Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	got := buildUserAgent("", bi, "go1.24.5", "linux", "amd64")
	want := "flexera-unified-go-client/devel (0123456789ab; dirty; go1.24.5; linux/amd64)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	dep := &debug.BuildInfo{
		Main: debug.Module{Path: "example.com/cli"},
		Deps: []*debug.Module{{Path: modulePath, Version: "v0.0.0-20240102150405-abcdef123456"}},
	}
	got = buildUserAgent("", dep, "go1.24.5", "linux", "amd64")
	want = "flexera-unified-go-client/v0.0.0-20240102150405-abcdef123456 (abcdef123456; go1.24.5; linux/amd64)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got = buildUserAgent("v1.2.3", nil, "go1.24.5", "linux", "amd64")
	if !strings.HasPrefix(got, "flexera-unified-go-client/v1.2.3 (") {
		t.Fatalf("ldflags version ignored: %q", got)
	}
}

func TestUserAgentDoer(t *testing.T) {
	srv, got := uaCapture(t)
	d := NewUserAgentDoer(nil, "")
	doGet(t, d, srv.URL, "")
	doGet(t, d, srv.URL, "custom/1")
	if (*got)[0] != DefaultUserAgent() || (*got)[1] != "custom/1" {
		t.Fatalf("got %v", *got)
	}
}

func TestNewClientWithResponsesForAuth_UserAgent(t *testing.T) {
	srv, got := uaCapture(t)
	newClient := func(opts ...ClientOption) *ClientWithResponses {
		cli, err := NewClientWithResponsesForAuth(ClientAuth{
			Zone: ZoneNAM, AccessToken: "tok", APIBaseURL: srv.URL, LoginBaseURL: srv.URL,
		}, opts...)
		if err != nil {
			t.Fatal(err)
		}
		return cli
	}
	call := func(cli *ClientWithResponses, eds ...RequestEditorFn) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		for _, e := range eds {
			_ = e(context.Background(), req)
		}
		resp, err := cli.ClientInterface.(*Client).Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	call(newClient())
	call(newClient(WithUserAgent("flexera-cli/9")))
	call(newClient(WithUserAgentProduct("flexera-cli", "1.2.3")))
	call(newClient(WithUserAgent("flexera-cli/9")), func(_ context.Context, r *http.Request) error {
		r.Header.Set("User-Agent", "per-request")
		return nil
	})

	if (*got)[0] != DefaultUserAgent() {
		t.Errorf("default: %q", (*got)[0])
	}
	if (*got)[1] != "flexera-cli/9" {
		t.Errorf("override: %q", (*got)[1])
	}
	if (*got)[2] != "flexera-cli/1.2.3 "+DefaultUserAgent() {
		t.Errorf("product: %q", (*got)[2])
	}
	if (*got)[3] != "per-request" {
		t.Errorf("per-request: %q", (*got)[3])
	}
}

func TestOAuthTokenAndAPIRequestsCarryUserAgent(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("User-Agent"))
		if strings.HasSuffix(r.URL.Path, "/oidc/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":3600}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cli, err := NewClientWithResponsesForAuth(ClientAuth{
		Zone: ZoneNAM, ClientID: "a", ClientSecret: "b", APIBaseURL: srv.URL, LoginBaseURL: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	resp, err := cli.ClientInterface.(*Client).Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(got) != 2 {
		t.Fatalf("expected token + API request, got %v", got)
	}
	for _, ua := range got {
		if ua != DefaultUserAgent() {
			t.Errorf("UA %q", ua)
		}
	}
}
