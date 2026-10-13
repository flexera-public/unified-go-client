package flexera

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// BillAnalysisCostsQueryClient is the subset of the unified client the helper depends on.
type BillAnalysisCostsQueryClient interface {
	BillAnalysisCostsAggregatedWithResponse(
		ctx context.Context,
		org int64,
		body BillAnalysisCostsAggregatedJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*BillAnalysisCostsAggregatedResponse, error)
	BillAnalysisCostsSelectWithResponse(
		ctx context.Context,
		org int64,
		body BillAnalysisCostsSelectJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*BillAnalysisCostsSelectResponse, error)
	BillAnalysisCostsExportSelectWithResponse(
		ctx context.Context,
		org int64,
		body BillAnalysisCostsExportSelectJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*BillAnalysisCostsExportSelectResponse, error)
	BillAnalysisCostsExportSelectStatusWithResponse(
		ctx context.Context,
		org int64,
		exportID string,
		reqEditors ...RequestEditorFn,
	) (*BillAnalysisCostsExportSelectStatusResponse, error)
}

// BillAnalysisCostsEndpoint selects which underlying cost endpoint to use.
type BillAnalysisCostsEndpoint string

const (
	CostsEndpointAuto       BillAnalysisCostsEndpoint = "auto"
	CostsEndpointAggregated BillAnalysisCostsEndpoint = "aggregated"
	CostsEndpointSelect     BillAnalysisCostsEndpoint = "select"
	// CostsEndpointExport is the async export endpoint (limited to 1/day per org).
	CostsEndpointExport BillAnalysisCostsEndpoint = "export"
)

const (
	// CostsMaxPeriodMonths is the upstream API limit on a single cost request window.
	CostsMaxPeriodMonths = 24
	// CostsMaxAggregatedDimensions is the largest dimension count routed to /aggregated.
	CostsMaxAggregatedDimensions = 10
	// CostsMaxSelectRows is the /select row cap; a response this size is treated as truncated.
	CostsMaxSelectRows = 100000
	// CostsDefaultProgressInterval is how often auto-pagination progress is reported.
	CostsDefaultProgressInterval = 30 * time.Second
	// CostsDefaultExportPollInterval is how often an export's status is polled.
	CostsDefaultExportPollInterval = 30 * time.Second
)

// sliceDimensions are the preferred dimensions to slice on, in order, when a
// /select response hits the row cap.
var sliceDimensions = []string{"vendor", "vendor_account", "region", "service", "resource_type"}

// BillAnalysisCostsGranularity is "month" or "day".
type BillAnalysisCostsGranularity string

const (
	CostsGranularityMonth BillAnalysisCostsGranularity = "month"
	CostsGranularityDay   BillAnalysisCostsGranularity = "day"
)

// BillAnalysisCostsQueryRequest describes a logical cost query. It is converted into one or more
// bill_analysis API calls by Query.
type BillAnalysisCostsQueryRequest struct {
	OrgID            int
	BillingCenterIDs []string // empty → resolver fills in top-level set

	StartAt     string // YYYY-MM (month) or YYYY-MM-DD (day); inclusive
	EndAt       string // YYYY-MM (month) or YYYY-MM-DD (day); exclusive
	Granularity BillAnalysisCostsGranularity

	Dimensions []string
	Metrics    []string // empty → ["cost_amortized_unblended_adj"]
	Filter     *BillAnalysisFilterV1
	Summarized *bool

	// Limit caps rows per request (aggregated: optional; select: default and
	// max CostsMaxSelectRows).
	Limit *int64
	// PeriodType is "billing_period" or "charge_period"; empty uses the API default.
	PeriodType string

	// LineItems requests row-level data (default false). When true, /aggregated
	// is never used: auto routing picks /select, and export is sent with
	// aggregate_to_requested_dimensions=false. When false, export is sent with
	// aggregate_to_requested_dimensions=true so the export is aggregated.
	LineItems bool

	// AdjDimensionGranularity ("none" or "org-level") applies to export only.
	AdjDimensionGranularity string

	// Endpoint selects /aggregated, /select, /export, or auto (default). Auto
	// picks /select if there are more than CostsMaxAggregatedDimensions dimensions
	// or Dimensions contains "resource_id", otherwise /aggregated.
	Endpoint BillAnalysisCostsEndpoint

	// Export uses the async export endpoint (limited to 1 per day per org).
	// Equivalent to Endpoint == CostsEndpointExport.
	Export bool
	// ExportNoWait returns right after the export is started, without polling.
	ExportNoWait bool
	// ExportPollInterval overrides CostsDefaultExportPollInterval.
	ExportPollInterval time.Duration

	// NoChunk disables automatic splitting of windows >24 months into
	// sequential sub-requests.
	NoChunk bool
	// NoSlice disables automatic slicing of /select requests that hit the row cap.
	NoSlice bool

	// Progress receives auto-pagination / export status messages. Defaults
	// to stderr. ProgressInterval defaults to CostsDefaultProgressInterval.
	Progress         func(msg string)
	ProgressInterval time.Duration

	// RequestEditors are applied to every underlying API call.
	RequestEditors []RequestEditorFn
}

// BillAnalysisCostsQueryResponse is the merged result of `bill-analysis costs query`
// Can be a result of one or more API calls
type BillAnalysisCostsQueryResponse struct {
	Rows            []BillAnalysisRow         `json:"rows"`
	RowsTruncated   bool                      `json:"rows_truncated"`
	EndpointUsed    BillAnalysisCostsEndpoint `json:"endpoint_used"`
	ChunksRequested int                       `json:"chunks_requested"`
	// Requests is the total number of cost API requests issued (including slices).
	Requests int `json:"requests"`

	// Export results (CostsEndpointExport only).
	ExportID     string            `json:"export_id,omitempty"`
	ExportStatus string            `json:"export_status,omitempty"`
	ExportURLs   map[string]string `json:"export_urls,omitempty"`
}

// BillAnalysisCostsQuery is the curated extension of the generated Bill Analysis
// costs endpoints (aggregated, select, export select). It routes between them,
// chunks long periods, slices oversized /select results, and polls exports.
// It backs `bill-analysis costs query` in flexera-cli.
type BillAnalysisCostsQuery struct {
	Cost     BillAnalysisCostsQueryClient
	Resolver *BillingCenterResolver
}

// NewBillAnalysisCostsQuery constructs a BillAnalysisCostsQuery.
func NewBillAnalysisCostsQuery(cost BillAnalysisCostsQueryClient, resolver *BillingCenterResolver) *BillAnalysisCostsQuery {
	return &BillAnalysisCostsQuery{Cost: cost, Resolver: resolver}
}

// Query executes the request, resolving billing centers and chunking the
// period if needed, and returns merged rows.
func (q *BillAnalysisCostsQuery) Query(ctx context.Context, req BillAnalysisCostsQueryRequest) (*BillAnalysisCostsQueryResponse, error) {
	if q == nil || q.Cost == nil {
		return nil, fmt.Errorf("BillAnalysisCostsQuery not initialised")
	}
	if req.OrgID <= 0 {
		return nil, fmt.Errorf("OrgID is required")
	}
	if req.StartAt == "" || req.EndAt == "" {
		return nil, fmt.Errorf("StartAt and EndAt are required")
	}
	if req.Granularity == "" {
		req.Granularity = CostsGranularityMonth
	}

	// Resolve billing-center IDs if not supplied.
	bcIDs := req.BillingCenterIDs
	if len(bcIDs) == 0 {
		if q.Resolver == nil {
			return nil, fmt.Errorf("BillingCenterIDs empty and no resolver configured")
		}
		resolved, err := q.Resolver.TopLevelIDs(ctx, req.OrgID)
		if err != nil {
			return nil, fmt.Errorf("resolve billing centers: %w", err)
		}
		bcIDs = resolved
	}

	// Auto-route endpoint.
	ep := req.Endpoint
	if ep == "" {
		ep = CostsEndpointAuto
	}
	if req.Export {
		ep = CostsEndpointExport
	}
	if ep == CostsEndpointAuto {
		if req.LineItems || len(req.Dimensions) > CostsMaxAggregatedDimensions || dimensionsContain(req.Dimensions, "resource_id") {
			ep = CostsEndpointSelect
		} else {
			ep = CostsEndpointAggregated
		}
	}

	if req.LineItems && ep == CostsEndpointAggregated {
		return nil, fmt.Errorf("LineItems cannot be used with the aggregated endpoint")
	}

	// Default metrics
	metrics := req.Metrics
	if len(metrics) == 0 {
		metrics = []string{"cost_amortized_unblended_adj"}
	}

	// Export is a single async job per period; never split it (1/day limit).
	noChunk := req.NoChunk || ep == CostsEndpointExport
	chunks, err := splitPeriod(req.StartAt, req.EndAt, req.Granularity, noChunk)
	if err != nil {
		return nil, err
	}

	out := &BillAnalysisCostsQueryResponse{
		EndpointUsed:    ep,
		ChunksRequested: len(chunks),
		Rows:            nil,
	}

	run := &costRun{h: q, req: req, bcIDs: bcIDs, metrics: metrics, out: out}
	if ep == CostsEndpointExport {
		if err := run.export(ctx, chunks[0].start, chunks[0].end); err != nil {
			return nil, err
		}
		return out, nil
	}

	stop := run.startProgress(ctx)
	defer stop()
	for _, c := range chunks {
		rows, truncated, err := run.query(ctx, ep, c.start, c.end)
		if err != nil {
			return nil, fmt.Errorf("chunk %s..%s: %w", c.start, c.end, err)
		}
		out.Rows = append(out.Rows, rows...)
		if truncated {
			out.RowsTruncated = true
		}
	}
	return out, nil
}

// costBody is the union of the JSON request-body fields accepted by the
// aggregated, select and export endpoints.
type costBody struct {
	BillingCenterIDs               []string                     `json:"billing_center_ids"`
	StartAt                        string                       `json:"start_at"`
	EndAt                          string                       `json:"end_at"`
	Granularity                    BillAnalysisCostsGranularity `json:"granularity"`
	Dimensions                     []string                     `json:"dimensions"`
	Metrics                        []string                     `json:"metrics"`
	Filter                         *BillAnalysisFilterV1        `json:"filter"`
	Summarized                     *bool                        `json:"summarized"`
	Limit                          *int64                       `json:"limit"`
	PeriodType                     string                       `json:"period_type"`
	AdjDimensionGranularity        string                       `json:"adj_dimension_granularity"`
	AggregateToRequestedDimensions *bool                        `json:"aggregate_to_requested_dimensions"`
}

// QueryWithBody is the raw-body variant of Query. The JSON body uses the
// same fields as the generated ...WithBody endpoints; routing, slicing and
// export behavior come from opts (Endpoint, Export, ...). Only the
// request-body-derived fields of opts are overwritten.
func (q *BillAnalysisCostsQuery) QueryWithBody(ctx context.Context, orgID int, contentType string, body io.Reader, opts BillAnalysisCostsQueryRequest) (*BillAnalysisCostsQueryResponse, error) {
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return nil, fmt.Errorf("unsupported content type %q: only JSON is supported", contentType)
	}
	var b costBody
	if err := json.NewDecoder(body).Decode(&b); err != nil {
		return nil, fmt.Errorf("decode cost request body: %w", err)
	}
	opts.OrgID = orgID
	opts.BillingCenterIDs = b.BillingCenterIDs
	opts.StartAt, opts.EndAt = b.StartAt, b.EndAt
	opts.Granularity = b.Granularity
	opts.Dimensions = b.Dimensions
	opts.Metrics = b.Metrics
	opts.Filter = b.Filter
	opts.Summarized = b.Summarized
	opts.Limit = b.Limit
	opts.PeriodType = b.PeriodType
	opts.AdjDimensionGranularity = b.AdjDimensionGranularity
	if b.AggregateToRequestedDimensions != nil {
		opts.LineItems = !*b.AggregateToRequestedDimensions
	}
	return q.Query(ctx, opts)
}

// costRun holds the state for one Query invocation.
type costRun struct {
	h       *BillAnalysisCostsQuery
	req     BillAnalysisCostsQueryRequest
	bcIDs   []string
	metrics []string
	out     *BillAnalysisCostsQueryResponse

	mu       sync.Mutex
	requests int
	status   string
}

func (r *costRun) progress(msg string) {
	if r.req.Progress != nil {
		r.req.Progress(msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

func (r *costRun) setStatus(s string) {
	r.mu.Lock()
	r.status = s
	r.mu.Unlock()
}

func (r *costRun) count() {
	r.mu.Lock()
	r.requests++
	r.out.Requests = r.requests
	r.mu.Unlock()
}

// startProgress emits periodic status messages while auto-pagination is
// active. Nothing is printed unless setStatus has been called.
func (r *costRun) startProgress(ctx context.Context) func() {
	interval := r.req.ProgressInterval
	if interval <= 0 {
		interval = CostsDefaultProgressInterval
	}
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				r.mu.Lock()
				s := r.status
				r.mu.Unlock()
				if s != "" {
					r.progress("cost auto-pagination in progress: " + s)
				}
			}
		}
	}()
	return func() { close(done) }
}

func (r *costRun) query(ctx context.Context, ep BillAnalysisCostsEndpoint, start, end string) ([]BillAnalysisRow, bool, error) {
	switch ep {
	case CostsEndpointAggregated:
		return r.aggregated(ctx, start, end, r.req.Dimensions, r.req.Filter, r.req.Limit)
	case CostsEndpointSelect:
		rows, truncated, err := r.selectRows(ctx, start, end, r.req.Filter)
		if err != nil || !truncated || r.req.NoSlice || r.userLimited() {
			return rows, truncated, err
		}
		r.progress(fmt.Sprintf("cost query %s..%s hit the %d row limit; auto-paginating by slicing on dimension values", start, end, r.selectLimit()))
		return r.slice(ctx, start, end, r.req.Filter, rows)
	default:
		return nil, false, fmt.Errorf("unsupported endpoint %q", ep)
	}
}

// userLimited reports whether the caller asked for fewer rows than the API cap;
// in that case a full page is the requested result, not truncation.
func (r *costRun) userLimited() bool {
	return r.selectLimit() < CostsMaxSelectRows
}

func (r *costRun) selectLimit() int64 {
	if r.req.Limit != nil && *r.req.Limit > 0 && *r.req.Limit < CostsMaxSelectRows {
		return *r.req.Limit
	}
	return CostsMaxSelectRows
}

func (r *costRun) aggregated(ctx context.Context, start, end string, dims []string, filter *BillAnalysisFilterV1, limit *int64) ([]BillAnalysisRow, bool, error) {
	gran := BillAnalysisAggregatedRequestBodyGranularity(r.req.Granularity)
	body := BillAnalysisAggregatedRequestBody{
		BillingCenterIds: r.bcIDs,
		StartAt:          start,
		EndAt:            end,
		Granularity:      &gran,
		Metrics:          r.metrics,
		Summarized:       r.req.Summarized,
		Filter:           filter,
		Limit:            limit,
	}
	if r.req.PeriodType != "" {
		pt := BillAnalysisAggregatedRequestBodyPeriodType(r.req.PeriodType)
		body.PeriodType = &pt
	}
	if len(dims) > 0 {
		d := append([]string{}, dims...)
		body.Dimensions = &d
	}
	r.count()
	resp, err := r.h.Cost.BillAnalysisCostsAggregatedWithResponse(ctx, int64(r.req.OrgID), body, r.req.RequestEditors...)
	if err != nil {
		return nil, false, err
	}
	return unpackResult(resp.HTTPResponse, resp.Body)
}

// selectRows returns the rows and whether the response hit the row cap.
func (r *costRun) selectRows(ctx context.Context, start, end string, filter *BillAnalysisFilterV1) ([]BillAnalysisRow, bool, error) {
	sgran := BillAnalysisSelectRequestBodyGranularity(r.req.Granularity)
	limit := r.selectLimit()
	body := BillAnalysisSelectRequestBody{
		BillingCenterIds: r.bcIDs,
		StartAt:          start,
		EndAt:            end,
		Granularity:      &sgran,
		Metrics:          r.metrics,
		Filter:           filter,
		Dimensions:       append([]string{}, r.req.Dimensions...),
		Limit:            limit,
	}
	if r.req.PeriodType != "" {
		pt := BillAnalysisSelectRequestBodyPeriodType(r.req.PeriodType)
		body.PeriodType = &pt
	}
	r.count()
	resp, err := r.h.Cost.BillAnalysisCostsSelectWithResponse(ctx, int64(r.req.OrgID), body, r.req.RequestEditors...)
	if err != nil {
		return nil, false, err
	}
	rows, truncated, err := unpackResult(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, false, err
	}
	return rows, truncated || int64(len(rows)) >= limit, nil
}

// slice re-runs a truncated /select query as one request per value of a
// slicing dimension (not already used in the filter). A slice that is itself
// truncated is sliced again on the next dimension. If no dimension can split
// the data, the partial rows are returned with truncated=true.
func (r *costRun) slice(ctx context.Context, start, end string, filter *BillAnalysisFilterV1, partial []BillAnalysisRow) ([]BillAnalysisRow, bool, error) {
	used := filterDimensions(filter, map[string]bool{})
	for _, dim := range sliceDimensions {
		if used[strings.ToLower(dim)] {
			continue
		}
		values, hasEmpty, err := r.distinctValues(ctx, start, end, dim, filter)
		if err != nil {
			return nil, false, err
		}
		if len(values)+boolInt(hasEmpty) < 2 {
			continue
		}

		slices := make([]*BillAnalysisFilterV1, 0, len(values)+1)
		for _, v := range values {
			slices = append(slices, andFilter(filter, equalFilter(dim, v)))
		}
		if hasEmpty {
			// Rows with no value for the dimension: everything that matches none of the values.
			eqs := make([]BillAnalysisFilterV1, len(values))
			for i, v := range values {
				eqs[i] = *equalFilter(dim, v)
			}
			slices = append(slices, andFilter(filter, &BillAnalysisFilterV1{
				Type: BillAnalysisFilterV1TypeNot,
				Expression: &BillAnalysisFilterV1{
					Type:        BillAnalysisFilterV1TypeOr,
					Expressions: &eqs,
				},
			}))
		}

		var all []BillAnalysisRow
		truncated := false
		for i, f := range slices {
			r.setStatus(fmt.Sprintf("slicing by %s: %d/%d slices done, %d rows so far", dim, i, len(slices), len(all)))
			rows, trunc, err := r.selectRows(ctx, start, end, f)
			if err != nil {
				return nil, false, fmt.Errorf("slice %s (%d/%d): %w", dim, i+1, len(slices), err)
			}
			if trunc {
				rows, trunc, err = r.slice(ctx, start, end, f, rows)
				if err != nil {
					return nil, false, err
				}
			}
			all = append(all, rows...)
			truncated = truncated || trunc
		}
		r.setStatus("")
		r.progress(fmt.Sprintf("cost auto-pagination by %s complete: %d slices, %d rows", dim, len(slices), len(all)))
		return all, truncated, nil
	}
	return partial, true, nil
}

// distinctValues lists the values of dim under filter using the cheap
// aggregated endpoint.
func (r *costRun) distinctValues(ctx context.Context, start, end, dim string, filter *BillAnalysisFilterV1) ([]string, bool, error) {
	rows, _, err := r.aggregated(ctx, start, end, []string{dim}, filter, nil)
	if err != nil {
		return nil, false, fmt.Errorf("list %s values: %w", dim, err)
	}
	seen := map[string]bool{}
	var values []string
	hasEmpty := false
	for _, row := range rows {
		v := row.Dimensions[dim]
		if v == "" {
			hasEmpty = true
			continue
		}
		if !seen[v] {
			seen[v] = true
			values = append(values, v)
		}
	}
	sort.Strings(values)
	return values, hasEmpty, nil
}

func (r *costRun) export(ctx context.Context, start, end string) error {
	gran := BillAnalysisExportSelectRequestBodyGranularity(r.req.Granularity)
	aggregate := !r.req.LineItems
	body := BillAnalysisExportSelectRequestBody{
		BillingCenterIds:               r.bcIDs,
		StartAt:                        start,
		EndAt:                          end,
		Granularity:                    &gran,
		Metrics:                        r.metrics,
		Filter:                         r.req.Filter,
		Dimensions:                     append([]string{}, r.req.Dimensions...),
		AggregateToRequestedDimensions: &aggregate,
	}
	if r.req.PeriodType != "" {
		pt := BillAnalysisExportSelectRequestBodyPeriodType(r.req.PeriodType)
		body.PeriodType = &pt
	}
	if r.req.AdjDimensionGranularity != "" {
		a := BillAnalysisExportSelectRequestBodyAdjDimensionGranularity(r.req.AdjDimensionGranularity)
		body.AdjDimensionGranularity = &a
	}
	r.count()
	resp, err := r.h.Cost.BillAnalysisCostsExportSelectWithResponse(ctx, int64(r.req.OrgID), body, r.req.RequestEditors...)
	if err != nil {
		return err
	}
	if sc := statusCode(resp.HTTPResponse); sc != http.StatusOK && sc != http.StatusAccepted {
		return fmt.Errorf("cost export API returned status %d: %s", sc, string(resp.Body))
	}
	started := resp.JSON200
	if started == nil {
		started = resp.JSON202
	}
	if started == nil || started.ExportId == "" {
		return fmt.Errorf("cost export API returned no export id: %s", string(resp.Body))
	}
	r.out.ExportID = started.ExportId
	r.out.ExportStatus = string(BillAnalysisExportSelectStatusResultStatusInProgress)
	r.progress(fmt.Sprintf("cost export %s started (exports are limited to 1 per day per org)", started.ExportId))
	if r.req.ExportNoWait {
		return nil
	}

	poll := r.req.ExportPollInterval
	if poll <= 0 {
		poll = CostsDefaultExportPollInterval
	}
	interval := r.req.ProgressInterval
	if interval <= 0 {
		interval = CostsDefaultProgressInterval
	}
	began := time.Now()
	lastReport := began
	for {
		r.count()
		st, err := r.h.Cost.BillAnalysisCostsExportSelectStatusWithResponse(ctx, int64(r.req.OrgID), started.ExportId, r.req.RequestEditors...)
		if err != nil {
			return fmt.Errorf("export %s status: %w", started.ExportId, err)
		}
		if sc := statusCode(st.HTTPResponse); sc != http.StatusOK && sc != http.StatusAccepted {
			return fmt.Errorf("export %s status API returned status %d: %s", started.ExportId, sc, string(st.Body))
		}
		res := st.JSON200
		if res == nil {
			res = st.JSON202
		}
		if res != nil {
			r.out.ExportStatus = string(res.Status)
			switch res.Status {
			case BillAnalysisExportSelectStatusResultStatusDone:
				if res.Urls != nil {
					r.out.ExportURLs = *res.Urls
				}
				return nil
			case BillAnalysisExportSelectStatusResultStatusFailed:
				return fmt.Errorf("cost export %s failed", started.ExportId)
			}
		}
		if time.Since(lastReport) >= interval {
			lastReport = time.Now()
			r.progress(fmt.Sprintf("cost export %s still in progress (%s elapsed)", started.ExportId, time.Since(began).Round(time.Second)))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

func statusCode(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func equalFilter(dim, value string) *BillAnalysisFilterV1 {
	return &BillAnalysisFilterV1{Type: BillAnalysisFilterV1TypeEqual, Dimension: &dim, Value: &value}
}

// andFilter returns base AND extra, or just extra when base is nil.
func andFilter(base, extra *BillAnalysisFilterV1) *BillAnalysisFilterV1 {
	if base == nil {
		return extra
	}
	return &BillAnalysisFilterV1{
		Type:        BillAnalysisFilterV1TypeAnd,
		Expressions: &[]BillAnalysisFilterV1{*base, *extra},
	}
}

// filterDimensions collects every dimension referenced by the filter tree.
func filterDimensions(f *BillAnalysisFilterV1, into map[string]bool) map[string]bool {
	if f == nil {
		return into
	}
	if f.Dimension != nil {
		into[strings.ToLower(*f.Dimension)] = true
	}
	filterDimensions(f.Expression, into)
	if f.Expressions != nil {
		for i := range *f.Expressions {
			filterDimensions(&(*f.Expressions)[i], into)
		}
	}
	return into
}

func unpackResult(httpResp *http.Response, body []byte) ([]BillAnalysisRow, bool, error) {
	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return nil, false, fmt.Errorf("cost API returned status %d: %s", status, string(body))
	}
	var result BillAnalysisAnalyticsQueryResult
	err := json.Unmarshal(body, &result)
	if err != nil {
		return nil, false, fmt.Errorf("cost API: unmarshal: %w", err)
	}
	truncated := false
	if result.RowsTruncated != nil {
		truncated = *result.RowsTruncated
	}
	return result.Rows, truncated, nil
}

func dimensionsContain(dims []string, want string) bool {
	for _, d := range dims {
		if strings.EqualFold(d, want) {
			return true
		}
	}
	return false
}

// chunk represents one [start, end) period bucket.
type chunk struct {
	start string
	end   string
}

// splitPeriod returns one chunk if noChunk is true or the period fits within
// CostsMaxPeriodMonths; otherwise returns a sequence of ≤CostsMaxPeriodMonths chunks.
func splitPeriod(start, end string, granularity BillAnalysisCostsGranularity, noChunk bool) ([]chunk, error) {
	startT, endT, layout, err := parsePeriod(start, end, granularity)
	if err != nil {
		return nil, err
	}
	if !endT.After(startT) {
		return nil, fmt.Errorf("EndAt %q must be after StartAt %q", end, start)
	}

	months := monthsBetween(startT, endT)
	if noChunk || months <= CostsMaxPeriodMonths {
		return []chunk{{start: start, end: end}}, nil
	}

	out := []chunk{}
	cursor := startT
	for cursor.Before(endT) {
		next := cursor.AddDate(0, CostsMaxPeriodMonths, 0)
		if next.After(endT) {
			next = endT
		}
		out = append(out, chunk{
			start: cursor.Format(layout),
			end:   next.Format(layout),
		})
		cursor = next
	}
	return out, nil
}

func parsePeriod(start, end string, granularity BillAnalysisCostsGranularity) (time.Time, time.Time, string, error) {
	layout := "2006-01"
	if granularity == CostsGranularityDay {
		layout = "2006-01-02"
	}
	st, err := time.Parse(layout, start)
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("parse StartAt %q (expected %s): %w", start, layout, err)
	}
	en, err := time.Parse(layout, end)
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("parse EndAt %q (expected %s): %w", end, layout, err)
	}
	return st, en, layout, nil
}

// monthsBetween returns the integer number of calendar months between
// start (inclusive) and end (exclusive). Rounds up on partial months so a
// "25 month + 1 day" window still splits.
func monthsBetween(start, end time.Time) int {
	y := end.Year() - start.Year()
	m := int(end.Month()) - int(start.Month())
	months := y*12 + m
	if end.Day() > start.Day() {
		months++
	}
	if months < 0 {
		return 0
	}
	return months
}
