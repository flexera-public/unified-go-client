// Pagination helpers for the unified Flexera API.
//
// All paginated responses in the unified API share this JSON envelope:
//
//	{"nextPage": "https://...?skipToken=TOKEN", "values": [...], ...metadata...}
//
// CollectPages drives a fetch callback to walk the entire collection,
// merging values arrays and accumulating per-page counts. Because the
// generated client exposes paginated responses as concrete typed structs,
// the helper works at the JSON-envelope level (map[string]any) — callers
// marshal each page through json before passing it in.
//
// For typed-result aggregation (`[]T`) see go_pkg/pagination, which works
// against a `Page[T]` fetcher and remains the recommended path when the
// caller has already destructured the response.
package flexera

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// PageFetcher fetches one page of results. skipToken is nil on the first
// call (or contains the caller-supplied starting token); subsequent calls
// receive the token extracted from the previous page's nextPage URL.
//
// The returned value should be a JSON-encodable representation of the
// response envelope (typically the generated *200.JSON200 struct, or a
// map[string]any for ad-hoc requests).
type PageFetcher func(ctx context.Context, skipToken *string) (any, error)

// PartialError reports a pagination failure mid-walk, carrying the
// already-merged result envelope and the cursor that failed to fetch.
// Callers wishing to resume after a transient failure can extract via
// errors.As, retry the LastNextPage URL via the initialSkipToken
// parameter on a fresh CollectPages, and merge the result with
// PartialError.Merged.
//
// CollectPages returns *PartialError only on subsequent-page failures
// (i.e. at least one page was successfully merged). First-page failures
// surface the underlying error directly.
type PartialError struct {
	Merged       map[string]any
	LastNextPage string
	Err          error
}

func (e *PartialError) Error() string {
	if e == nil {
		return "<nil>"
	}
	n := 0
	if vals, _ := e.Merged["values"].([]any); vals != nil {
		n = len(vals)
	}
	return fmt.Sprintf("pagination: subsequent page failed after %d merged value(s) (lastNextPage=%q): %v", n, e.LastNextPage, e.Err)
}

func (e *PartialError) Unwrap() error { return e.Err }

// CollectPages invokes fetch until no nextPage is present, aggregating the
// "values" arrays from each page into the returned map. When noPaginate is
// true only the first page is returned (unchanged). When initialSkipToken
// is non-nil the first fetch receives that token (useful for resume-from
// workflows AND for resuming from a PartialError).
//
// Error semantics:
//
//   - First-page fetch failure: returns (nil, err) with err = underlying
//     fetch error. Caller has no partial state to recover.
//   - Subsequent-page failure: returns (merged, *PartialError) where
//     merged is the partial envelope and *PartialError.LastNextPage is
//     the cursor that failed. Caller may errors.As(err, &partialErr) to
//     extract.
//   - First-page response not a JSON object: returned as-is, no
//     pagination attempted.
func CollectPages(ctx context.Context, noPaginate bool, initialSkipToken *string, fetch PageFetcher) (any, error) {
	first, err := fetch(ctx, initialSkipToken)
	if err != nil {
		return nil, err
	}
	if noPaginate {
		return first, nil
	}

	merged, err := toMap(first)
	if err != nil {
		// Response is not a JSON object — return as-is, no pagination.
		return first, nil //nolint:nilerr
	}

	for {
		nextPageURL := stringVal(merged["nextPage"])
		if nextPageURL == "" {
			break
		}
		token, err := SkipTokenFromURL(nextPageURL)
		if err != nil {
			return merged, &PartialError{Merged: merged, LastNextPage: nextPageURL, Err: fmt.Errorf("pagination: %w", err)}
		}
		page, err := fetch(ctx, &token)
		if err != nil {
			return merged, &PartialError{Merged: merged, LastNextPage: nextPageURL, Err: err}
		}
		pageMap, err := toMap(page)
		if err != nil {
			return merged, &PartialError{Merged: merged, LastNextPage: nextPageURL, Err: fmt.Errorf("pagination: subsequent page is not a JSON object")}
		}
		appendValues(merged, pageMap)
		accumulateCount(merged, pageMap)
		if t, ok := pageMap["total"]; ok {
			merged["total"] = t
		}
		if np, ok := pageMap["nextPage"]; ok {
			merged["nextPage"] = np
		} else {
			delete(merged, "nextPage")
		}
		delete(merged, "prevPage")
	}

	return merged, nil
}

// SkipTokenFromURL extracts the skipToken query parameter from a nextPage URL.
func SkipTokenFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("cannot parse nextPage URL %q: %w", rawURL, err)
	}
	token := u.Query().Get("skipToken")
	if token == "" {
		return "", fmt.Errorf("nextPage URL has no skipToken parameter: %q", rawURL)
	}
	return token, nil
}

func appendValues(dst, src map[string]any) {
	dstVals, dstOK := dst["values"].([]any)
	srcVals, srcOK := src["values"].([]any)
	switch {
	case dstOK && srcOK:
		dst["values"] = append(dstVals, srcVals...)
	case !dstOK && srcOK:
		dst["values"] = srcVals
	}
}

func accumulateCount(dst, src map[string]any) {
	dc, dOK := dst["count"].(float64)
	sc, sOK := src["count"].(float64)
	if dOK && sOK {
		dst["count"] = dc + sc
	}
}

func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func stringVal(v any) string {
	s, _ := v.(string)
	return s
}
