package anomaly

import (
	"context"
	"testing"
	"time"
)

func mockNow(t *testing.T, fixed time.Time) {
	t.Helper()
	orig := staticNow
	staticNow = func() time.Time { return fixed }
	t.Cleanup(func() { staticNow = orig })
}

func TestParseRecency_AliasesAndISO(t *testing.T) {
	cases := []struct {
		in         string
		gran       string
		wantRaw    string
		wantIsZero bool
		wantMonths int
		wantDays   int
	}{
		{"", "month", "P0M", true, 0, 0},
		{"", "day", "P7D", false, 0, 7},
		{"current_month", "month", "P0M", true, 0, 0},
		{"today", "day", "P0D", true, 0, 0},
		{"current_week", "day", "P7D", false, 0, 7},
		{"P3M", "month", "P3M", false, 3, 0},
		{"P14D", "day", "P14D", false, 0, 14},
	}
	for _, c := range cases {
		got, err := parseRecency(c.in, c.gran)
		if err != nil {
			t.Errorf("parseRecency(%q,%q) err: %v", c.in, c.gran, err)
			continue
		}
		if got.Raw != c.wantRaw || got.IsZero != c.wantIsZero || got.Months != c.wantMonths || got.Days != c.wantDays {
			t.Errorf("parseRecency(%q,%q) = %+v, want raw=%s zero=%v months=%d days=%d", c.in, c.gran, got, c.wantRaw, c.wantIsZero, c.wantMonths, c.wantDays)
		}
	}
}

func TestParseRecency_Invalid(t *testing.T) {
	if _, err := parseRecency("garbage", "month"); err == nil {
		t.Fatal("expected error for invalid recency")
	}
}

func TestRecencyCutoff_P0MIsStartOfMonth(t *testing.T) {
	mockNow(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	r, _ := parseRecency("P0M", "month")
	cut := recencyCutoff(r, "month")
	want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !cut.Equal(want) {
		t.Errorf("got %v, want %v", cut, want)
	}
}

func TestRecencyCutoff_P7DSubtracts7Days(t *testing.T) {
	mockNow(t, time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC))
	r, _ := parseRecency("P7D", "day")
	cut := recencyCutoff(r, "day")
	want := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	if !cut.Equal(want) {
		t.Errorf("got %v, want %v", cut, want)
	}
}

func TestFilterAnomaliesByRecency(t *testing.T) {
	cutoff := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	anomalies := []DimensionAnomaly{
		{Date: "2026-06-15"},
		{Date: "2026-06-05"},
		{Date: "2026-06-10"},
		{Date: "not-a-date"},
	}
	got := filterAnomaliesByRecency(anomalies, cutoff, "day")
	if len(got) != 2 || got[0].Date != "2026-06-15" || got[1].Date != "2026-06-10" {
		t.Fatalf("unexpected filter result: %+v", got)
	}
}

func TestMapLegacyLookbackPeriod(t *testing.T) {
	cases := map[string][2]string{
		"last_7d":  {"day", "P7D"},
		"last_3m":  {"month", "P3M"},
		"last_12m": {"month", "P12M"},
		"last_1m":  {"month", "P0M"},
	}
	for in, want := range cases {
		gran, rec, err := mapLegacyLookbackPeriod(in)
		if err != nil || gran != want[0] || rec != want[1] {
			t.Errorf("%q → %s/%s err=%v, want %s/%s", in, gran, rec, err, want[0], want[1])
		}
	}
	if _, _, err := mapLegacyLookbackPeriod("garbage"); err == nil {
		t.Error("expected error for invalid lookback")
	}
}

func TestGetBaselineLookback(t *testing.T) {
	mockNow(t, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	startD, endD, winD := getBaselineLookback("day")
	if winD != 1 || startD != "2026-03-17" || endD != "2026-06-14" {
		t.Errorf("day lookback got start=%s end=%s win=%d", startD, endD, winD)
	}
	startM, endM, winM := getBaselineLookback("month")
	if winM != 1 || startM != "2025-05" || endM != "2026-07" {
		t.Errorf("month lookback got start=%s end=%s win=%d", startM, endM, winM)
	}
}

func TestFormatDimensionValue(t *testing.T) {
	cases := []struct {
		in   map[string]string
		want string
	}{
		{nil, "All"},
		{map[string]string{}, "All"},
		{map[string]string{"service": "EC2", "region": "us-east-1"}, "region=us-east-1;service=EC2"},
		{map[string]string{"service": ""}, "Unknown"},
	}
	for _, c := range cases {
		if got := formatDimensionValue(c.in); got != c.want {
			t.Errorf("formatDimensionValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInvoke_RejectsInvalidGranularity(t *testing.T) {
	inv := New(nil, nil, nil)
	gran := "year"
	in := Input{OrgID: 1, Granularity: &gran}
	_, err := inv.Invoke(context.Background(), in)
	if err == nil || err.Error() == "" {
		t.Fatal("expected error for invalid granularity")
	}
}

func TestInvoke_RequiresClient(t *testing.T) {
	inv := &Investigator{}
	_, err := inv.Invoke(context.Background(), Input{OrgID: 1})
	if err == nil {
		t.Fatal("expected error when client is nil")
	}
}

func TestBuildAnalysisFromAnomalies(t *testing.T) {
	cutoff := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	raw := map[string][]DimensionAnomaly{
		"Service Anomaly": {
			{Date: "2026-06-12", Deviation: 2000},
			{Date: "2026-06-05", Deviation: 1800}, // dropped by recency
		},
		"Compute Resources": {
			{Date: "2026-06-15", Deviation: 500}, // below threshold (1500)
		},
	}
	configs := []dimensionConfig{
		{Category: "Service Anomaly", Description: "d1", FinOpsContext: "f1"},
		{Category: "Compute Resources", Description: "d2", FinOpsContext: "f2"},
	}
	analysis, total, sig, impact := buildAnalysisFromAnomalies(raw, configs, cutoff, "day", 1500)
	if total != 2 || sig != 1 || impact != 2500 {
		t.Fatalf("total=%d sig=%d impact=%v", total, sig, impact)
	}
	if len(analysis) != 2 {
		t.Fatalf("expected 2 dimension categories, got %d", len(analysis))
	}
	if analysis[0].AnomaliesFound != 1 || analysis[1].AnomaliesFound != 1 {
		t.Errorf("anomaly counts: %+v", analysis)
	}
}

func TestRunDiscoveryMode_RelaxesRecencyAndThreshold(t *testing.T) {
	mockNow(t, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	cutoff := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	raw := map[string][]DimensionAnomaly{
		"Service Anomaly": {
			{Date: "2026-04-01", Deviation: 2000}, // threshold-met but old
			{Date: "2026-06-10", Deviation: 50},   // recent but tiny
		},
	}
	r := runDiscoveryMode(raw, "month", 1500, cutoff)
	if r.ThresholdMetNotRecent != 1 {
		t.Errorf("ThresholdMetNotRecent = %d, want 1", r.ThresholdMetNotRecent)
	}
	if r.RecentNotThreshold == 0 {
		t.Errorf("RecentNotThreshold = 0, want >0 (recent low-deviation anomaly should count)")
	}
	if r.PhasesExecuted < 2 {
		t.Errorf("PhasesExecuted = %d, want >=2", r.PhasesExecuted)
	}
}

func TestToolContract(t *testing.T) {
	inv := New(nil, nil, nil)
	if inv.Name() != "anomaly-investigation" {
		t.Errorf("Name = %s", inv.Name())
	}
	if inv.Description() == "" {
		t.Error("Description should not be empty")
	}
}
