package anomaly

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	billanalysis "github.com/flexera-public/unified-go-client/rightscale/bill_analysis"
)

// BillingCenterResolver returns the top-level billing-center IDs for an
// org. The finopscost.BillingCenterResolver satisfies this interface;
// callers may also inject a static slice via StaticResolver.
type BillingCenterResolver interface {
	TopLevelIDs(ctx context.Context, orgID int) ([]string, error)
}

// StaticResolver is a trivial BillingCenterResolver that returns a fixed
// slice. Useful in tests and for callers that already resolved IDs.
type StaticResolver struct{ IDs []string }

func (s StaticResolver) TopLevelIDs(context.Context, int) ([]string, error) { return s.IDs, nil }

// DebugLogger is an optional callback for structured trace output. The
// MCP server's debugLog signature fits; pass nil to disable logging.
type DebugLogger func(ctx context.Context, format string, args ...any)

// Investigator runs the cost-anomaly investigation workflow. Construct
// one with New and call Invoke to execute.
type Investigator struct {
	Client   *billanalysis.ClientWithResponses
	Resolver BillingCenterResolver
	Debug    DebugLogger
}

// New constructs an Investigator. Resolver and debug may be nil; if
// resolver is nil and the input does not pin a billing-center ID, Invoke
// returns an error.
func New(client *billanalysis.ClientWithResponses, resolver BillingCenterResolver, debug DebugLogger) *Investigator {
	return &Investigator{Client: client, Resolver: resolver, Debug: debug}
}

// Name implements curated.Tool.
func (*Investigator) Name() string { return "anomaly-investigation" }

// Description implements curated.Tool.
func (*Investigator) Description() string {
	return "AI-powered cost anomaly detection across service/region/compute dimensions"
}

func (i *Investigator) logf(ctx context.Context, format string, args ...any) {
	if i.Debug != nil {
		i.Debug(ctx, format, args...)
	}
}

// Invoke runs the full investigation workflow.
func (i *Investigator) Invoke(ctx context.Context, in Input) (Output, error) {
	if i.Client == nil {
		return Output{}, fmt.Errorf("anomaly: bill_analysis client is nil")
	}

	// 1. Resolve granularity + recency (legacy lookback_period compat).
	granularity := "month"
	recencyStr := ""
	if in.LookbackPeriod != nil && *in.LookbackPeriod != "" && in.Granularity == nil {
		g, r, err := mapLegacyLookbackPeriod(*in.LookbackPeriod)
		if err != nil {
			return Output{}, err
		}
		granularity, recencyStr = g, r
	} else {
		if in.Granularity != nil && *in.Granularity != "" {
			granularity = *in.Granularity
		}
		if in.Recency != nil && *in.Recency != "" {
			recencyStr = *in.Recency
		}
	}
	if granularity != "month" && granularity != "day" {
		return Output{}, fmt.Errorf("invalid granularity: %q (must be 'month' or 'day')", granularity)
	}
	recency, err := parseRecency(recencyStr, granularity)
	if err != nil {
		return Output{}, err
	}

	// 2. Baseline window + recency cutoff.
	startAt, endAt, windowSize := getBaselineLookback(granularity)
	cutoff := recencyCutoff(recency, granularity)

	discoveryEnabled := in.Discovery != nil && *in.Discovery

	// 3. Determine billing centers.
	var billingCenterIDs []string
	if in.BillingCenterID != nil && *in.BillingCenterID != "" {
		billingCenterIDs = []string{*in.BillingCenterID}
	} else {
		if i.Resolver == nil {
			return Output{}, fmt.Errorf("billing_center_id not provided and no BillingCenterResolver was configured")
		}
		bcs, err := i.Resolver.TopLevelIDs(ctx, in.OrgID)
		if err != nil {
			return Output{}, fmt.Errorf("resolve billing centers: %w", err)
		}
		if len(bcs) == 0 {
			return Output{}, fmt.Errorf("no top-level billing centers found")
		}
		billingCenterIDs = bcs
	}

	// 4. Threshold (explicit > dynamic > static default).
	costThreshold := 1500.0
	if granularity == "day" {
		costThreshold = 50.0
	}
	if in.CostThreshold != nil {
		costThreshold = *in.CostThreshold
	} else {
		if dyn, err := i.calcDynamicThreshold(ctx, in.OrgID, billingCenterIDs, startAt, endAt, granularity); err != nil {
			i.logf(ctx, "WARNING: dynamic threshold failed, using default: %v", err)
		} else {
			costThreshold = dyn
			i.logf(ctx, "Using dynamic cost threshold: $%.2f", costThreshold)
		}
	}

	increaseOnly := true
	if in.IncreaseOnly != nil {
		increaseOnly = *in.IncreaseOnly
	}

	i.logf(ctx, "Starting anomaly detection: granularity=%s, recency=%s, baseline=%s..%s, window=%d, threshold=$%.2f, cutoff=%s, discovery=%v",
		granularity, recency.Raw, startAt, endAt, windowSize, costThreshold, cutoff.Format(time.RFC3339), discoveryEnabled)

	// 5. Per-dimension detection loop.
	configs := getDimensionConfigs()
	rawByCategory := make(map[string][]DimensionAnomaly)
	detectionMethod := "ai_model"
	for _, cfg := range configs {
		anomalies, method, err := i.detectForDimension(ctx, in.OrgID, billingCenterIDs, startAt, endAt, granularity, windowSize, cfg, costThreshold, increaseOnly)
		if err != nil {
			i.logf(ctx, "WARNING: detect %s failed: %v", cfg.Category, err)
			continue
		}
		if method != "" {
			detectionMethod = method
		}
		if len(anomalies) > 0 {
			rawByCategory[cfg.Category] = anomalies
		}
	}

	// 6. Apply primary recency filter.
	dimensionAnalysis, total, significant, totalImpact := buildAnalysisFromAnomalies(rawByCategory, configs, cutoff, granularity, costThreshold)

	// 7. Discovery mode fallback.
	var discoveryResult *DiscoveryModeResult
	if total == 0 && discoveryEnabled {
		i.logf(ctx, "No anomalies in primary window; entering discovery mode")
		discoveryResult = runDiscoveryMode(rawByCategory, granularity, costThreshold, cutoff)
		if discoveryResult.ThresholdMetNotRecent > 0 || discoveryResult.RecentNotThreshold > 0 {
			dimensionAnalysis, total, significant, totalImpact = buildDiscoveryAnalysis(rawByCategory, configs, granularity, costThreshold, discoveryResult)
		}
	}

	baselineLookback := fmt.Sprintf("%d months", BaselineLookbackMonthly)
	if granularity == "day" {
		baselineLookback = fmt.Sprintf("%d days", BaselineLookbackDaily)
	}

	return Output{
		DimensionAnalysis: dimensionAnalysis,
		Summary: Summary{
			TotalAnomalies:       total,
			SignificantAnomalies: significant,
			TotalImpact:          totalImpact,
			AnalysisPeriod:       recency.Raw,
			DetectionMethod:      detectionMethod,
			CostThreshold:        fmt.Sprintf("%.2f", costThreshold),
			OrgID:                in.OrgID,
			AnalysisDate:         staticNow().Format(time.RFC3339),
			Granularity:          granularity,
			RecencyWindow:        recency.Raw,
			BaselineLookback:     baselineLookback,
			DiscoveryEnabled:     discoveryEnabled,
		},
		DiscoveryResult: discoveryResult,
	}, nil
}

// ---------- dynamic threshold ----------

func (i *Investigator) calcDynamicThreshold(ctx context.Context, orgID int, bcIDs []string, startAt, endAt, granularity string) (float64, error) {
	summarized := true
	body := billanalysis.AggregatedRequestBody{
		BillingCenterIds: bcIDs,
		StartAt:          startAt,
		EndAt:            endAt,
		Metrics:          []string{"cost_amortized_unblended_adj"},
		Summarized:       &summarized,
	}
	gran := billanalysis.AggregatedRequestBodyGranularity(granularity)
	body.Granularity = &gran
	body.Filter = &billanalysis.FilterV1{
		Type:      billanalysis.FilterV1TypeEqual,
		Dimension: strPtr("capability"),
		Value:     strPtr("csm"),
	}
	resp, err := i.Client.CostsAggregatedWithResponse(ctx, int64(orgID), body)
	if err != nil {
		return 0, fmt.Errorf("aggregated cost: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return 0, fmt.Errorf("aggregated cost: status %d: %s", resp.StatusCode(), string(resp.Body))
	}
	var parsed costsAggregatedOKResponseBody
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return 0, fmt.Errorf("aggregated cost unmarshal: %w", err)
	}
	if len(parsed.Rows) == 0 {
		return 0, fmt.Errorf("aggregated cost: no rows")
	}
	total := 0.0
	points := 0
	for _, row := range parsed.Rows {
		if v, ok := row.Metrics["cost_amortized_unblended_adj"]; ok {
			total += v
			points++
		}
	}
	if points == 0 {
		return 0, fmt.Errorf("aggregated cost: no metric values")
	}
	avg := total / float64(points)
	threshold := avg * 0.001
	minT := 10.0
	if granularity == "month" {
		minT = 100.0
	}
	if threshold < minT {
		threshold = minT
	}
	i.logf(ctx, "Dynamic threshold: total=$%.2f points=%d avg=$%.2f → $%.2f", total, points, avg, threshold)
	return threshold, nil
}

// ---------- per-dimension detection ----------

func (i *Investigator) detectForDimension(ctx context.Context, orgID int, bcIDs []string, startAt, endAt, granularity string, windowSize int64, cfg dimensionConfig, costThreshold float64, increaseOnly bool) ([]DimensionAnomaly, string, error) {
	detectionMethod := "ai_model"
	body := billanalysis.ReportRequestBody{
		BillingCenterIds:   bcIDs,
		StartAt:            startAt,
		EndAt:              endAt,
		Metric:             "cost_amortized_unblended_adj",
		WindowSize:         windowSize,
		StandardDeviations: 2.0,
		Dimensions:         &cfg.Dimensions,
	}
	gran := billanalysis.ReportRequestBodyGranularity(granularity)
	body.Granularity = &gran
	method := billanalysis.ReportRequestBodyDetectionMethod("ai_model")
	body.DetectionMethod = &method
	if cfg.Filter != nil {
		body.Filter = cfg.Filter
	}

	i.logf(ctx, "AI model detection for %s", cfg.Category)
	resp, err := i.Client.AnomaliesReportWithResponse(ctx, int64(orgID), body)
	if err != nil || resp.StatusCode() != http.StatusOK {
		i.logf(ctx, "AI model failed for %s, falling back to Bollinger: %v", cfg.Category, err)
		detectionMethod = "bollinger_band"
		bollinger := billanalysis.ReportRequestBodyDetectionMethod("bollinger_band")
		body.DetectionMethod = &bollinger
		body.WindowSize = 1
		body.StandardDeviations = 2.0
		resp, err = i.Client.AnomaliesReportWithResponse(ctx, int64(orgID), body)
		if err != nil {
			return nil, detectionMethod, fmt.Errorf("anomaly detection: %w", err)
		}
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, detectionMethod, fmt.Errorf("anomaly API status %d: %s", resp.StatusCode(), string(resp.Body))
	}

	var parsed anomaliesReportResponseBody
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, detectionMethod, fmt.Errorf("anomaly unmarshal: %w", err)
	}
	if parsed.Values == nil {
		return nil, detectionMethod, fmt.Errorf("anomaly response: nil values")
	}

	anomalies := make([]DimensionAnomaly, 0)
	var raw, filteredByIncrease, filteredByThreshold int
	for _, ts := range *parsed.Values {
		if len(ts.TimeSeries.Data) == 0 {
			continue
		}
		dimVal := formatDimensionValue(ts.TimeSeries.Dimensions)
		for _, dp := range ts.TimeSeries.Data {
			if !dp.Anomalous {
				continue
			}
			raw++
			actual := dp.Value
			expected := 0.0
			if dp.Annotations != nil {
				if exp, ok := (*dp.Annotations)["movingAverage"]; ok {
					expected = exp
				}
			}
			dev := actual - expected
			pct := 0.0
			if expected > 0 {
				pct = (dev / expected) * 100
			}
			if increaseOnly && dev < 0 {
				filteredByIncrease++
				continue
			}
			if dev < costThreshold && dev > -costThreshold {
				filteredByThreshold++
				continue
			}
			anomalies = append(anomalies, DimensionAnomaly{
				DimensionValue:   dimVal,
				Date:             dp.Date.Format(time.RFC3339),
				ActualCost:       actual,
				ExpectedCost:     expected,
				Deviation:        dev,
				DeviationPercent: pct,
			})
		}
	}
	i.logf(ctx, "Filter stats for %s: raw=%d, !increase=%d, !threshold=%d, kept=%d (threshold=$%.2f, increaseOnly=%v)",
		cfg.Category, raw, filteredByIncrease, filteredByThreshold, len(anomalies), costThreshold, increaseOnly)

	sort.Slice(anomalies, func(a, b int) bool {
		ai := anomalies[a].Deviation
		if ai < 0 {
			ai = -ai
		}
		bj := anomalies[b].Deviation
		if bj < 0 {
			bj = -bj
		}
		return ai > bj
	})
	return anomalies, detectionMethod, nil
}

// formatDimensionValue builds a stable "k=v;k=v" string from the
// dimensions map; keys are sorted alphabetically and empty values are
// skipped. Returns "All" for an empty map.
func formatDimensionValue(dims map[string]string) string {
	if len(dims) == 0 {
		return "All"
	}
	keys := make([]string, 0, len(dims))
	for k := range dims {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := ""
	for _, k := range keys {
		v := dims[k]
		if v == "" {
			continue
		}
		if result != "" {
			result += ";"
		}
		result += k + "=" + v
	}
	if result == "" {
		return "Unknown"
	}
	return result
}

// ---------- analysis builders + discovery ----------

func buildAnalysisFromAnomalies(raw map[string][]DimensionAnomaly, configs []dimensionConfig, cutoff time.Time, granularity string, costThreshold float64) (analysis []DimensionAnomalyAnalysis, total int, significant int, impact float64) {
	analysis = make([]DimensionAnomalyAnalysis, 0)
	for _, cfg := range configs {
		anomalies, ok := raw[cfg.Category]
		if !ok || len(anomalies) == 0 {
			continue
		}
		recent := filterAnomaliesByRecency(anomalies, cutoff, granularity)
		if len(recent) == 0 {
			continue
		}
		analysis = append(analysis, DimensionAnomalyAnalysis{
			Category:       cfg.Category,
			Description:    cfg.Description,
			AnomaliesFound: len(recent),
			TopAnomalies:   recent,
			FinOpsContext:  cfg.FinOpsContext,
		})
		total += len(recent)
		for _, a := range recent {
			if a.Deviation >= costThreshold {
				significant++
			}
			impact += a.Deviation
		}
	}
	return analysis, total, significant, impact
}

func runDiscoveryMode(raw map[string][]DimensionAnomaly, granularity string, originalThreshold float64, originalCutoff time.Time) *DiscoveryModeResult {
	r := &DiscoveryModeResult{PhasesExecuted: 1, OriginalThreshold: originalThreshold}

	for _, anomalies := range raw {
		for _, a := range anomalies {
			t, err := parseAnomalyDate(a.Date, granularity)
			if err != nil {
				continue
			}
			if t.Before(originalCutoff) && a.Deviation >= originalThreshold {
				r.ThresholdMetNotRecent++
			}
			if !t.Before(originalCutoff) && a.Deviation < originalThreshold {
				r.RecentNotThreshold++
			}
		}
	}

	if r.ThresholdMetNotRecent > 0 {
		r.PhasesExecuted = 2
		steps := discoveryRecencyStepsMonthly
		if granularity == "day" {
			steps = discoveryRecencyStepsDaily
		}
		for _, step := range steps {
			parsed, _ := parseRecency(step, granularity)
			expanded := recencyCutoff(parsed, granularity)
			found := false
			for _, anomalies := range raw {
				for _, a := range anomalies {
					t, err := parseAnomalyDate(a.Date, granularity)
					if err != nil {
						continue
					}
					if !t.Before(expanded) && a.Deviation >= originalThreshold {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				r.RelaxedRecency = step
				break
			}
		}
	}

	if r.RecentNotThreshold == 0 {
		r.PhasesExecuted = 3
		for _, mult := range discoveryThresholdMultipliers[1:] {
			reduced := originalThreshold * mult
			count := 0
			for _, anomalies := range raw {
				for _, a := range anomalies {
					t, err := parseAnomalyDate(a.Date, granularity)
					if err != nil {
						continue
					}
					if !t.Before(originalCutoff) && a.Deviation >= reduced {
						count++
					}
				}
			}
			if count > 0 {
				r.RelaxedThreshold = reduced
				r.RecentNotThreshold = count
				break
			}
		}
	}

	return r
}

func buildDiscoveryAnalysis(raw map[string][]DimensionAnomaly, configs []dimensionConfig, granularity string, originalThreshold float64, d *DiscoveryModeResult) (analysis []DimensionAnomalyAnalysis, total int, significant int, impact float64) {
	analysis = make([]DimensionAnomalyAnalysis, 0)
	var relaxedCutoff time.Time
	if d.RelaxedRecency != "" {
		parsed, _ := parseRecency(d.RelaxedRecency, granularity)
		relaxedCutoff = recencyCutoff(parsed, granularity)
	} else {
		relaxedCutoff = staticNow().AddDate(-1, 0, 0)
	}
	threshold := originalThreshold
	if d.RelaxedThreshold > 0 {
		threshold = d.RelaxedThreshold
	}

	for _, cfg := range configs {
		anomalies, ok := raw[cfg.Category]
		if !ok || len(anomalies) == 0 {
			continue
		}
		var discovered []DimensionAnomaly
		for _, a := range anomalies {
			t, err := parseAnomalyDate(a.Date, granularity)
			if err != nil {
				continue
			}
			isRecent := !t.Before(relaxedCutoff)
			meets := a.Deviation >= threshold
			if !isRecent && !meets {
				continue
			}
			cat := DiscoveryCategoryPrimary
			ctxStr := ""
			meetsOrig := a.Deviation >= originalThreshold
			if meetsOrig && !isRecent {
				cat = DiscoveryCategoryThresholdNotRecent
				ctxStr = fmt.Sprintf("Anomaly of $%.2f meets threshold ($%.2f) but occurred on %s (outside original recency window)", a.Deviation, originalThreshold, a.Date)
			} else if isRecent && !meetsOrig {
				cat = DiscoveryCategoryRecentNotThreshold
				ctxStr = fmt.Sprintf("Recent anomaly on %s with $%.2f deviation (below original threshold of $%.2f)", a.Date, a.Deviation, originalThreshold)
			} else if !meetsOrig && !isRecent {
				cat = DiscoveryCategoryRelaxedBoth
				ctxStr = fmt.Sprintf("Found by relaxing both constraints: $%.2f deviation on %s", a.Deviation, a.Date)
			}
			tagged := a
			tagged.DiscoveryCategory = &cat
			tagged.DiscoveryContext = &ctxStr
			discovered = append(discovered, tagged)
		}
		if len(discovered) == 0 {
			continue
		}
		analysis = append(analysis, DimensionAnomalyAnalysis{
			Category:       cfg.Category,
			Description:    cfg.Description,
			AnomaliesFound: len(discovered),
			TopAnomalies:   discovered,
			FinOpsContext:  cfg.FinOpsContext + " [Discovery mode: constraints relaxed to find these anomalies]",
		})
		total += len(discovered)
		for _, a := range discovered {
			if a.Deviation >= originalThreshold {
				significant++
			}
			impact += a.Deviation
		}
	}
	return analysis, total, significant, impact
}
