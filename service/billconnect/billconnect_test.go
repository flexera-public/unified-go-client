package billconnect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	flexera "github.com/flexera-public/unified-go-client"
	"github.com/flexera-public/unified-go-client/service/billconnect"
)

func newService(t *testing.T, h http.HandlerFunc) (*billconnect.Service, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(h)
	client, err := flexera.NewClientWithResponsesForAuth(flexera.ClientAuth{
		Zone:        flexera.ZoneNAM,
		APIBaseURL:  server.URL,
		AccessToken: "t",
		HTTPClient:  server.Client(),
	})
	if err != nil {
		server.Close()
		t.Fatalf("build client: %v", err)
	}
	return billconnect.New(client), server
}

func TestServiceList(t *testing.T) {
	svc, server := newService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/finops-onboarding/v1/orgs/123/bill-connects" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"finops:bill-connect-list","values":[]}`))
	})
	defer server.Close()

	resp, err := svc.List(context.Background(), 123)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		t.Fatalf("unexpected response: status=%d json200=%v", resp.StatusCode(), resp.JSON200)
	}
}

func TestServiceAzureCSPCreate(t *testing.T) {
	svc, server := newService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/finops-onboarding/v1/orgs/123/bill-connects/azure-csp" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"bc-1","azureCsp":{},"createdAt":"2026-01-01T00:00:00Z"}`))
	})
	defer server.Close()

	resp, err := svc.AzureCSPCreate(context.Background(), 123, flexera.FinopsOnboardingBillConnectAzureCSPCreateJSONRequestBody{
		ClientId: "client-guid",
	})
	if err != nil {
		t.Fatalf("AzureCSPCreate: %v", err)
	}
	if resp.JSON201 == nil {
		t.Fatalf("expected JSON201, status=%d body=%s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON201.Id != "bc-1" {
		t.Fatalf("expected id bc-1, got %q", resp.JSON201.Id)
	}
}
