package flexera

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// optimaHostedPathPrefixes are the top-level path prefixes (relative to
// Server) whose operations are served from a dedicated Optima backend
// (api.optima*.flexeraeng.com), not the unified API gateway, even though
// their OpenAPI operations are merged into the same document as
// everything else:
//
//   - /bill-analysis  — bill_analysis (adjustments, bill-months, custom
//     dimensions, billing settings, cloud vendor accounts, ...)
//   - /analytics       — billing_center_service (billing centers,
//     allocation tables, access rules)
//   - /recommendations — optima_recommendations
//
// NOTE: /recommendation (singular) is the unrelated Flexera risk
// misconfiguration API and stays on the regular gateway host.
var optimaHostedPathPrefixes = []string{"/bill-analysis", "/analytics", "/recommendations"}

// isOptimaHostedPath reports whether path belongs to one of the merged
// spec's Optima-hosted services (see optimaHostedPathPrefixes).
func isOptimaHostedPath(path string) bool {
	for _, prefix := range optimaHostedPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// optimaRoutingDoer rewrites the scheme and host of outgoing requests whose
// path is Optima-hosted (see isOptimaHostedPath) to point at optimaHost,
// leaving the path, query, and body untouched: the Optima host serves the
// exact same path shape the unified spec declares.
type optimaRoutingDoer struct {
	next       HttpRequestDoer
	optimaHost *url.URL
}

func (d *optimaRoutingDoer) Do(req *http.Request) (*http.Response, error) {
	if !isOptimaHostedPath(req.URL.Path) {
		return d.next.Do(req)
	}
	rerouted := req.Clone(req.Context())
	rerouted.URL.Scheme = d.optimaHost.Scheme
	rerouted.URL.Host = d.optimaHost.Host
	rerouted.Host = d.optimaHost.Host
	// The Optima backends (unlike the unified gateway) reject requests
	// missing this header; service/optima.NewClients defaults its own
	// three rightscale clients to the same value.
	if rerouted.Header.Get("Api-Version") == "" {
		rerouted.Header.Set("Api-Version", "1.0")
	}
	return d.next.Do(rerouted)
}

// WithOptimaRouting returns a ClientOption that transparently reroutes
// Optima-hosted requests (see optimaHostedPathPrefixes) to optimaBaseURL
// instead of the client's default Server. An empty optimaBaseURL is a
// no-op (Optima-hosted requests then fall through to the gateway host,
// which will 404).
//
// NewClientWithResponsesForAuth installs this automatically, deriving
// optimaBaseURL from ClientAuth.Zone (or ClientAuth.OptimaBaseURL, if set),
// so callers normally never need this directly.
func WithOptimaRouting(optimaBaseURL string) ClientOption {
	return func(c *Client) error {
		if strings.TrimSpace(optimaBaseURL) == "" {
			return nil
		}
		u, err := url.Parse(optimaBaseURL)
		if err != nil {
			return fmt.Errorf("WithOptimaRouting: parse optima base URL: %w", err)
		}
		next := c.Client
		if next == nil {
			next = &http.Client{}
		}
		c.Client = &optimaRoutingDoer{next: next, optimaHost: u}
		return nil
	}
}
