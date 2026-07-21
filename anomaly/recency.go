package anomaly

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Recency aliases for agent ergonomics.
const (
	RecencyCurrentMonth = "current_month"
	RecencyCurrentWeek  = "current_week"
	RecencyToday        = "today"
)

// Default recency values per granularity.
const (
	DefaultRecencyMonthly = "P0M" // Current month only
	DefaultRecencyDaily   = "P7D" // Last 7 days
)

// Default baseline lookback per granularity (data sent to AI model).
const (
	BaselineLookbackMonthly = 13 // months
	BaselineLookbackDaily   = 90 // days
)

// Discovery mode relaxation steps.
var discoveryRecencyStepsDaily = []string{"P7D", "P14D", "P30D", "P90D"}
var discoveryRecencyStepsMonthly = []string{"P0M", "P1M", "P3M", "P6M"}
var discoveryThresholdMultipliers = []float64{1.0, 0.5, 0.25, 0.1}

// DiscoveryCategory values classify how an anomaly was found.
const (
	DiscoveryCategoryPrimary            = "primary"
	DiscoveryCategoryThresholdNotRecent = "threshold_met_not_recent"
	DiscoveryCategoryRecentNotThreshold = "recent_not_threshold"
	DiscoveryCategoryRelaxedBoth        = "discovery_relaxed_both"
)

var iso8601DurationRegex = regexp.MustCompile(`^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// ParsedRecency represents a parsed recency duration.
type ParsedRecency struct {
	Years  int
	Months int
	Weeks  int
	Days   int
	IsZero bool
	Raw    string
}

// parseRecency parses an ISO 8601 duration string (or named alias) into
// a ParsedRecency.
func parseRecency(recency string, granularity string) (ParsedRecency, error) {
	if recency == "" {
		if granularity == "day" {
			recency = DefaultRecencyDaily
		} else {
			recency = DefaultRecencyMonthly
		}
	}
	switch recency {
	case RecencyCurrentMonth:
		return ParsedRecency{Months: 0, IsZero: true, Raw: "P0M"}, nil
	case RecencyCurrentWeek:
		return ParsedRecency{Weeks: 0, Days: 7, Raw: "P7D"}, nil
	case RecencyToday:
		return ParsedRecency{Days: 0, IsZero: true, Raw: "P0D"}, nil
	}
	matches := iso8601DurationRegex.FindStringSubmatch(recency)
	if matches == nil {
		return ParsedRecency{}, fmt.Errorf("invalid recency format: %q (expected ISO 8601 duration like P7D, P1M, P14D, or alias: current_month, current_week, today)", recency)
	}
	parsed := ParsedRecency{Raw: recency}
	if matches[1] != "" {
		parsed.Years, _ = strconv.Atoi(matches[1])
	}
	if matches[2] != "" {
		parsed.Months, _ = strconv.Atoi(matches[2])
	}
	if matches[3] != "" {
		parsed.Weeks, _ = strconv.Atoi(matches[3])
	}
	if matches[4] != "" {
		parsed.Days, _ = strconv.Atoi(matches[4])
	}
	if parsed.Years == 0 && parsed.Months == 0 && parsed.Weeks == 0 && parsed.Days == 0 {
		parsed.IsZero = true
	}
	return parsed, nil
}

// recencyCutoff calculates the earliest date that qualifies as "recent".
func recencyCutoff(recency ParsedRecency, granularity string) time.Time {
	now := staticNow()
	if recency.IsZero {
		if recency.Raw == "P0M" || recency.Raw == RecencyCurrentMonth {
			return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		if recency.Raw == "P0D" || recency.Raw == RecencyToday {
			return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		}
		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		daysBack := int(weekday) - 1
		return time.Date(now.Year(), now.Month(), now.Day()-daysBack, 0, 0, 0, 0, time.UTC)
	}
	return now.AddDate(-recency.Years, -recency.Months, -(recency.Days + recency.Weeks*7))
}

// filterAnomaliesByRecency keeps only anomalies on or after the cutoff.
func filterAnomaliesByRecency(anomalies []DimensionAnomaly, cutoff time.Time, granularity string) []DimensionAnomaly {
	filtered := make([]DimensionAnomaly, 0, len(anomalies))
	for _, a := range anomalies {
		t, err := parseAnomalyDate(a.Date, granularity)
		if err != nil {
			continue
		}
		if !t.Before(cutoff) {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// parseAnomalyDate parses an anomaly date string based on granularity.
func parseAnomalyDate(dateStr string, granularity string) (time.Time, error) {
	if granularity == "month" {
		if t, err := time.Parse("2006-01", dateStr); err == nil {
			return t, nil
		}
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			return t, nil
		}
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			return t, nil
		}
		return time.Time{}, fmt.Errorf("cannot parse date %q for monthly granularity", dateStr)
	}
	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse date %q for daily granularity", dateStr)
}

// getBaselineLookback returns the fixed API window for baseline building.
func getBaselineLookback(granularity string) (startAt, endAt string, windowSize int64) {
	now := staticNow()
	windowSize = 1
	if granularity == "day" {
		startAt = now.AddDate(0, 0, -BaselineLookbackDaily).Format("2006-01-02")
		endAt = now.AddDate(0, 0, -1).Format("2006-01-02")
	} else {
		startAt = now.AddDate(0, -BaselineLookbackMonthly, 0).Format("2006-01")
		endAt = now.AddDate(0, 1, 0).Format("2006-01")
	}
	return startAt, endAt, windowSize
}

// mapLegacyLookbackPeriod maps the deprecated lookback_period values to
// the new granularity + recency pair.
func mapLegacyLookbackPeriod(lookback string) (granularity string, recency string, err error) {
	switch lookback {
	case "last_3d":
		return "day", "P3D", nil
	case "last_7d":
		return "day", "P7D", nil
	case "last_14d":
		return "day", "P14D", nil
	case "last_30d":
		return "day", "P30D", nil
	case "last_1m":
		return "month", "P0M", nil
	case "last_2m":
		return "month", "P2M", nil
	case "last_3m":
		return "month", "P3M", nil
	case "last_4m":
		return "month", "P4M", nil
	case "last_6m":
		return "month", "P6M", nil
	case "last_12m":
		return "month", "P12M", nil
	default:
		return "", "", fmt.Errorf("invalid lookback_period: %s", lookback)
	}
}
