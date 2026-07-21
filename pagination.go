package flexera

import (
	"context"
	"fmt"
	"net/url"
)

// Page represents a single page of paginated results
type Page[T any] struct {
	Values             []T
	NextPage           *string
	NextCursor         *string // MCP-compatible opaque cursor (uses skipToken)
	Total              *int64  // Total count of all items (if provided by API)
	NextPageDetail     *PageDetail
	PreviousPageDetail *PageDetail
}

type PageDetail struct {
	URL       *string
	SkipToken *string
}

// FetchAll fetches all pages by repeatedly calling fetchPage until no more pages
func FetchAll[T any](ctx context.Context, fetchPage func(skipToken *string) (Page[T], error)) ([]T, error) {
	return FetchAllWithProgress(ctx, fetchPage, nil)
}

// ProgressCallback is called after each page is fetched
// fetched: total items fetched so far
// estimatedTotal: estimated total items (0 if unknown)
type ProgressCallback func(fetched, estimatedTotal int)

// FetchAllWithProgress fetches all pages with optional progress reporting
func FetchAllWithProgress[T any](ctx context.Context, fetchPage func(skipToken *string) (Page[T], error), onProgress ProgressCallback) ([]T, error) {
	var all []T
	var skipToken *string
	var knownTotal *int64

	for {
		page, err := fetchPage(skipToken)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch page: %w", err)
		}

		all = append(all, page.Values...)

		// Capture the total from the first page if provided
		if knownTotal == nil && page.Total != nil {
			knownTotal = page.Total
		}

		// Report progress if callback provided
		if onProgress != nil {
			estimatedTotal := 0
			if knownTotal != nil {
				// Use the actual total from the API
				estimatedTotal = int(*knownTotal)
			} else if page.NextPage != nil && *page.NextPage != "" {
				// Total unknown and more pages exist
				estimatedTotal = 0 // Indicate unknown
			} else {
				// Last page reached, we know the exact total now
				estimatedTotal = len(all)
			}
			onProgress(len(all), estimatedTotal)
		}

		if page.NextPage == nil || *page.NextPage == "" {
			break
		}

		// Extract skipToken from nextPage URL
		token := ExtractSkipToken(page.NextPage)
		if token == nil {
			// No skipToken, perhaps end of pagination
			break
		}

		skipToken = token
	}

	return all, nil
}

// ExtractSkipToken extracts the skipToken from a NextPage URL
// IMPORTANT: Returns the token WITHOUT URL-decoding to avoid double-encoding
// when the OpenAPI client builds the next request URL
func ExtractSkipToken(nextPage *string) *string {
	if nextPage == nil || *nextPage == "" {
		return nil
	}

	u, err := url.Parse(*nextPage)
	if err != nil {
		return nil
	}

	// Use RawQuery to avoid automatic URL decoding
	// The token is already URL-encoded in the response and will be
	// URL-encoded again by the OpenAPI client, so we must preserve
	// the original encoding to prevent double-encoding
	if u.RawQuery == "" {
		return nil
	}

	// Parse the raw query string manually to preserve encoding
	for _, param := range splitQuery(u.RawQuery) {
		key, value := splitParam(param)
		if key == "skipToken" && value != "" {
			return &value
		}
	}

	return nil
}

// splitQuery splits a raw query string into parameter pairs
func splitQuery(query string) []string {
	if query == "" {
		return nil
	}
	var params []string
	start := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '&' {
			params = append(params, query[start:i])
			start = i + 1
		}
	}
	params = append(params, query[start:])
	return params
}

// splitParam splits a parameter into key and value (preserving URL encoding)
func splitParam(param string) (string, string) {
	for i := 0; i < len(param); i++ {
		if param[i] == '=' {
			return param[:i], param[i+1:]
		}
	}
	return param, ""
}
