package billanalysis

import (
	"context"
	"net/http"
)

// WithHeaders adds headers to every request made by the client
func WithHeaders(headers map[string]string) ClientOption {
	return func(c *Client) error {
		c.RequestEditors = append(c.RequestEditors, func(ctx context.Context, req *http.Request) error {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			return nil
		})
		return nil
	}
}
