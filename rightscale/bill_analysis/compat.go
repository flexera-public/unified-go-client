// Package billanalysis provides a Go client for the Flexera Optima Bill Analysis API.
// The client is generated from a separately maintained curated bill_analysis spec.
//
// This file provides backward-compatible type aliases so that existing callers
// (cost.go, anomaly/*, service/optima, flexera-cli) require zero import-path or
// type-name changes. The aliases map the old oapi-codegen name conventions to
// the names produced by the curated spec.
//
// New code should prefer the generated names directly.
package billanalysis

import "encoding/json"

// ── Request body type aliases ─────────────────────────────────────────────────

// AggregatedRequestBody is the request body for POST /costs/aggregated.
type AggregatedRequestBody = CostsAggregatedRequestBody

// AggregatedRequestBodyGranularity is the granularity enum for aggregated cost queries.
type AggregatedRequestBodyGranularity = CostsAggregatedRequestBodyGranularity

// SelectRequestBody is the request body for POST /costs/select.
type SelectRequestBody = CostsSelectRequestBody

// SelectRequestBodyGranularity is the granularity enum for select cost queries.
type SelectRequestBodyGranularity = CostsSelectRequestBodyGranularity

// ReportRequestBody is the request body for POST /anomalies/report.
type ReportRequestBody = AnomaliesReportRequestBody

// ReportRequestBodyGranularity is the granularity enum for anomaly reports.
type ReportRequestBodyGranularity = AnomaliesReportRequestBodyGranularity

// ReportRequestBodyDetectionMethod is the detection method enum for anomaly reports.
type ReportRequestBodyDetectionMethod = AnomaliesReportRequestBodyDetectionMethod

// ── Filter type aliases ───────────────────────────────────────────────────────

// FilterV1 is the filter expression type used in cost and anomaly queries.
type FilterV1 = FilterV1RequestBody

// FilterV1Type is the discriminator enum for FilterV1.
type FilterV1Type = FilterV1RequestBodyType

// FilterV1Type constants for building filter expressions.
const (
	FilterV1TypeAnd       FilterV1RequestBodyType = FilterV1RequestBodyTypeAnd
	FilterV1TypeEqual     FilterV1RequestBodyType = FilterV1RequestBodyTypeEqual
	FilterV1TypeNot       FilterV1RequestBodyType = FilterV1RequestBodyTypeNot
	FilterV1TypeOr        FilterV1RequestBodyType = FilterV1RequestBodyTypeOr
	FilterV1TypeSubstring FilterV1RequestBodyType = FilterV1RequestBodyTypeSubstring
)

// ── Response type aliases ─────────────────────────────────────────────────────

// Row is a single row in a cost query result.
type Row = RowResponseBody

// AnalyticsQueryResult is the result of a successful cost aggregated or select
// query. It is used by the CostClient interface in cost.go and by the anomaly
// package. The rowsTruncated field indicates whether the server limited the
// result set.
//
// Both the 200 OK and 202 Accepted responses for /costs/aggregated and
// /costs/select carry this shape; callers can parse resp.Body into this type
// regardless of the exact status code.
type AnalyticsQueryResult struct {
	Rows          []RowResponseBody `json:"rows"`
	RowsTruncated *bool             `json:"rowsTruncated,omitempty"`
}

// ParseAnalyticsQueryResult unmarshals the raw response body into an
// AnalyticsQueryResult. Returns nil if body is empty or status is not 200/202.
func ParseAnalyticsQueryResult(body []byte, statusCode int) (*AnalyticsQueryResult, error) {
	if statusCode != 200 && statusCode != 202 {
		return nil, nil
	}
	var r AnalyticsQueryResult
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
