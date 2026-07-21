package anomaly

import "time"

// These types model the bill_analysis response shapes that are not
// emitted by oapi-codegen (it only generates the request side). Copied
// from cli/go-flexera-mcp/server/missing_types.go so this package is
// self-contained.

type rowResponseBody struct {
	Dimensions map[string]string  `json:"dimensions,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Timestamp  string             `json:"timestamp,omitempty"`
}

type costsAggregatedOKResponseBody struct {
	Rows []rowResponseBody `json:"rows,omitempty"`
}

type anomalyAnnotation map[string]float64

type anomalyDatum struct {
	Anomalous   bool               `json:"anomalous,omitempty"`
	Annotations *anomalyAnnotation `json:"annotations,omitempty"`
	Date        time.Time          `json:"date,omitempty"`
	Value       float64            `json:"value,omitempty"`
}

type anomalyTimeSeries struct {
	Data       []anomalyDatum    `json:"data,omitempty"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
}

type anomalyValue struct {
	TimeSeries anomalyTimeSeries `json:"timeSeries,omitempty"`
}

type anomaliesReportResponseBody struct {
	Values *[]anomalyValue `json:"values,omitempty"`
}
