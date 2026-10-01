package flexera

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCostHelperUsesMergedBillAnalysisClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bill-analysis/orgs/42/costs/aggregated" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body BillAnalysisAggregatedRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(body.BillingCenterIds) != 1 || body.BillingCenterIds[0] != "bc-1" {
			t.Errorf("unexpected billing centers: %v", body.BillingCenterIds)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows":[{"dimensions":{"service":"compute"},"metrics":{"cost_amortized_unblended_adj":12},"timestamp":"2026-01-01T00:00:00Z"}],"rowsTruncated":true}`))
	}))
	defer server.Close()

	client, err := NewClientWithResponses(server.URL, WithOptimaRouting(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	result, err := New(client, nil).GetCost(context.Background(), Request{
		OrgID: 42, BillingCenterIDs: []string{"bc-1"}, StartAt: "2026-01", EndAt: "2026-02",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || !result.RowsTruncated || result.Rows[0].Metrics["cost_amortized_unblended_adj"] != 12 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestBillingCenterResolverUsesMergedClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analytics/orgs/42/billing_centers" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"top","name":"Top"},{"id":"child","name":"Child","parent_id":"top"}]`))
	}))
	defer server.Close()
	client, err := NewClientWithResponses(server.URL, WithOptimaRouting(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	ids, err := NewBillingCenterResolver(client).TopLevelIDs(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "top" {
		t.Fatalf("unexpected top-level IDs: %v", ids)
	}
}

func TestGenerateQueryUsesUnifiedClientEditors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql/v1/orgs/42/generate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("missing request headers: %v", r.Header)
		}
		_, _ = w.Write([]byte(`{"prompt":"cost","query":"{ cost }","valid":true}`))
	}))
	defer server.Close()
	client, err := NewClientWithResponses(server.URL, WithBearerToken("test"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GenerateQuery(context.Background(), 42, GenerateQueryRequestBody{Prompt: "cost"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Query != "{ cost }" {
		t.Fatalf("unexpected generated query: %+v", result)
	}
}
