package flexera

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GraphQLRequest describes a raw POST to the unified GraphQL endpoint. The
// generated client does not expose this path, so the helper sends a hand
// constructed request that still picks up any RequestEditors registered on
// the underlying Client (notably the bearer-token editor).
type GraphQLRequest struct {
	// Body is the raw payload to send (typically JSON). Required.
	Body []byte
	// ContentType defaults to "application/json".
	ContentType string
	// Accept defaults to "application/json".
	Accept string
	// AcceptEncoding is optional.
	AcceptEncoding string
	// AcceptLanguage is optional.
	AcceptLanguage string
	// Path overrides the default "/explore/graphql".
	Path string
}

// GraphQLResponse captures the raw HTTP response from a GraphQL call.
type GraphQLResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// GraphQL POSTs req to the unified GraphQL endpoint using c's base server,
// HTTP doer, and any registered request editors. It does not interpret the
// response body; callers should pass it to ResponseError on non-2xx.
func (c *Client) GraphQL(ctx context.Context, req GraphQLRequest, editors ...RequestEditorFn) (*GraphQLResponse, error) {
	if len(req.Body) == 0 {
		return nil, fmt.Errorf("graphql: request body is required")
	}
	path := req.Path
	if path == "" {
		path = "/explore/graphql"
	}
	server := strings.TrimRight(c.Server, "/")
	urlStr := server + path

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("graphql: build request: %w", err)
	}
	contentType := req.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	accept := req.Accept
	if accept == "" {
		accept = "application/json"
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Accept", accept)
	if req.AcceptEncoding != "" {
		httpReq.Header.Set("Accept-Encoding", req.AcceptEncoding)
	}
	if req.AcceptLanguage != "" {
		httpReq.Header.Set("Accept-Language", req.AcceptLanguage)
	}

	for _, fn := range c.RequestEditors {
		if err := fn(ctx, httpReq); err != nil {
			return nil, fmt.Errorf("graphql: apply client request editor: %w", err)
		}
	}
	for _, fn := range editors {
		if err := fn(ctx, httpReq); err != nil {
			return nil, fmt.Errorf("graphql: apply request editor: %w", err)
		}
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("graphql: call endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("graphql: read response: %w", err)
	}
	return &GraphQLResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
}
