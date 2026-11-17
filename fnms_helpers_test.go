package flexera

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fnmsTestClient(t *testing.T, server *httptest.Server, options ...ClientOption) *ClientWithResponses {
	t.Helper()
	c, err := NewClientWithResponses(server.URL, options...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func fnmsTestJSON(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprint(w, body)
}

func TestFnmsDownloadFile(t *testing.T) {
	var externalCalls int
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		externalCalls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Api-Secret") != "" {
			t.Error("API credentials leaked")
		}
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/file")
			w.WriteHeader(307)
			return
		}
		fmt.Fprint(w, "file content")
	}))
	defer external.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fnms/v1/orgs/7/files/export.zip" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer api-token" {
			t.Error("missing API auth")
		}
		w.Header().Set("Location", external.URL+"/redirect")
		w.WriteHeader(303)
	}))
	defer api.Close()
	original := api.Client()
	original.CheckRedirect = func(*http.Request, []*http.Request) error { t.Error("original redirect callback invoked"); return nil }
	c := fnmsTestClient(t, api, WithHTTPClient(original), WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer api-token")
		r.Header.Set("X-Api-Secret", "secret")
		r.Header.Set("Cookie", "secret=value")
		return nil
	}))
	var dst bytes.Buffer
	n, err := c.FnmsDownloadFile(context.Background(), 7, "export.zip", &dst, nil)
	if err != nil || n != 12 || dst.String() != "file content" || externalCalls != 2 {
		t.Fatalf("n=%d content=%q calls=%d err=%v", n, dst.String(), externalCalls, err)
	}
	if original.CheckRedirect == nil {
		t.Fatal("original client mutated")
	}
}

type fnmsFailWriter struct{ err error }

func (w fnmsFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestFnmsDownloadFailures(t *testing.T) {
	for _, tc := range []struct {
		name, location string
		status         int
		max            int
		want           string
	}{
		{"missing location", "", 303, 0, "missing Location"},
		{"api error", "", 403, 0, "403"},
		{"invalid scheme", "file:///secret", 303, 0, "HTTP(S)"},
		{"credentials", "http://user:pass@example.com", 303, 0, "credentials"},
		{"redirect loop", "/loop", 303, 2, "redirect limit"},
		{"download error", "/failed", 303, 0, "503"},
		{"negative limit", "", 303, -1, "negative"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/failed" {
					w.WriteHeader(503)
					return
				}
				w.Header().Set("Location", tc.location)
				w.WriteHeader(tc.status)
			}))
			defer api.Close()
			c := fnmsTestClient(t, api)
			_, err := c.FnmsDownloadFile(context.Background(), 1, "file", io.Discard, &FnmsDownloadOptions{MaxRedirects: tc.max})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error: %v", err)
			}
		})
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "content") }))
	defer api.Close()
	c := fnmsTestClient(t, api)
	sentinel := errors.New("write failed")
	_, err := c.FnmsDownloadReportExport(context.Background(), "/file", fnmsFailWriter{sentinel}, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("writer error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.FnmsDownloadReportExport(ctx, "/file", io.Discard, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context error: %v", err)
	}
}

func TestFnmsExportDownloadAuthentication(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("external authorization leaked")
		}
		fmt.Fprint(w, "zip")
	}))
	defer destination.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing same-origin auth")
		}
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(302)
	}))
	defer api.Close()
	oauth := NewOAuth2Doer("", "", "", "", api.Client())
	oauth.accessToken, oauth.expiry = "token", time.Now().Add(time.Hour)
	c := fnmsTestClient(t, api, WithHTTPClient(NewRetryingHTTPClient(oauth, RetryConfig{})))
	for _, path := range []string{"/fnmsdata/v1/signed", destination.URL} {
		var dst bytes.Buffer
		n, err := c.FnmsDownloadReportExport(context.Background(), path, &dst, nil)
		if err != nil || n != 3 || dst.String() != "zip" {
			t.Fatalf("n=%d body=%s error=%v", n, &dst, err)
		}
	}
}

func TestFnmsExecuteReport(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			polls := 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "POST":
					if r.URL.Query().Get("filterParams[DeviceName]") != "a & b" || r.URL.Query().Get("forceNewExecution") != "true" || r.URL.Query().Get("limit") != "5000" {
						t.Errorf("query: %s", r.URL.RawQuery)
					}
					if conflict {
						fnmsTestJSON(w, 409, `{"jobId":"existing","status":"In progress"}`)
					} else {
						fnmsTestJSON(w, 202, `{"jobId":"existing","status":"Created"}`)
					}
				default:
					if !strings.Contains(r.URL.Path, "existing") {
						t.Errorf("path: %s", r.URL.Path)
					}
					polls++
					status := "In progress"
					if polls == 2 {
						status = "Completed"
					}
					fnmsTestJSON(w, 200, fmt.Sprintf(`{"jobId":"existing","status":%q,"totalPages":2}`, status))
				}
			}))
			defer api.Close()
			c := fnmsTestClient(t, api)
			filters := map[string]interface{}{"DeviceName": "a & b"}
			force := true
			limit := int64(5000)
			got, err := c.FnmsExecuteReport(context.Background(), 1, "r", &FnmsReportExecuteAsyncParams{FilterParams: &filters, ForceNewExecution: &force, Limit: &limit}, &FnmsPollOptions{Interval: time.Millisecond})
			if err != nil || got.JobId != "existing" || polls != 2 || *got.TotalPages != 2 {
				t.Fatalf("result=%+v polls=%d err=%v", got, polls, err)
			}
		})
	}
}

func TestFnmsExportReport(t *testing.T) {
	polls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if r.URL.Query().Get("filterParams[Owner]") != "owner" || r.Header.Get("Accept") != "text/csv" {
				t.Errorf("query=%s headers=%v", r.URL.RawQuery, r.Header)
			}
			fnmsTestJSON(w, 201, `{"data":{"exportId":"e"}}`)
			return
		}
		polls++
		if polls == 1 {
			fnmsTestJSON(w, 200, `{"data":{"status":"Created"}}`)
		} else {
			fnmsTestJSON(w, 200, `{"data":{"status":"Completed","path":"/fnmsdata/v1/file"}}`)
		}
	}))
	defer api.Close()
	c := fnmsTestClient(t, api)
	filters := map[string]interface{}{"Owner": "owner"}
	accept := FnmsReportExportReportParamsAccept("text/csv")
	got, err := c.FnmsExportReport(context.Background(), 1, "r", &FnmsReportExportReportParams{FilterParams: &filters, Accept: &accept}, &FnmsPollOptions{Interval: time.Millisecond})
	if err != nil || got.Path == nil || polls != 2 {
		t.Fatalf("result=%+v polls=%d err=%v", got, polls, err)
	}
}

func TestFnmsPollingTerminalAndMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		export       bool
		code         int
		want         string
	}{
		{"execution failure", "Errored", false, 200, "errored"},
		{"unknown execution", "Unknown", false, 200, "unknown"},
		{"unknown export", "Unknown", true, 200, "unknown"},
		{"export failure", "Errored", true, 200, "errored"},
		{"no data", "No data found", true, 200, ""},
		{"missing path", "Completed", true, 200, "missing path"},
		{"http failure", "", true, 503, "503"},
		{"missing execution status", "", false, 200, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := fmt.Sprintf(`{"status":%q,"errorMessage":"failed"}`, tc.status)
				if tc.export {
					body = `{"data":` + body + `}`
				}
				fnmsTestJSON(w, tc.code, body)
			}))
			defer api.Close()
			c := fnmsTestClient(t, api)
			var err error
			if tc.export {
				_, err = c.FnmsWaitReportExport(context.Background(), 1, "e", nil)
			} else {
				_, err = c.FnmsWaitReportExecution(context.Background(), 1, "r", "j", nil)
			}
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

func TestFnmsPollingCancellation(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fnmsTestJSON(w, 200, `{"status":"Created"}`) }))
	defer api.Close()
	c := fnmsTestClient(t, api)
	got, err := c.FnmsWaitReportExecution(context.Background(), 1, "r", "j", &FnmsPollOptions{Interval: time.Hour, Timeout: 10 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) || got == nil {
		t.Fatalf("last=%+v err=%v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.FnmsWaitReportExecution(ctx, 1, "r", "j", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error: %v", err)
	}
	_, err = c.FnmsExecuteReport(context.Background(), 1, "r", nil, &FnmsPollOptions{Timeout: -1})
	if err == nil {
		t.Fatal("expected invalid options error")
	}
}
