package flexera

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	ba "github.com/flexera-public/unified-go-client/rightscale/bill_analysis"
)

// CostClient is the subset of the bill_analysis client the helper depends on.
type CostClient interface {
	CostsAggregatedWithResponse(
		ctx context.Context,
		org int,
		params *ba.CostsAggregatedParams,
		body ba.CostsAggregatedJSONRequestBody,
		reqEditors ...ba.RequestEditorFn,
	) (*ba.CostsAggregatedResponse, error)
	CostsSelectWithResponse(
		ctx context.Context,
		org int,
		params *ba.CostsSelectParams,
		body ba.CostsSelectJSONRequestBody,
		reqEditors ...ba.RequestEditorFn,
	) (*ba.CostsSelectResponse, error)
}

// Endpoint selects which underlying cost endpoint to use.
type Endpoint string

const (
	EndpointAuto       Endpoint = "auto"
	EndpointAggregated Endpoint = "aggregated"
	EndpointSelect     Endpoint = "select"
)

// Granularity is "month" or "day".
type Granularity string

const (
	GranularityMonth Granularity = "month"
	GranularityDay   Granularity = "day"
)

// MaxPeriodMonths is the upstream API limit on a single cost request window.
const MaxPeriodMonths = 24

// Request describes a logical cost query. It is converted into one or more
// bill_analysis API calls by GetCost.
type Request struct {
	OrgID            int
	BillingCenterIDs []string // empty → resolver fills in top-level set

	StartAt     string // YYYY-MM (month) or YYYY-MM-DD (day); inclusive
	EndAt       string // YYYY-MM (month) or YYYY-MM-DD (day); exclusive
	Granularity Granularity

	Dimensions []string
	Metrics    []string // empty → ["cost_amortized_unblended_adj"]
	Filter     *ba.FilterV1
	Summarized *bool

	// Endpoint selects /aggregated, /select, or auto (default). Auto picks
	// /select iff Dimensions contains "resource_id".
	Endpoint Endpoint

	// NoChunk disables automatic splitting of windows >24 months into
	// sequential sub-requests.
	NoChunk bool
}

// CostResponse is the merged result of one or more API calls.
type CostResponse struct {
	Rows            []ba.Row
	RowsTruncated   bool
	EndpointUsed    Endpoint
	ChunksRequested int
}

// Helper bundles the cost client + billing-center resolver for repeated use.
type Helper struct {
	Cost     CostClient
	Resolver *BillingCenterResolver
}

// New constructs a Helper.
func New(cost CostClient, resolver *BillingCenterResolver) *Helper {
	return &Helper{Cost: cost, Resolver: resolver}
}

// GetCost executes the request, resolving billing centers and chunking the
// period if needed, and returns merged rows.
func (h *Helper) GetCost(ctx context.Context, req Request) (*CostResponse, error) {
	if h == nil || h.Cost == nil {
		return nil, fmt.Errorf("finopscost.Helper not initialised")
	}
	if req.OrgID <= 0 {
		return nil, fmt.Errorf("OrgID is required")
	}
	if req.StartAt == "" || req.EndAt == "" {
		return nil, fmt.Errorf("StartAt and EndAt are required")
	}
	if req.Granularity == "" {
		req.Granularity = GranularityMonth
	}

	// Resolve billing-center IDs if not supplied.
	bcIDs := req.BillingCenterIDs
	if len(bcIDs) == 0 {
		if h.Resolver == nil {
			return nil, fmt.Errorf("BillingCenterIDs empty and no resolver configured")
		}
		resolved, err := h.Resolver.TopLevelIDs(ctx, req.OrgID)
		if err != nil {
			return nil, fmt.Errorf("resolve billing centers: %w", err)
		}
		bcIDs = resolved
	}

	// Auto-route endpoint.
	ep := req.Endpoint
	if ep == "" {
		ep = EndpointAuto
	}
	if ep == EndpointAuto {
		if dimensionsContain(req.Dimensions, "resource_id") {
			ep = EndpointSelect
		} else {
			ep = EndpointAggregated
		}
	}

	// Default metrics
	metrics := req.Metrics
	if len(metrics) == 0 {
		metrics = []string{"cost_amortized_unblended_adj"}
	}

	// Build period chunks.
	chunks, err := splitPeriod(req.StartAt, req.EndAt, req.Granularity, req.NoChunk)
	if err != nil {
		return nil, err
	}

	out := &CostResponse{
		EndpointUsed:    ep,
		ChunksRequested: len(chunks),
		Rows:            nil,
	}

	for _, c := range chunks {
		rows, truncated, err := h.callOnce(ctx, ep, req, bcIDs, metrics, c.start, c.end)
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

func (h *Helper) callOnce(
	ctx context.Context,
	ep Endpoint,
	req Request,
	bcIDs []string,
	metrics []string,
	start, end string,
) ([]ba.Row, bool, error) {
	gran := ba.AggregatedRequestBodyGranularity(req.Granularity)

	switch ep {
	case EndpointAggregated:
		body := ba.AggregatedRequestBody{
			BillingCenterIds: bcIDs,
			StartAt:          start,
			EndAt:            end,
			Granularity:      &gran,
			Metrics:          metrics,
			Summarized:       req.Summarized,
			Filter:           req.Filter,
		}
		if len(req.Dimensions) > 0 {
			d := append([]string{}, req.Dimensions...)
			body.Dimensions = &d
		}
		resp, err := h.Cost.CostsAggregatedWithResponse(ctx, int(req.OrgID), nil, body)
		if err != nil {
			return nil, false, err
		}
		return unpackResult(resp.HTTPResponse, resp.Body)

	case EndpointSelect:
		sgran := ba.SelectRequestBodyGranularity(req.Granularity)
		dims := append([]string{}, req.Dimensions...)
		body := ba.SelectRequestBody{
			BillingCenterIds: bcIDs,
			StartAt:          start,
			EndAt:            end,
			Granularity:      &sgran,
			Metrics:          metrics,
			Filter:           req.Filter,
			Dimensions:       dims,
		}
		resp, err := h.Cost.CostsSelectWithResponse(ctx, int(req.OrgID), nil, body)
		if err != nil {
			return nil, false, err
		}
		return unpackResult(resp.HTTPResponse, resp.Body)

	default:
		return nil, false, fmt.Errorf("unsupported endpoint %q", ep)
	}
}

func unpackResult(httpResp *http.Response, body []byte) ([]ba.Row, bool, error) {
	status := 0
	if httpResp != nil {
		status = httpResp.StatusCode
	}
	result, err := ba.ParseAnalyticsQueryResult(body, status)
	if err != nil {
		return nil, false, fmt.Errorf("cost API: unmarshal: %w", err)
	}
	if result == nil {
		return nil, false, fmt.Errorf("cost API returned status %d: %s", status, string(body))
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
// MaxPeriodMonths; otherwise returns a sequence of ≤MaxPeriodMonths chunks.
func splitPeriod(start, end string, granularity Granularity, noChunk bool) ([]chunk, error) {
	startT, endT, layout, err := parsePeriod(start, end, granularity)
	if err != nil {
		return nil, err
	}
	if !endT.After(startT) {
		return nil, fmt.Errorf("EndAt %q must be after StartAt %q", end, start)
	}

	months := monthsBetween(startT, endT)
	if noChunk || months <= MaxPeriodMonths {
		return []chunk{{start: start, end: end}}, nil
	}

	out := []chunk{}
	cursor := startT
	for cursor.Before(endT) {
		next := cursor.AddDate(0, MaxPeriodMonths, 0)
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

func parsePeriod(start, end string, granularity Granularity) (time.Time, time.Time, string, error) {
	layout := "2006-01"
	if granularity == GranularityDay {
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
