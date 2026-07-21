package flexera

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryConfig configures the retry behavior of the wrapping HttpRequestDoer
// returned by NewRetryingHTTPClient / WithRetryingHTTPClient.
//
// The retry layer is reactive: it observes responses and retries transient
// failures (HTTP 429 and 5xx) and temporary transport errors with capped
// exponential backoff. On HTTP 429 it honors the Retry-After header (both
// delta-seconds and HTTP-date forms) in preference to the computed backoff.
//
// Only methods listed in RetryMethods are retried; the default set contains
// the idempotent methods GET, HEAD, and OPTIONS. Requests with a non-nil
// Body but no GetBody are never retried because their body cannot be safely
// replayed.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts after the initial
	// request. A value of 0 disables retries entirely.
	MaxRetries int
	// BaseDelay is the initial backoff between attempts.
	BaseDelay time.Duration
	// MaxDelay caps the exponential backoff between attempts. Does not
	// apply to server-supplied Retry-After hints; those are governed by
	// MaxRetryAfter.
	MaxDelay time.Duration
	// MaxRetryAfter caps how long a Retry-After header value may extend a
	// single wait. Defaults to 60 seconds when zero. Set to a negative
	// value to disable the cap entirely (not recommended).
	MaxRetryAfter time.Duration
	// Jitter, when true, applies +/- 25% random jitter to each backoff to
	// spread retries from concurrent callers.
	Jitter bool
	// RetryMethods is the set of uppercase HTTP methods eligible for retry.
	// When nil, defaults to GET, HEAD, OPTIONS.
	RetryMethods map[string]bool
}

// DefaultRetryConfig returns defaults suitable for the Flexera unified API
// under moderate-to-high concurrency: 6 retries, 1s base delay, 15s cap on
// computed backoff, Retry-After honored up to 60s, jitter on, idempotent
// methods only.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    6,
		BaseDelay:     1 * time.Second,
		MaxDelay:      15 * time.Second,
		MaxRetryAfter: 60 * time.Second,
		Jitter:        true,
	}
}

var defaultRetryMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// NewRetryingHTTPClient wraps doer with retry-on-429/5xx behavior using cfg.
// If doer is nil, http.DefaultClient is used as the underlying doer.
func NewRetryingHTTPClient(doer HttpRequestDoer, cfg RetryConfig) HttpRequestDoer {
	if doer == nil {
		doer = http.DefaultClient
	}
	return &retryingDoer{next: doer, cfg: normalizeRetryConfig(cfg), sleep: contextSleep}
}

// WithRetryingHTTPClient returns a ClientOption that wraps the unified
// client's current HTTP doer (or http.DefaultClient if none has been set) in
// a retry layer using cfg. Place this option after any WithHTTPClient option
// so the retry layer wraps the OAuth or custom doer.
func WithRetryingHTTPClient(cfg RetryConfig) ClientOption {
	return func(c *Client) error {
		c.Client = NewRetryingHTTPClient(c.Client, cfg)
		return nil
	}
}

func normalizeRetryConfig(cfg RetryConfig) RetryConfig {
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 500 * time.Millisecond
	}
	if cfg.MaxDelay < cfg.BaseDelay {
		cfg.MaxDelay = cfg.BaseDelay
	}
	if cfg.MaxRetryAfter == 0 {
		cfg.MaxRetryAfter = 60 * time.Second
	}
	if cfg.RetryMethods == nil {
		cfg.RetryMethods = defaultRetryMethods
	}
	return cfg
}

type retryingDoer struct {
	next  HttpRequestDoer
	cfg   RetryConfig
	sleep func(ctx context.Context, d time.Duration) error
}

// tokenInvalidator is implemented by HTTP doers that can flush a stale
// auth token so a subsequent call performs a fresh refresh. The retry
// layer detects this on its directly-wrapped doer (r.next) via type
// assertion and calls Invalidate before retrying a 401.
//
// *oauth2.OAuth2Doer (go_pkg/client/oauth2) satisfies this interface;
// the unified client wires the OAuth doer as r.next via the option
// ordering established in auth.go (NewOAuthClient).
type tokenInvalidator interface {
	Invalidate()
}

func (r *retryingDoer) Do(req *http.Request) (*http.Response, error) {
	var (
		resp     *http.Response
		err      error
		attempt  int
		curDelay = r.cfg.BaseDelay
	)

	// Body-replayability gate up front — a non-replayable body cannot
	// be retried under any branch (429/5xx/network OR 401-with-invalidator),
	// so short-circuit straight to the underlying doer.
	if !r.eligibleForReplay(req) {
		return r.next.Do(req)
	}

	for {
		resp, err = r.next.Do(req)

		retryable, waitHint := r.shouldRetry(resp, err)
		if !retryable || attempt >= r.cfg.MaxRetries {
			return resp, err
		}

		// Status-driven token invalidation: shouldRetry already gated the
		// 401 branch on r.next satisfying tokenInvalidator. 429/5xx/network
		// retries are still gated by method scope (idempotent-only by
		// default); 401-with-invalidator bypasses the method gate because
		// Invalidate + replay after a fresh /oidc/token is safe regardless
		// of method (the body-replay gate above still applies).
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			if inv, ok := r.next.(tokenInvalidator); ok {
				inv.Invalidate()
			}
		} else if !r.cfg.RetryMethods[strings.ToUpper(req.Method)] {
			return resp, err
		}

		drainAndClose(resp)

		var wait time.Duration
		if waitHint > 0 {
			wait = waitHint
			if r.cfg.MaxRetryAfter > 0 && wait > r.cfg.MaxRetryAfter {
				wait = r.cfg.MaxRetryAfter
			}
		} else {
			wait = curDelay
			if r.cfg.Jitter {
				wait = applyJitter(wait)
			}
			if wait > r.cfg.MaxDelay {
				wait = r.cfg.MaxDelay
			}
		}

		if sleepErr := r.sleep(req.Context(), wait); sleepErr != nil {
			return nil, sleepErr
		}

		if err := resetRequestBody(req); err != nil {
			return nil, err
		}

		if curDelay < r.cfg.MaxDelay {
			curDelay *= 2
			if curDelay > r.cfg.MaxDelay {
				curDelay = r.cfg.MaxDelay
			}
		}
		attempt++
	}
}

// eligibleForReplay reports whether req's body can be safely replayed by
// the retry layer. A body without a GetBody factory cannot be rewound.
func (r *retryingDoer) eligibleForReplay(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

// eligible is the original method+body gate used for 429/5xx/network
// errors. Method scope (GET/HEAD/OPTIONS by default) keeps non-idempotent
// methods out of blind replay. Retained for any direct callers/tests.
func (r *retryingDoer) eligible(req *http.Request) bool {
	if !r.cfg.RetryMethods[strings.ToUpper(req.Method)] {
		return false
	}
	return r.eligibleForReplay(req)
}

func (r *retryingDoer) shouldRetry(resp *http.Response, err error) (bool, time.Duration) {
	if err != nil {
		return isTemporaryNetErr(err), 0
	}
	if resp == nil {
		return false, 0
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		// 401 retries on ALL methods when the wrapped doer can
		// invalidate its cached token; without invalidation the retry
		// would just present the same stale token and 401 again.
		// Body-replay is enforced at the inner-loop level via
		// eligibleForReplay (called from Do up front).
		if _, ok := r.next.(tokenInvalidator); ok {
			return true, 0
		}
		return false, 0
	case resp.StatusCode == http.StatusTooManyRequests:
		return true, parseRetryAfter(resp.Header.Get("Retry-After"))
	case resp.StatusCode >= 500 && resp.StatusCode <= 599:
		return true, 0
	}
	return false, 0
}

func isTemporaryNetErr(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	type temporary interface{ Temporary() bool }
	type timeout interface{ Timeout() bool }
	var t temporary
	if errors.As(err, &t) && t.Temporary() {
		return true
	}
	var to timeout
	if errors.As(err, &to) && to.Timeout() {
		return true
	}
	// Treat unclassified transport errors as transient; idempotency gate
	// above guards against unsafe replays.
	return true
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func applyJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	// +/- 25%
	delta := float64(d) * 0.25
	offset := (rand.Float64()*2 - 1) * delta
	out := time.Duration(float64(d) + offset)
	if out < 0 {
		out = d
	}
	return out
}

func contextSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if ctx == nil {
		time.Sleep(d)
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func resetRequestBody(req *http.Request) error {
	if req.Body == nil || req.GetBody == nil {
		return nil
	}
	body, err := req.GetBody()
	if err != nil {
		return fmt.Errorf("reset request body for retry: %w", err)
	}
	req.Body = body
	return nil
}
