package flexera

import (
	"fmt"
	"slices"
	"strings"
)

// canonicalZones is the in-Go single source of truth for which Zone
// values the unified client accepts. validateZone (auth.go) and ParseZone
// derive their accept sets from this slice.
//
// merge.go's server-variable enum (zoneVariables) lists only the three
// production zones (com/eu/au) because ZoneTest deliberately bypasses the
// production server-URL template — its host is flexeratest.com, not
// flexera.test. See APIBaseURLForZone for the test-zone literal-host
// code path.
var canonicalZones = []Zone{ZoneNAM, ZoneEU, ZoneAPAC, ZoneTest}

// zoneAliases maps user-friendly inputs to a canonicalZones member.
// Every value MUST be a member of canonicalZones; TestParseZone enforces
// this invariant at test time.
var zoneAliases = map[string]Zone{
	"":            ZoneNAM,
	"default":     ZoneNAM,
	"nam":         ZoneNAM,
	"na":          ZoneNAM,
	"us":          ZoneNAM,
	"com":         ZoneNAM,
	"eu":          ZoneEU,
	"apac":        ZoneAPAC,
	"au":          ZoneAPAC,
	"test":        ZoneTest,
	"flexeratest": ZoneTest,
	"staging":     ZoneTest,
}

// IsCanonicalZone reports whether zone is a member of the in-Go
// canonical zone set used by validateZone and ParseZone.
func IsCanonicalZone(zone Zone) bool {
	return slices.Contains(canonicalZones, zone)
}

// ParseZone resolves a user-friendly zone string (accepting common aliases:
// nam/na/us/com/default, eu, apac/au, test/staging/flexeratest) into the
// canonical Zone constant. An empty input yields the default zone (NAM).
func ParseZone(raw string) (Zone, error) {
	zone, ok := zoneAliases[strings.ToLower(strings.TrimSpace(raw))]
	if !ok {
		return "", fmt.Errorf("unsupported zone %q; expected nam, eu, apac, or test", raw)
	}
	return zone, nil
}

// GrsBaseURLForZone returns the dedicated GRS (Governance / Resource Service)
// host for the given zone, or "" if no mapping is known.
//
// GRS is a legacy service surfaced outside the unified Flexera API gateway on
// a dedicated grs-front host. Project enumeration (GrsProjectIndexForOrg) has
// no IAM equivalent, so consumers that need to list an org's projects build a
// client pointed at this base URL and use ProjectResolver.
func GrsBaseURLForZone(zone Zone) string {
	switch zone {
	case ZoneNAM:
		return "https://grs-front.iam-us-east-1.flexeraeng.com"
	case ZoneEU:
		return "https://grs-front.eu-central-1.iam-eu.flexeraeng.com"
	case ZoneAPAC:
		return "https://grs-front.ap-southeast-2.iam-apac.flexeraeng.com"
	case ZoneTest:
		return "https://grs-front.iam-us-east-1.flexeraengdev.com"
	default:
		return ""
	}
}

// OptimaBaseURL returns the Optima (api.optima*.flexeraeng.com) base URL for
// the given zone, or "" if no mapping is known.
//
// The Optima APIs (bill_analysis, billing_center_service,
// optima_recommendations) live on a separate host from the unified Flexera
// API gateway. Consumers needing Optima clients should point them at the
// URL returned by this function for their zone.
func OptimaBaseURL(zone Zone) string {
	switch zone {
	case ZoneNAM:
		return "https://api.optima.flexeraeng.com"
	case ZoneEU:
		return "https://api.optima-eu.flexeraeng.com"
	case ZoneAPAC:
		return "https://api.optima-apac.flexeraeng.com"
	case ZoneTest:
		return "https://api.optima-test.flexeraeng.com"
	default:
		return ""
	}
}
