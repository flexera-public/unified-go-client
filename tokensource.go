package flexera

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// SharedTokenSource is a single-flight, refresh-aware bearer-token source
// suitable for sharing across the unified API client and ancillary
// clients (e.g. Optima) within one process. It wraps an
// *oauth2.OAuth2Doer; the doer's lazy-refresh-on-expiry semantics provide
// the underlying token freshness, and the SharedTokenSource serializes
// concurrent callers so they observe a single fetch rather than a
// thundering herd.
//
// Concurrency: SharedTokenSource holds an outer mutex serializing
// Token/Invalidate callers; the underlying *OAuth2Doer holds its own
// internal mutex (added in Phase 1 §4) covering Do/Refresh/Invalidate so
// concurrent unified-API Do() and Optima Token() calls do not race on
// accessToken/expiry.
//
// Typical lifecycle (process-scoped, e.g. a CLI Factory):
//
//	src := unified.NewSharedTokenSource(oauth2.NewOAuth2Doer(...))
//	// inbound auth wiring on the unified API client:
//	cli, _ := unified.NewClientWithResponsesForAuth(unified.ClientAuth{
//	    Zone:              unified.ZoneNAM,
//	    SharedTokenSource: src,
//	})
//	// per-request bearer string for clients that don't accept a doer:
//	tok, err := src.Token(ctx)
//
// Invalidate forces the next Token call to bypass the cached token; the
// retry layer calls it on observed 401 so a server-side revocation
// recovers without operator intervention.
type SharedTokenSource struct {
	doer *OAuth2Doer
	mu   sync.Mutex
}

// NewSharedTokenSource wraps doer. The doer is the single source of
// token state; the SharedTokenSource adds single-flight semantics on top.
func NewSharedTokenSource(doer *OAuth2Doer) *SharedTokenSource {
	return &SharedTokenSource{doer: doer}
}

// Doer returns the underlying *oauth2.OAuth2Doer for clients that wire
// authentication via an HTTP doer (e.g. the unified API client through
// WithHTTPClient). Callers that only need a bearer string should prefer
// Token.
func (s *SharedTokenSource) Doer() *OAuth2Doer {
	if s == nil {
		return nil
	}
	return s.doer
}

// Token returns a current bearer access token, triggering a refresh if
// the underlying doer has none cached or its cached token has expired.
// The call is serialized: concurrent callers observe a single refresh
// round trip.
func (s *SharedTokenSource) Token(ctx context.Context) (string, error) {
	if s == nil || s.doer == nil {
		return "", errors.New("shared token source has no underlying OAuth2 doer")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok := s.doer.AccessToken(); tok != "" && time.Now().Before(s.doer.TokenExpiry()) {
		return tok, nil
	}
	if err := s.doer.Refresh(ctx); err != nil {
		return "", fmt.Errorf("oauth refresh: %w", err)
	}
	tok := s.doer.AccessToken()
	if tok == "" {
		return "", errors.New("oauth2 doer returned empty access token after refresh")
	}
	return tok, nil
}

// Invalidate clears the cached token on the underlying doer so the next
// Token call performs a fresh /oidc/token round trip. Safe to call
// concurrently with Token (the doer's internal mutex serializes the clear
// with in-progress Do/Refresh).
func (s *SharedTokenSource) Invalidate() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doer != nil {
		s.doer.Invalidate()
	}
}
