package anomaly

import (
	"time"

	billanalysis "github.com/flexera-public/unified-go-client/rightscale/bill_analysis"
)

// Input describes the parameters accepted by the anomaly investigator.
// All fields except OrgID are optional and have sensible defaults.
type Input struct {
	OrgID           int      `json:"org_id"`
	Granularity     *string  `json:"granularity,omitempty"`
	Recency         *string  `json:"recency,omitempty"`
	Discovery       *bool    `json:"discovery,omitempty"`
	LookbackPeriod  *string  `json:"lookback_period,omitempty"`
	BillingCenterID *string  `json:"billing_center_id,omitempty"`
	CostThreshold   *float64 `json:"cost_threshold,omitempty"`
	IncreaseOnly    *bool    `json:"increase_only,omitempty"`
}

// Output is the structured analysis returned by the investigator.
type Output struct {
	DimensionAnalysis []DimensionAnomalyAnalysis `json:"anomaly_analysis"`
	Summary           Summary                    `json:"summary"`
	DiscoveryResult   *DiscoveryModeResult       `json:"discovery_result,omitempty"`
}

// Summary provides a high-level overview of the analysis run.
type Summary struct {
	TotalAnomalies       int     `json:"total_anomalies"`
	SignificantAnomalies int     `json:"significant_anomalies"`
	TotalImpact          float64 `json:"total_impact"`
	AnalysisPeriod       string  `json:"analysis_period"`
	DetectionMethod      string  `json:"detection_method"`
	CostThreshold        string  `json:"cost_threshold"`
	OrgID                int     `json:"org_id"`
	Granularity          string  `json:"granularity"`
	AnalysisDate         string  `json:"analysis_date"`
	RecencyWindow        string  `json:"recency_window"`
	BaselineLookback     string  `json:"baseline_lookback"`
	DiscoveryEnabled     bool    `json:"discovery_enabled"`
}

// DiscoveryModeResult describes the outcome of discovery mode.
type DiscoveryModeResult struct {
	PhasesExecuted        int     `json:"phases_executed"`
	RelaxedRecency        string  `json:"relaxed_recency,omitempty"`
	RelaxedThreshold      float64 `json:"relaxed_threshold,omitempty"`
	OriginalThreshold     float64 `json:"original_threshold"`
	ThresholdMetNotRecent int     `json:"threshold_met_not_recent"`
	RecentNotThreshold    int     `json:"recent_not_threshold"`
}

// DimensionAnomalyAnalysis groups anomalies by the dimension category
// that produced them (e.g. service, region).
type DimensionAnomalyAnalysis struct {
	Category       string             `json:"category"`
	Description    string             `json:"description"`
	AnomaliesFound int                `json:"anomalies_found"`
	TopAnomalies   []DimensionAnomaly `json:"top_anomalies"`
	FinOpsContext  string             `json:"finops_context"`
}

// DimensionAnomaly is one detected anomaly inside a dimension category.
type DimensionAnomaly struct {
	DimensionValue    string  `json:"dimension_value"`
	Date              string  `json:"date"`
	ActualCost        float64 `json:"actual_cost"`
	ExpectedCost      float64 `json:"expected_cost"`
	Deviation         float64 `json:"deviation"`
	DeviationPercent  float64 `json:"deviation_percent"`
	DiscoveryCategory *string `json:"discovery_category,omitempty"`
	DiscoveryContext  *string `json:"discovery_context,omitempty"`
}

// dimensionConfig captures the multi-dimensional analysis matrix used by
// the investigator. Each entry produces an independent API call.
type dimensionConfig struct {
	Dimensions    []string
	Filter        *billanalysis.FilterV1
	Category      string
	Description   string
	FinOpsContext string
}

// Unused but exported types kept for future symmetry with the MCP shape.
// They mirror what the MCP-era code surfaced but were not actually emitted
// in the structured Output. Kept here so downstream consumers can still
// reference them if they previously imported the symbol.
type FinOpsImpactAssessment struct {
	BudgetRisk            string   `json:"budget_risk"`
	ForecastImpact        string   `json:"forecast_impact"`
	RequiresInvestigation bool     `json:"requires_investigation"`
	PotentialSavings      *float64 `json:"potential_savings,omitempty"`
	KeyFindings           []string `json:"key_findings"`
}

// staticNow is overridable in tests to make recency math deterministic.
var staticNow = func() time.Time { return time.Now().UTC() }
