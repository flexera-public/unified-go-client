package flexera

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type fakeCost struct {
	mu       sync.Mutex
	agg      []BillAnalysisAggregatedRequestBody
	sel      []BillAnalysisSelectRequestBody
	exp      []BillAnalysisExportSelectRequestBody
	statuses int
	selFn    func(b BillAnalysisSelectRequestBody) []BillAnalysisRow
	aggFn    func(b BillAnalysisAggregatedRequestBody) []BillAnalysisRow
}

func okBody(rows []BillAnalysisRow) ([]byte, *http.Response) {
	var sb strings.Builder
	sb.WriteString(`{"rows":[`)
	for i, r := range rows {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"dimensions":{`)
		first := true
		for k, v := range r.Dimensions {
			if !first {
				sb.WriteString(",")
			}
			first = false
			fmt.Fprintf(&sb, "%q:%q", k, v)
		}
		sb.WriteString(`},"metrics":{},"timestamp":"2026-01-01T00:00:00Z"}`)
	}
	sb.WriteString(`]}`)
	return []byte(sb.String()), &http.Response{StatusCode: 200}
}

func (f *fakeCost) BillAnalysisCostsAggregatedWithResponse(_ context.Context, _ int64, b BillAnalysisCostsAggregatedJSONRequestBody, _ ...RequestEditorFn) (*BillAnalysisCostsAggregatedResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agg = append(f.agg, b)
	var rows []BillAnalysisRow
	if f.aggFn != nil {
		rows = f.aggFn(b)
	}
	body, hr := okBody(rows)
	return &BillAnalysisCostsAggregatedResponse{Body: body, HTTPResponse: hr}, nil
}

func (f *fakeCost) BillAnalysisCostsSelectWithResponse(_ context.Context, _ int64, b BillAnalysisCostsSelectJSONRequestBody, _ ...RequestEditorFn) (*BillAnalysisCostsSelectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sel = append(f.sel, b)
	var rows []BillAnalysisRow
	if f.selFn != nil {
		rows = f.selFn(b)
	}
	body, hr := okBody(rows)
	return &BillAnalysisCostsSelectResponse{Body: body, HTTPResponse: hr}, nil
}

func (f *fakeCost) BillAnalysisCostsExportSelectWithResponse(_ context.Context, _ int64, b BillAnalysisCostsExportSelectJSONRequestBody, _ ...RequestEditorFn) (*BillAnalysisCostsExportSelectResponse, error) {
	f.exp = append(f.exp, b)
	return &BillAnalysisCostsExportSelectResponse{
		HTTPResponse: &http.Response{StatusCode: 202},
		JSON202:      &BillAnalysisExportSelectResult{ExportId: "e1"},
	}, nil
}

func (f *fakeCost) BillAnalysisCostsExportSelectStatusWithResponse(_ context.Context, _ int64, _ string, _ ...RequestEditorFn) (*BillAnalysisCostsExportSelectStatusResponse, error) {
	f.statuses++
	st := BillAnalysisExportSelectStatusResultStatusInProgress
	var urls *map[string]string
	if f.statuses >= 2 {
		st = BillAnalysisExportSelectStatusResultStatusDone
		urls = &map[string]string{"2026-01": "http://x"}
	}
	return &BillAnalysisCostsExportSelectStatusResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200:      &BillAnalysisExportSelectStatusResult{Status: st, Urls: urls},
	}, nil
}

func baseReq(dims ...string) BillAnalysisCostsQueryRequest {
	return BillAnalysisCostsQueryRequest{
		OrgID: 1, BillingCenterIDs: []string{"bc"}, StartAt: "2026-01", EndAt: "2026-02",
		Dimensions: dims, Progress: func(string) {},
	}
}

func TestQueryRouting(t *testing.T) {
	many := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	tests := []struct {
		name    string
		dims    []string
		export  bool
		wantEP  BillAnalysisCostsEndpoint
		wantSel int
	}{
		{"few dims", []string{"service"}, false, CostsEndpointAggregated, 0},
		{"ten dims", many[:10], false, CostsEndpointAggregated, 0},
		{"eleven dims", many, false, CostsEndpointSelect, 1},
		{"resource_id", []string{"resource_id"}, false, CostsEndpointSelect, 1},
		{"export", []string{"service"}, true, CostsEndpointExport, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCost{}
			r := baseReq(tc.dims...)
			r.Export = tc.export
			r.ExportPollInterval = 1
			out, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if out.EndpointUsed != tc.wantEP || len(f.sel) != tc.wantSel {
				t.Fatalf("endpoint=%s sel=%d", out.EndpointUsed, len(f.sel))
			}
			if tc.export && (len(f.exp) != 1 || out.ExportURLs["2026-01"] == "" || out.ExportStatus != "Done") {
				t.Fatalf("export: %+v", out)
			}
		})
	}
}

func TestQuerySelectSendsLimit(t *testing.T) {
	f := &fakeCost{}
	if _, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), baseReq("resource_id")); err != nil {
		t.Fatal(err)
	}
	if f.sel[0].Limit != CostsMaxSelectRows {
		t.Fatalf("limit = %d", f.sel[0].Limit)
	}
}

func TestQuerySlicesWhenTruncated(t *testing.T) {
	f := &fakeCost{}
	f.aggFn = func(b BillAnalysisAggregatedRequestBody) []BillAnalysisRow {
		d := (*b.Dimensions)[0]
		if d != "vendor" {
			t.Errorf("unexpected slice dimension %s", d)
		}
		return []BillAnalysisRow{{Dimensions: map[string]string{"vendor": "aws"}}, {Dimensions: map[string]string{"vendor": "azure"}}}
	}
	f.selFn = func(b BillAnalysisSelectRequestBody) []BillAnalysisRow {
		if b.Filter == nil {
			rows := make([]BillAnalysisRow, CostsMaxSelectRows)
			for i := range rows {
				rows[i].Dimensions = map[string]string{"resource_id": "x"}
			}
			return rows
		}
		return []BillAnalysisRow{{Dimensions: map[string]string{"resource_id": "y"}}}
	}
	out, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), baseReq("resource_id"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 2 || out.RowsTruncated || len(f.sel) != 3 {
		t.Fatalf("rows=%d trunc=%v sel=%d", len(out.Rows), out.RowsTruncated, len(f.sel))
	}
}

func TestQuerySliceSkipsFilteredDimension(t *testing.T) {
	f := &fakeCost{}
	f.aggFn = func(b BillAnalysisAggregatedRequestBody) []BillAnalysisRow {
		if (*b.Dimensions)[0] == "vendor" {
			t.Errorf("sliced on a dimension present in the filter")
		}
		return []BillAnalysisRow{{Dimensions: map[string]string{(*b.Dimensions)[0]: "1"}}, {Dimensions: map[string]string{(*b.Dimensions)[0]: "2"}}}
	}
	f.selFn = func(b BillAnalysisSelectRequestBody) []BillAnalysisRow {
		if len(f.sel) == 1 {
			return make([]BillAnalysisRow, CostsMaxSelectRows)
		}
		return nil
	}
	r := baseReq("resource_id")
	r.Filter = equalFilter("vendor", "aws")
	if _, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}

func TestQueryWithBody(t *testing.T) {
	f := &fakeCost{}
	body := `{"billing_center_ids":["bc"],"start_at":"2026-01","end_at":"2026-02","granularity":"month","dimensions":["resource_id"],"metrics":["m"],"limit":5}`
	out, err := NewBillAnalysisCostsQuery(f, nil).QueryWithBody(context.Background(), 1, "application/json", strings.NewReader(body), BillAnalysisCostsQueryRequest{Progress: func(string) {}})
	if err != nil {
		t.Fatal(err)
	}
	if out.EndpointUsed != CostsEndpointSelect || f.sel[0].Limit != 5 || f.sel[0].Metrics[0] != "m" {
		t.Fatalf("unexpected: %+v %+v", out, f.sel)
	}
}

func TestQueryLineItems(t *testing.T) {
	f := &fakeCost{}
	r := baseReq("service")
	r.LineItems = true
	out, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), r)
	if err != nil || out.EndpointUsed != CostsEndpointSelect || len(f.agg) != 0 {
		t.Fatalf("err=%v out=%+v", err, out)
	}
	r.Endpoint = CostsEndpointAggregated
	if _, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), r); err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestQueryExportAggregateFlag(t *testing.T) {
	for _, li := range []bool{false, true} {
		f := &fakeCost{}
		r := baseReq("service")
		r.Export, r.LineItems, r.ExportPollInterval = true, li, 1
		if _, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), r); err != nil {
			t.Fatal(err)
		}
		if a := f.exp[0].AggregateToRequestedDimensions; a == nil || *a == li {
			t.Fatalf("lineItems=%v aggregate=%v", li, a)
		}
	}
}

func TestQueryUserLimitDoesNotSlice(t *testing.T) {
	f := &fakeCost{}
	f.selFn = func(BillAnalysisSelectRequestBody) []BillAnalysisRow { return make([]BillAnalysisRow, 10) }
	r := baseReq("resource_id")
	lim := int64(10)
	r.Limit = &lim
	out, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.sel) != 1 || len(f.agg) != 0 || len(out.Rows) != 10 {
		t.Fatalf("sel=%d agg=%d rows=%d", len(f.sel), len(f.agg), len(out.Rows))
	}
}

func TestSliceValueListIgnoresUserLimit(t *testing.T) {
	f := &fakeCost{}
	f.aggFn = func(b BillAnalysisAggregatedRequestBody) []BillAnalysisRow {
		if b.Limit != nil {
			t.Errorf("value-listing request carried limit %d", *b.Limit)
		}
		return []BillAnalysisRow{{Dimensions: map[string]string{"vendor": "a"}}, {Dimensions: map[string]string{"vendor": "b"}}}
	}
	f.selFn = func(BillAnalysisSelectRequestBody) []BillAnalysisRow {
		if len(f.sel) == 1 {
			return make([]BillAnalysisRow, CostsMaxSelectRows)
		}
		return nil
	}
	r := baseReq("resource_id")
	lim := int64(CostsMaxSelectRows)
	r.Limit = &lim
	if _, err := NewBillAnalysisCostsQuery(f, nil).Query(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}
