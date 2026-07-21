package flexera

import (
	"strings"
	"testing"
)

func TestParseAPIError_Valid(t *testing.T) {
	body := []byte(`{"name":"invalid_request","id":"req-1","message":"validation failed","fault":false,"temporary":false,"timeout":false}`)
	e, ok := ParseAPIError(body)
	if !ok || e == nil {
		t.Fatalf("expected parsed error, got ok=%v e=%v", ok, e)
	}
	if e.Name != "invalid_request" || e.Message != "validation failed" {
		t.Fatalf("unexpected parsed values: %+v", e)
	}
}

func TestParseAPIError_EmptyOrJunkReturnsFalse(t *testing.T) {
	if _, ok := ParseAPIError(nil); ok {
		t.Fatal("expected ok=false for nil body")
	}
	if _, ok := ParseAPIError([]byte("not json")); ok {
		t.Fatal("expected ok=false for non-JSON body")
	}
	if _, ok := ParseAPIError([]byte(`{}`)); ok {
		t.Fatal("expected ok=false for empty error envelope")
	}
}

func TestResponseError_Goa(t *testing.T) {
	body := []byte(`{"name":"conflict","id":"r1","message":"already exists","fault":false,"temporary":false,"timeout":false}`)
	err := ResponseError(409, body)
	got := err.Error()
	for _, want := range []string{"status 409", "already exists", "name=conflict", "id=r1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in error, got: %s", want, got)
		}
	}
}

func TestResponseError_Fallback(t *testing.T) {
	err := ResponseError(500, []byte("boom"))
	if !strings.Contains(err.Error(), "request failed with status 500: boom") {
		t.Fatalf("unexpected fallback error: %v", err)
	}
}

func TestExpectStatus_Allowed(t *testing.T) {
	if err := ExpectStatus(204, nil, 200, 204); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestExpectStatus_Rejects(t *testing.T) {
	err := ExpectStatus(409, []byte(`{"name":"conflict","message":"x","id":"i","fault":false,"temporary":false,"timeout":false}`), 201)
	if err == nil {
		t.Fatal("expected error")
	}
	got := err.Error()
	if !strings.Contains(got, "unexpected status 409") || !strings.Contains(got, "allowed: 201") {
		t.Fatalf("missing prefix/allowed hint: %s", got)
	}
}

func TestParseZone(t *testing.T) {
	cases := map[string]Zone{
		"":     ZoneNAM,
		"nam":  ZoneNAM,
		"NAM":  ZoneNAM,
		"eu":   ZoneEU,
		"apac": ZoneAPAC,
		"au":   ZoneAPAC,
		"test": ZoneTest,
	}
	for in, want := range cases {
		got, err := ParseZone(in)
		if err != nil {
			t.Fatalf("ParseZone(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseZone(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseZone("mars"); err == nil {
		t.Fatal("expected error for unknown zone")
	}
}

func TestOptimaBaseURL(t *testing.T) {
	if got := OptimaBaseURL(ZoneNAM); got != "https://api.optima.flexeraeng.com" {
		t.Fatalf("NAM: got %s", got)
	}
	if got := OptimaBaseURL(ZoneEU); got != "https://api.optima-eu.flexeraeng.com" {
		t.Fatalf("EU: got %s", got)
	}
	if got := OptimaBaseURL(Zone("garbage")); got != "" {
		t.Fatalf("unknown: got %s", got)
	}
}

func TestGrsBaseURLForZone(t *testing.T) {
	cases := map[Zone]string{
		ZoneNAM:  "https://grs-front.iam-us-east-1.flexeraeng.com",
		ZoneEU:   "https://grs-front.eu-central-1.iam-eu.flexeraeng.com",
		ZoneAPAC: "https://grs-front.ap-southeast-2.iam-apac.flexeraeng.com",
		ZoneTest: "https://grs-front.iam-us-east-1.flexeraengdev.com",
	}
	for z, want := range cases {
		if got := GrsBaseURLForZone(z); got != want {
			t.Fatalf("%s: got %s want %s", z, got, want)
		}
	}
	if got := GrsBaseURLForZone(Zone("garbage")); got != "" {
		t.Fatalf("unknown: got %s", got)
	}
}

func TestParseZone_AliasesAreCanonical(t *testing.T) {
	canon := map[Zone]struct{}{}
	for _, z := range canonicalZones {
		canon[z] = struct{}{}
	}
	for alias, target := range zoneAliases {
		if _, ok := canon[target]; !ok {
			t.Errorf("zoneAliases[%q] = %q is not a member of canonicalZones %v", alias, target, canonicalZones)
		}
	}
}
