package flexera

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestRetryConfig returns a deterministic config with no real sleeping.
func newTestRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   40 * time.Millisecond,
		Jitter:     false,
	}
}

// wrapWithFakeSleep returns a retrying doer whose sleep is captured rather
// than actually waited on. Tests can assert on the recorded waits.
func wrapWithFakeSleep(t *testing.T, doer HttpRequestDoer, cfg RetryConfig) (*retryingDoer, *[]time.Duration) {
	t.Helper()
	waits := []time.Duration{}
	d := NewRetryingHTTPClient(doer, cfg).(*retryingDoer)
	d.sleep = func(ctx context.Context, dur time.Duration) error {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		waits = append(waits, dur)
		return nil
	}
	return d, &waits
}

func TestRetryingDoer_PassThroughSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `ok`)
	}))
	defer srv.Close()

	doer, waits := wrapWithFakeSleep(t, srv.Client(), newTestRetryConfig())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	if len(*waits) != 0 {
		t.Fatalf("expected no sleeps, got %v", *waits)
	}
}

func TestRetryingDoer_RetriesOn429WithRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newTestRetryConfig()
	cfg.MaxRetryAfter = 5 * time.Second
	doer, waits := wrapWithFakeSleep(t, srv.Client(), cfg)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
	// Retry-After=2s must be honored (not clamped to MaxDelay=40ms) when
	// it fits within MaxRetryAfter.
	if len(*waits) != 1 || (*waits)[0] != 2*time.Second {
		t.Fatalf("expected 2s wait honoring Retry-After, got %v", *waits)
	}
}

func TestRetryingDoer_RetryAfterCappedByMaxRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := newTestRetryConfig()
	cfg.MaxRetryAfter = 50 * time.Millisecond
	doer, waits := wrapWithFakeSleep(t, srv.Client(), cfg)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if len(*waits) != 1 || (*waits)[0] != 50*time.Millisecond {
		t.Fatalf("expected Retry-After capped to MaxRetryAfter, got %v", *waits)
	}
}

func TestRetryingDoer_RetriesOn429WithoutRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	doer, waits := wrapWithFakeSleep(t, srv.Client(), newTestRetryConfig())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(*waits) != 1 || (*waits)[0] != 10*time.Millisecond {
		t.Fatalf("expected BaseDelay wait, got %v", *waits)
	}
}

func TestRetryingDoer_RetriesOn5xx(t *testing.T) {
	codes := []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
	for _, code := range codes {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt32(&calls, 1) < 2 {
					w.WriteHeader(code)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			doer, _ := wrapWithFakeSleep(t, srv.Client(), newTestRetryConfig())
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
			resp, err := doer.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestRetryingDoer_ExhaustsAndReturnsLastResponse(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `nope`)
	}))
	defer srv.Close()

	cfg := newTestRetryConfig()
	cfg.MaxRetries = 2
	doer, _ := wrapWithFakeSleep(t, srv.Client(), cfg)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "nope" {
		t.Fatalf("body lost; got %q", body)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestRetryingDoer_DoesNotRetryNon429_4xx(t *testing.T) {
	codes := []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}
	for _, code := range codes {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(code)
			}))
			defer srv.Close()

			doer, waits := wrapWithFakeSleep(t, srv.Client(), newTestRetryConfig())
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
			resp, err := doer.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != code {
				t.Fatalf("expected %d, got %d", code, resp.StatusCode)
			}
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Fatalf("expected 1 call, got %d", got)
			}
			if len(*waits) != 0 {
				t.Fatalf("expected no waits, got %v", *waits)
			}
		})
	}
}

func TestRetryingDoer_DoesNotRetryMutationsByDefault(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer srv.Close()

			doer, waits := wrapWithFakeSleep(t, srv.Client(), newTestRetryConfig())
			req, _ := http.NewRequestWithContext(context.Background(), method, srv.URL, strings.NewReader(`{}`))
			resp, err := doer.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Fatalf("expected 1 call for %s, got %d", method, got)
			}
			if len(*waits) != 0 {
				t.Fatalf("expected no waits, got %v", *waits)
			}
		})
	}
}

func TestRetryingDoer_ContextCancelDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := RetryConfig{MaxRetries: 5, BaseDelay: 50 * time.Millisecond, MaxDelay: 100 * time.Millisecond}
	doer := NewRetryingHTTPClient(srv.Client(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	start := time.Now()
	_, err := doer.Do(req)
	if err == nil {
		t.Fatal("expected context error")
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("retry did not honor ctx cancel quickly: %s", elapsed)
	}
}

func TestRetryingDoer_RetriesTransportErrorsOnIdempotent(t *testing.T) {
	var calls int32
	failingDoer := doerFunc(func(req *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1) < 2 {
			return nil, &fakeTempErr{}
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	doer, _ := wrapWithFakeSleep(t, failingDoer, newTestRetryConfig())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example", nil)
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestRetryingDoer_DoesNotRetryRequestWithUnsafeBody(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := newTestRetryConfig()
	cfg.RetryMethods = map[string]bool{http.MethodGet: true, http.MethodPost: true}
	doer, _ := wrapWithFakeSleep(t, srv.Client(), cfg)

	// io.Pipe gives a Body but no GetBody; replay is unsafe.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("x"))
		_ = pw.Close()
	}()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, pr)
	req.GetBody = nil
	resp, err := doer.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("3"); got != 3*time.Second {
		t.Fatalf("delta-seconds parse failed: %v", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Fatalf("empty should be 0, got %v", got)
	}
	if got := parseRetryAfter("not-a-date"); got != 0 {
		t.Fatalf("garbage should be 0, got %v", got)
	}
	future := time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 || got > 6*time.Second {
		t.Fatalf("http-date parse out of range: %v", got)
	}
}

func TestWithRetryingHTTPClient_ComposesWithGeneratedClient(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL,
		WithHTTPClient(srv.Client()),
		WithRetryingHTTPClient(newTestRetryConfig()),
	)
	if err != nil {
		t.Fatal(err)
	}
	// Replace internal sleep so the test stays instant.
	if rd, ok := client.Client.(*retryingDoer); ok {
		rd.sleep = func(ctx context.Context, d time.Duration) error { return nil }
	} else {
		t.Fatalf("expected client.Client to be *retryingDoer, got %T", client.Client)
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := client.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

// --- helpers ---

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

type fakeTempErr struct{}

func (fakeTempErr) Error() string   { return "temporary" }
func (fakeTempErr) Temporary() bool { return true }

// fakeOAuthDoer satisfies HttpRequestDoer + tokenInvalidator for 401
// retry tests. It returns scripted responses in order and counts
// Invalidate calls.
type fakeOAuthDoer struct {
	responses   []*http.Response
	idx         int
	invalidated int
}

func (f *fakeOAuthDoer) Do(*http.Request) (*http.Response, error) {
	if f.idx >= len(f.responses) {
		return nil, fmt.Errorf("fakeOAuthDoer: exhausted at idx=%d", f.idx)
	}
	r := f.responses[f.idx]
	f.idx++
	return r, nil
}

func (f *fakeOAuthDoer) Invalidate() { f.invalidated++ }

func newResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}

func TestRetryingDoer_RetriesOn401WithInvalidate(t *testing.T) {
	fake := &fakeOAuthDoer{responses: []*http.Response{
		newResp(http.StatusUnauthorized, ""),
		newResp(http.StatusOK, `{"ok":true}`),
	}}
	rd := NewRetryingHTTPClient(fake, RetryConfig{MaxRetries: 2, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://x.example/y", nil)
	resp, err := rd.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if fake.invalidated != 1 {
		t.Fatalf("expected 1 Invalidate call, got %d", fake.invalidated)
	}
}

func TestRetryingDoer_DoesNotRetry401WithoutInvalidator(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	rd := NewRetryingHTTPClient(http.DefaultClient, RetryConfig{MaxRetries: 3, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := rd.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 unretried, got %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("expected 1 server call (no retry), got %d", calls)
	}
}
