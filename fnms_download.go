package flexera

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// FnmsDownloadOptions configures streaming downloads. HTTPClient must be an
// unauthenticated client; API request editors and OAuth wrappers are never used
// for redirected requests. Its cookie jar is ignored. Nil uses a fresh client.
type FnmsDownloadOptions struct {
	HTTPClient *http.Client
	// MaxRedirects defaults to 10. Negative values are invalid.
	MaxRedirects int
}

// FnmsDownloadFile resolves File_show's 303 Location and streams the file to dst.
// It returns bytes written, including partial writes on failure. API authentication
// is used only for File_show, never for its Location or subsequent redirects.
// The API client's doer must be an http.Client or a built-in wrapper around one.
func (c *ClientWithResponses) FnmsDownloadFile(ctx context.Context, orgID int64, filename string, dst io.Writer, options *FnmsDownloadOptions, editors ...RequestEditorFn) (int64, error) {
	if dst == nil {
		return 0, fmt.Errorf("FNMS download: writer is required")
	}
	downloader, limit, err := fnmsDownloadClient(options)
	if err != nil {
		return 0, err
	}
	api, err := c.fnmsStreamingClient()
	if err != nil {
		return 0, err
	}
	resp, err := api.FnmsFileShow(ctx, orgID, filename, editors...)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		return 0, fnmsDownloadResponseError(resp)
	}
	target, err := fnmsRedirectURL(resp)
	if err != nil {
		return 0, err
	}
	return fnmsStreamDownload(ctx, downloader, target, dst, limit-1)
}

// FnmsDownloadReportExport streams an export status Path to dst. Same-origin
// API paths receive API authentication only on the initial request; all redirect
// destinations use the separate unauthenticated download client. External paths
// never receive API authentication. Relative paths are resolved against Server.
func (c *ClientWithResponses) FnmsDownloadReportExport(ctx context.Context, path string, dst io.Writer, options *FnmsDownloadOptions, editors ...RequestEditorFn) (int64, error) {
	if dst == nil {
		return 0, fmt.Errorf("FNMS download: writer is required")
	}
	downloader, limit, err := fnmsDownloadClient(options)
	if err != nil {
		return 0, err
	}
	api, err := c.fnmsStreamingClient()
	if err != nil {
		return 0, err
	}
	base, err := url.Parse(api.Server)
	if err != nil {
		return 0, err
	}
	target, err := url.Parse(path)
	if err != nil || path == "" {
		return 0, fmt.Errorf("FNMS download: invalid export path")
	}
	target = base.ResolveReference(target)
	if err := fnmsValidateDownloadURL(target); err != nil {
		return 0, err
	}
	if target.Scheme != base.Scheme || target.Host != base.Host {
		return fnmsStreamDownload(ctx, downloader, target, dst, limit)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, err
	}
	if err := api.applyEditors(ctx, req, editors); err != nil {
		return 0, err
	}
	resp, err := api.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if fnmsIsRedirect(resp.StatusCode) {
		target, err = fnmsRedirectURL(resp)
		if err != nil {
			return 0, err
		}
		return fnmsStreamDownload(ctx, downloader, target, dst, limit-1)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fnmsDownloadResponseError(resp)
	}
	return io.Copy(dst, resp.Body)
}

func (c *ClientWithResponses) fnmsStreamingClient() (*Client, error) {
	api, ok := c.ClientInterface.(*Client)
	if !ok {
		return nil, fmt.Errorf("FNMS download: requires a generated *Client")
	}
	doer, err := fnmsNoRedirectDoer(api.Client)
	if err != nil {
		return nil, err
	}
	clone := *api
	clone.Client = doer
	return &clone, nil
}

// Clone the built-in chain so redirect prevention does not mutate shared clients.
// Unknown doers are rejected: they might follow redirects while injecting auth.
func fnmsNoRedirectDoer(doer HttpRequestDoer) (HttpRequestDoer, error) {
	switch d := doer.(type) {
	case *http.Client:
		clone := *d
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return &clone, nil
	case *OAuth2Doer:
		underlying, err := fnmsNoRedirectDoer(d.underlying)
		if err != nil {
			return nil, err
		}
		return &fnmsOAuthDownloadDoer{OAuth2Doer: d, do: func(req *http.Request) (*http.Response, error) {
			d.mu.Lock()
			if d.accessToken == "" || time.Now().After(d.expiry) {
				if err := d.refreshTokenLocked(); err != nil {
					d.mu.Unlock()
					return nil, err
				}
			}
			token := d.accessToken
			d.mu.Unlock()
			req = req.Clone(req.Context())
			req.Header.Set("Authorization", "Bearer "+token)
			return underlying.Do(req)
		}}, nil
	case *retryingDoer:
		next, err := fnmsNoRedirectDoer(d.next)
		if err != nil {
			return nil, err
		}
		clone := *d
		clone.next = next
		return &clone, nil
	case *userAgentDoer:
		next, err := fnmsNoRedirectDoer(d.next)
		if err != nil {
			return nil, err
		}
		clone := *d
		clone.next = next
		return &clone, nil
	default:
		return nil, fmt.Errorf("FNMS download: unsupported API HTTP doer %T; use an http.Client or built-in wrappers to prevent credential forwarding", doer)
	}
}

type fnmsOAuthDownloadDoer struct {
	*OAuth2Doer
	do func(*http.Request) (*http.Response, error)
}

func (d *fnmsOAuthDownloadDoer) Do(req *http.Request) (*http.Response, error) { return d.do(req) }

func fnmsDownloadClient(options *FnmsDownloadOptions) (*http.Client, int, error) {
	client := http.Client{}
	limit := 10
	if options != nil {
		if options.MaxRedirects < 0 {
			return nil, 0, fmt.Errorf("FNMS download: negative redirect limit")
		}
		if options.MaxRedirects > 0 {
			limit = options.MaxRedirects
		}
		if options.HTTPClient != nil {
			client = *options.HTTPClient
		}
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client, limit, nil
}

func fnmsValidateDownloadURL(target *url.URL) error {
	if (target.Scheme != "https" && target.Scheme != "http") || target.Host == "" || target.User != nil {
		return fmt.Errorf("FNMS download: expected an HTTP(S) URL without user credentials")
	}
	return nil
}
func fnmsRedirectURL(resp *http.Response) (*url.URL, error) {
	if resp.Header.Get("Location") == "" {
		return nil, fmt.Errorf("FNMS download: redirect missing Location")
	}
	target, err := resp.Location()
	if err != nil {
		return nil, fmt.Errorf("FNMS download: invalid Location")
	}
	if err := fnmsValidateDownloadURL(target); err != nil {
		return nil, err
	}
	if resp.Request != nil && resp.Request.URL.Scheme == "https" && target.Scheme == "http" {
		return nil, fmt.Errorf("FNMS download: refusing HTTPS downgrade")
	}
	return target, nil
}
func fnmsIsRedirect(status int) bool {
	return status == 301 || status == 302 || status == 303 || status == 307 || status == 308
}
func fnmsDownloadResponseError(resp *http.Response) error {
	// Do not include signed URLs or untrusted response bodies in errors.
	return fmt.Errorf("FNMS download: unexpected HTTP status %d", resp.StatusCode)
}
func fnmsStreamDownload(ctx context.Context, client *http.Client, target *url.URL, dst io.Writer, remaining int) (int64, error) {
	for {
		if remaining < 0 {
			return 0, fmt.Errorf("FNMS download: redirect limit exceeded")
		}
		if err := fnmsValidateDownloadURL(target); err != nil {
			return 0, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return 0, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		if fnmsIsRedirect(resp.StatusCode) {
			target, err = fnmsRedirectURL(resp)
			resp.Body.Close()
			if err != nil {
				return 0, err
			}
			remaining--
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return 0, fnmsDownloadResponseError(resp)
		}
		n, err := io.Copy(dst, resp.Body)
		resp.Body.Close()
		return n, err
	}
}
