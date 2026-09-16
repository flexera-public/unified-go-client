package flexera

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testBillUploadID = "0ae0f4c1-7c3a-4172-bcce-e96b9927fa07"

func TestBillUploadPushToolInvoke(t *testing.T) {
	dir := t.TempDir()
	first := writeBillUploadTestFile(t, dir, "first.csv", "first cost data")
	second := writeBillUploadTestFile(t, dir, "second.json", "second cost data")
	var actions []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/optima/orgs/42/billUploads":
			actions = append(actions, "create")
			if r.Method != http.MethodPost || string(body) != `{"billConnectId":"cbi-123","billingPeriod":"2026-09"}` {
				t.Fatalf("unexpected create request: method=%s body=%s", r.Method, body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"`+testBillUploadID+`"}`)
		case "/optima/orgs/42/billUploads/" + testBillUploadID + "/files/first.csv":
			actions = append(actions, "upload first")
			assertBillUploadFileRequest(t, r, body, "first cost data")
			w.WriteHeader(http.StatusCreated)
		case "/optima/orgs/42/billUploads/" + testBillUploadID + "/files/second.json":
			actions = append(actions, "upload second")
			assertBillUploadFileRequest(t, r, body, "second cost data")
			w.WriteHeader(http.StatusCreated)
		case "/optima/orgs/42/billUploads/" + testBillUploadID + "/operations":
			actions = append(actions, "commit")
			if r.Method != http.MethodPost || string(body) != `{"operation":"commit"}` {
				t.Fatalf("unexpected commit request: method=%s body=%s", r.Method, body)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewBillUploadPushTool(client).Invoke(context.Background(), BillUploadPushInput{
		OrgID:         42,
		BillConnectID: "cbi-123",
		BillingPeriod: "2026-09",
		Files:         []string{first, second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.BillUploadID != testBillUploadID || strings.Join(got.FileIDs, ",") != "first.csv,second.json" {
		t.Fatalf("unexpected output: %+v", got)
	}
	if strings.Join(actions, ",") != "create,upload first,upload second,commit" {
		t.Fatalf("unexpected action order: %v", actions)
	}
}

func TestBillUploadPushToolDoesNotCommitAfterFailedUpload(t *testing.T) {
	file := writeBillUploadTestFile(t, t.TempDir(), "cost.csv", "cost data")
	var committed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/optima/orgs/42/billUploads":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"`+testBillUploadID+`"}`)
		case strings.Contains(r.URL.Path, "/files/"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "invalid cost data")
		case strings.HasSuffix(r.URL.Path, "/operations"):
			committed = true
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewBillUploadPushTool(client).Invoke(context.Background(), BillUploadPushInput{
		OrgID: 42, BillConnectID: "cbi-123", BillingPeriod: "2026-09", Files: []string{file},
	})
	if err == nil || !strings.Contains(err.Error(), "upload file") {
		t.Fatalf("expected upload error, got %v", err)
	}
	if committed {
		t.Fatal("commit must not run after an upload failure")
	}
}

func TestBillUploadPushToolValidatesFilesBeforeCreating(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewBillUploadPushTool(client).Invoke(context.Background(), BillUploadPushInput{
		OrgID: 42, BillConnectID: "cbi-123", BillingPeriod: "2026-09", Files: []string{"missing.csv"},
	})
	if err == nil || !strings.Contains(err.Error(), "stat file") {
		t.Fatalf("expected file validation error, got %v", err)
	}
	if called {
		t.Fatal("create must not run when local file validation fails")
	}
}

func writeBillUploadTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertBillUploadFileRequest(t *testing.T, r *http.Request, body []byte, want string) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Fatalf("unexpected upload method: %s", r.Method)
	}
	if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("unexpected upload content type: %q", got)
	}
	if got := string(body); got != want {
		t.Fatalf("unexpected upload body: %q", got)
	}
}
