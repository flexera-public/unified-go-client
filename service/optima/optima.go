// Package optima provides a thin facade over the three rightscale-generated
// clients that hang off the Optima zone host (api.optima*.flexeraeng.com):
//
//   - bill_analysis           — cost/spend analytics
//   - billing_center_service  — billing center CRUD
//   - optima_recommendations  — savings recommendations
//
// All three share authentication and base URL, so consumers nearly always
// want them as a bundle. NewClientsForZone wires that bundle up against a
// flexera.Zone using the canonical Optima host mapping in the unified
// client package.
package optima

import (
	"context"
	"fmt"
	"net/http"

	flexera "github.com/flexera-public/unified-go-client"
	ba "github.com/flexera-public/unified-go-client/rightscale/bill_analysis"
	bcs "github.com/flexera-public/unified-go-client/rightscale/billing_center_service"
	optrec "github.com/flexera-public/unified-go-client/rightscale/optima_recommendations"
)

// Clients bundles the three rightscale clients that live on the Optima
// host. All three share the same HTTP doer and request editors.
type Clients struct {
	BillAnalysis          *ba.ClientWithResponses
	BillingCenterService  *bcs.ClientWithResponses
	OptimaRecommendations *optrec.ClientWithResponses
}

// Config configures NewClients.
type Config struct {
	// BaseURL overrides the Optima base URL. When empty,
	// flexera.OptimaBaseURL(Zone) is used.
	BaseURL string
	// Zone is the Flexera zone; used to derive BaseURL when BaseURL is
	// empty.
	Zone flexera.Zone
	// AccessToken, when non-empty AND TokenSource is nil, is sent as a
	// Bearer token on every request.
	AccessToken string
	// TokenSource, when non-nil, is consulted on every request to
	// obtain the bearer token. Takes precedence over AccessToken. The
	// ctx passed in is the request's context, so cancellation
	// propagates wherever the underlying source supports it.
	//
	// For long-running operations (paginated cost queries, anomaly
	// walks) prefer TokenSource over AccessToken so a token rotation
	// mid-walk is transparent. Wire flexera.SharedTokenSource.Token
	// for shared-cache semantics with the unified API client.
	TokenSource func(ctx context.Context) (string, error)
	// APIVersion is sent as the Api-Version header. Defaults to "1.0".
	APIVersion string
	// HTTPClient is the underlying HTTP doer. Defaults to
	// http.DefaultClient.
	HTTPClient HttpRequestDoer
	// RequestEditors are appended after the built-in auth + api-version
	// editor.
	RequestEditors []func(ctx context.Context, req *http.Request) error
}

// HttpRequestDoer is the minimal interface satisfied by *http.Client.
type HttpRequestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewClients constructs the three-client bundle from cfg.
func NewClients(cfg Config) (*Clients, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = flexera.OptimaBaseURL(cfg.Zone)
	}
	if baseURL == "" {
		return nil, fmt.Errorf("optima: BaseURL is required (zone %q has no default)", cfg.Zone)
	}

	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = "1.0"
	}

	var httpDoer HttpRequestDoer = cfg.HTTPClient
	if httpDoer == nil {
		httpDoer = http.DefaultClient
	}

	authEditor := func(ctx context.Context, req *http.Request) error {
		token := cfg.AccessToken
		if cfg.TokenSource != nil {
			t, err := cfg.TokenSource(ctx)
			if err != nil {
				return fmt.Errorf("optima: token source: %w", err)
			}
			if t == "" {
				return fmt.Errorf("optima: token source returned empty token")
			}
			token = t
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if req.Header.Get("Api-Version") == "" {
			req.Header.Set("Api-Version", apiVersion)
		}
		return nil
	}

	baDoer := adaptDoer[ba.HttpRequestDoer](httpDoer)
	bcsDoer := adaptDoer[bcs.HttpRequestDoer](httpDoer)
	optDoer := adaptDoer[optrec.HttpRequestDoer](httpDoer)

	baOpts := []ba.ClientOption{ba.WithHTTPClient(baDoer), ba.WithRequestEditorFn(authEditor)}
	bcsOpts := []bcs.ClientOption{bcs.WithHTTPClient(bcsDoer), bcs.WithRequestEditorFn(authEditor)}
	optOpts := []optrec.ClientOption{optrec.WithHTTPClient(optDoer), optrec.WithRequestEditorFn(authEditor)}
	for _, ed := range cfg.RequestEditors {
		baOpts = append(baOpts, ba.WithRequestEditorFn(ed))
		bcsOpts = append(bcsOpts, bcs.WithRequestEditorFn(ed))
		optOpts = append(optOpts, optrec.WithRequestEditorFn(ed))
	}

	baClient, err := ba.NewClientWithResponses(baseURL, baOpts...)
	if err != nil {
		return nil, fmt.Errorf("optima: bill_analysis client: %w", err)
	}
	bcsClient, err := bcs.NewClientWithResponses(baseURL, bcsOpts...)
	if err != nil {
		return nil, fmt.Errorf("optima: billing_center_service client: %w", err)
	}
	optClient, err := optrec.NewClientWithResponses(baseURL, optOpts...)
	if err != nil {
		return nil, fmt.Errorf("optima: optima_recommendations client: %w", err)
	}

	return &Clients{
		BillAnalysis:          baClient,
		BillingCenterService:  bcsClient,
		OptimaRecommendations: optClient,
	}, nil
}

// NewClientsForZone is a convenience constructor for the common case of
// "give me Optima clients for zone X authenticated by bearer token Y".
func NewClientsForZone(zone flexera.Zone, accessToken string, httpClient HttpRequestDoer) (*Clients, error) {
	return NewClients(Config{
		Zone:        zone,
		AccessToken: accessToken,
		HTTPClient:  httpClient,
	})
}

// adaptDoer adapts our local HttpRequestDoer to the generated clients' own
// (structurally identical) interface. The runtime check exists because each
// generated package defines its own named interface type.
func adaptDoer[T any](d HttpRequestDoer) T {
	if d == nil {
		return any(http.DefaultClient).(T)
	}
	if v, ok := any(d).(T); ok {
		return v
	}
	return any(&doerShim{inner: d}).(T)
}

type doerShim struct{ inner HttpRequestDoer }

func (s *doerShim) Do(req *http.Request) (*http.Response, error) { return s.inner.Do(req) }
