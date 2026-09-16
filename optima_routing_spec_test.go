package flexera

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestOptimaHostedPathPrefixes_CoversSpecMergeServers guards against the
// generated client_gen_optima_routing.go (produced by cmd/split-client's
// computeOptimaHostedPrefixes) drifting out of sync with the committed
// unified-openapi/openapi3.json — e.g. if the spec is regenerated/edited
// without also rerunning `make generate`, or if client_gen_optima_routing.go
// is hand-edited. Every path with a path-item "servers" override pointing
// at an Optima backend host must be recognized by isOptimaHostedPath, or
// requests for it will silently fall through to the unified gateway
// (api.flexera.{zone}) instead of the Optima host declared in the spec.
//
// This is a belt-and-suspenders check: cmd/split-client's own
// computeOptimaHostedPrefixes (see cmd/split-client/main_test.go) already
// derives and validates these prefixes at generation time. This test
// additionally catches the case where the *committed* generated file and
// the *committed* spec have fallen out of sync for any reason.
//
// This exact class of bug (a hand-maintained prefix list that fell out of
// sync with the spec) previously caused BillUpload operations (paths under
// /optima/...) to be silently sent to api.flexera.com instead of the
// Optima backend.
func TestOptimaHostedPathPrefixes_CoversSpecMergeServers(t *testing.T) {
	raw, err := os.ReadFile("unified-openapi/openapi3.json")
	if err != nil {
		t.Fatalf("reading committed spec: %v", err)
	}

	var doc struct {
		Paths map[string]struct {
			Servers []struct {
				URL string `json:"url"`
			} `json:"servers"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing committed spec: %v", err)
	}

	var uncovered []string
	for path, item := range doc.Paths {
		for _, server := range item.Servers {
			// Only paths overridden onto an Optima backend host are in
			// scope for optimaRoutingDoer; other overrides (e.g. the
			// /oidc/token login host) are handled by separate,
			// dedicated code paths (see auth.go) and are not expected
			// to be covered here.
			if !strings.Contains(server.URL, "optima") || !strings.Contains(server.URL, "flexeraeng.com") {
				continue
			}
			if !isOptimaHostedPath(path) {
				uncovered = append(uncovered, path+" -> "+server.URL)
			}
		}
	}

	if len(uncovered) > 0 {
		t.Fatalf("optimaHostedPathPrefixes is missing prefixes for %d path(s) with an Optima-hosted "+
			"merge_servers override in unified-openapi/openapi3.json; update optimaHostedPathPrefixes "+
			"in optima_routing.go to match specs.yaml's merge_servers entries:\n%s",
			len(uncovered), strings.Join(uncovered, "\n"))
	}
}
