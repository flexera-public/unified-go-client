package flexera

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"sync/atomic"
)

// DebugOptions configures HTTP request/response tracing produced by
// NewDebugDoer / WithHTTPDebug.
type DebugOptions struct {
	// Label is included in each banner line. Defaults to "flexera".
	Label string
	// IncludeBody controls whether request/response bodies are dumped.
	// Defaults to true.
	IncludeBody bool
	// RedactHeaders are header names whose values will be replaced with
	// "[REDACTED]" in the dump. The default redact set already covers
	// Authorization, Cookie, Set-Cookie, X-Api-Key, and
	// Proxy-Authorization; RedactHeaders extends that set.
	RedactHeaders []string
}

// redactHeader replaces every value for header h in hdr with
// [REDACTED] and returns the original values for restoration. Returns
// nil when the header is not present.
func redactHeader(hdr http.Header, h string) []string {
	vs := hdr.Values(h)
	if len(vs) == 0 {
		return nil
	}
	saved := append([]string(nil), vs...)
	hdr.Set(h, "[REDACTED]")
	return saved
}

// restoreHeader rewrites hdr[h] with the saved values from a prior
// redactHeader call. Multi-value-safe (Set-Cookie).
func restoreHeader(hdr http.Header, h string, saved []string) {
	if len(saved) == 0 {
		return
	}
	hdr.Del(h)
	for _, v := range saved {
		hdr.Add(h, v)
	}
}

// NewDebugDoer wraps inner with an HttpRequestDoer that writes each
// request and response (headers + body) to out. Sensitive headers
// listed in the default redact set are replaced with "[REDACTED]"
// before dumping; opts.RedactHeaders extends the set.
func NewDebugDoer(inner HttpRequestDoer, out io.Writer, opts DebugOptions) HttpRequestDoer {
	if inner == nil {
		inner = http.DefaultClient
	}
	if opts.Label == "" {
		opts.Label = "flexera"
	}
	if !opts.IncludeBody {
		// default to true when caller passed the zero value (existing
		// sentinel-flip behaviour preserved; I-3 deferred).
		opts.IncludeBody = true
	}
	// Default header redact set: covers standard secret-bearing
	// request/response headers. (Body redaction is NOT covered here —
	// see I-3 in the 2026-06-10 review for the deferred body-leak
	// fix.)
	redact := map[string]struct{}{
		http.CanonicalHeaderKey("Authorization"):       {},
		http.CanonicalHeaderKey("Cookie"):              {},
		http.CanonicalHeaderKey("Set-Cookie"):          {},
		http.CanonicalHeaderKey("X-Api-Key"):           {}, // also matches X-API-Key after canonicalization
		http.CanonicalHeaderKey("Proxy-Authorization"): {},
	}
	for _, h := range opts.RedactHeaders {
		redact[http.CanonicalHeaderKey(h)] = struct{}{}
	}
	return &debugDoer{inner: inner, out: out, opts: opts, redact: redact}
}

// WithHTTPDebug returns a ClientOption that wraps the unified client's
// current HTTP doer in a debug-logging layer. Place after WithHTTPClient and
// after WithRetryingHTTPClient so debug logging observes retries.
func WithHTTPDebug(out io.Writer, opts DebugOptions) ClientOption {
	return func(c *Client) error {
		c.Client = NewDebugDoer(c.Client, out, opts)
		return nil
	}
}

type debugDoer struct {
	inner  HttpRequestDoer
	out    io.Writer
	opts   DebugOptions
	redact map[string]struct{}
	seq    uint64
}

func (d *debugDoer) Do(req *http.Request) (*http.Response, error) {
	n := atomic.AddUint64(&d.seq, 1)

	// Redact request headers around DumpRequestOut, then restore.
	reqSaved := map[string][]string{}
	for h := range d.redact {
		if v := redactHeader(req.Header, h); v != nil {
			reqSaved[h] = v
		}
	}
	dumpReq, dumpErr := httputil.DumpRequestOut(req, d.opts.IncludeBody)
	for h, vs := range reqSaved {
		restoreHeader(req.Header, h, vs)
	}

	fmt.Fprintf(d.out, "\n--- %s debug request #%d ---\n", d.opts.Label, n)
	if dumpErr != nil {
		fmt.Fprintf(d.out, "(failed to dump request: %v)\n%s %s\n", dumpErr, req.Method, req.URL.String())
	} else {
		_, _ = d.out.Write(dumpReq)
		if !bytes.HasSuffix(dumpReq, []byte("\n")) {
			fmt.Fprintln(d.out)
		}
	}

	resp, err := d.inner.Do(req)
	if err != nil {
		fmt.Fprintf(d.out, "--- %s debug response #%d (transport error) ---\n%v\n", d.opts.Label, n, err)
		return nil, err
	}

	// Redact response headers around DumpResponse, then restore so
	// the caller's resp is unmodified. Set-Cookie can have multiple
	// values; redactHeader/restoreHeader preserves all of them.
	respSaved := map[string][]string{}
	for h := range d.redact {
		if v := redactHeader(resp.Header, h); v != nil {
			respSaved[h] = v
		}
	}
	dumpResp, dumpErr := httputil.DumpResponse(resp, d.opts.IncludeBody)
	for h, vs := range respSaved {
		restoreHeader(resp.Header, h, vs)
	}

	fmt.Fprintf(d.out, "--- %s debug response #%d ---\n", d.opts.Label, n)
	if dumpErr != nil {
		fmt.Fprintf(d.out, "(failed to dump response: %v)\nstatus=%s\n", dumpErr, resp.Status)
	} else {
		_, _ = d.out.Write(dumpResp)
		if !bytes.HasSuffix(dumpResp, []byte("\n")) {
			fmt.Fprintln(d.out)
		}
	}
	fmt.Fprintf(d.out, "--- end debug #%d ---\n\n", n)
	return resp, nil
}
