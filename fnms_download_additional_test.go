package flexera

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type fnmsCustomDoer struct{}

func (fnmsCustomDoer) Do(*http.Request) (*http.Response, error) {
	panic("unsupported doer must not run")
}

func TestFnmsDownloadRejectsUnknownDoer(t *testing.T) {
	c, err := NewClientWithResponses("https://api.example", WithHTTPClient(fnmsCustomDoer{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.FnmsDownloadFile(context.Background(), 1, "file", io.Discard, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error: %v", err)
	}
}

func TestFnmsDownloadRejectsDowngrade(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Location": []string{"http://example.com/file"}}, Request: &http.Request{URL: &url.URL{Scheme: "https", Host: "api.example.com"}}}
	_, err := fnmsRedirectURL(resp)
	if err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("error: %v", err)
	}
}

type fnmsCancelWriter struct {
	cancel context.CancelFunc
	buffer bytes.Buffer
}

func (w *fnmsCancelWriter) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	w.cancel()
	return n, err
}

func TestFnmsDownloadStreamingCancellation(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		io.WriteString(w, "partial")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer api.Close()
	c := fnmsTestClient(t, api)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dst := &fnmsCancelWriter{cancel: cancel}
	n, err := c.FnmsDownloadReportExport(ctx, "/file", dst, nil)
	if !errors.Is(err, context.Canceled) || n != 7 || dst.buffer.String() != "partial" {
		t.Fatalf("n=%d body=%q error=%v", n, dst.buffer.String(), err)
	}
}

func TestFnmsStartReportMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		export     bool
		code       int
		body, want string
	}{
		{"missing job", false, 202, `{}`, "job ID"},
		{"missing conflict job", false, 409, `{}`, "job ID"},
		{"unexpected execution status", false, 400, `{}`, "400"},
		{"missing export data", true, 201, `{}`, "export ID"},
		{"missing export id", true, 201, `{"data":{}}`, "export ID"},
		{"unexpected export conflict", true, 409, `{}`, "409"},
		{"invalid json", false, 202, `{`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fnmsTestJSON(w, tc.code, tc.body) }))
			defer api.Close()
			c := fnmsTestClient(t, api)
			var err error
			if tc.export {
				_, err = c.FnmsStartReportExport(context.Background(), 1, "r", nil)
			} else {
				_, err = c.FnmsStartReportExecution(context.Background(), 1, "r", nil)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error: %v", err)
			}
		})
	}
}
