package flexera

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// isOptimaHostedPath reports whether path belongs to one of the merged
// spec's Optima-hosted services. The optimaHostedPathPrefixes it checks
// against is generated (see client_gen_optima_routing.go) directly from
// the merged spec's path-item-level "servers" overrides by
// cmd/split-client's computeOptimaHostedPrefixes, so it can never drift
// out of sync with unified-openapi/openapi3.json the way a hand-maintained
// list could: any spec whose merge_servers config changes automatically
// updates this list the next time `make generate` runs.
//
// NOTE: "/recommendation" (singular) is the unrelated Flexera risk
// misconfiguration API and correctly stays on the regular gateway host,
// since it is a distinct top-level prefix from "/recommendations".
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
