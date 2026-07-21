package billingcenterservice

import (
	"context"
	"net/http"
)

// This file is for any extra types or functions that are not
// provided by the generated code

// WithHeader adds a header to every request made by the client
func WithHeader(name, value string) ClientOption {
	return func(c *Client) error {
		c.RequestEditors = append(c.RequestEditors, func(ctx context.Context, req *http.Request) error {
			req.Header.Add(name, value)
			return nil
		})
		return nil
	}
}
