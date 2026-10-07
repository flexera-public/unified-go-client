package flexera

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestToMap_PreservesTypedNumbers(t *testing.T) {
	page := struct {
		Values []int64 `json:"values"`
		Count  uint64  `json:"count"`
		Total  uint64  `json:"total"`
	}{[]int64{9007199254740993, -9007199254740993}, 9007199254740993, 18446744073709551615}
	got, err := toMap(page)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"count":9007199254740993,"total":18446744073709551615,"values":[9007199254740993,-9007199254740993]}`
	if string(wire) != want {
		t.Fatalf("numeric values changed: got %s, want %s", wire, want)
	}
}

func TestAccumulateCount_Exact(t *testing.T) {
	for _, tc := range []struct {
		name, first, second, want string
	}{
		{"ordinary", "2", "3", "5"},
		{"above float precision", "9007199254740993", "2", "9007199254740995"},
		{"above uint64", "18446744073709551615", "1", "18446744073709551616"},
		{"integer exponent", "9.007199254740993e15", "2.0", "9007199254740995"},
		{"fraction left unchanged", "1.25", "2", "1.25"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := map[string]any{"count": json.Number(tc.first)}
			accumulateCount(dst, map[string]any{"count": json.Number(tc.second)})
			if got := dst["count"]; got != json.Number(tc.want) {
				t.Fatalf("count = %v, want %s", got, tc.want)
			}
		})
	}
	for _, src := range []map[string]any{{}, {"count": nil}, {"count": "2"}} {
		dst := map[string]any{"count": json.Number("9007199254740993")}
		accumulateCount(dst, src)
		if dst["count"] != json.Number("9007199254740993") {
			t.Fatalf("missing/non-numeric count changed metadata: %v", dst)
		}
	}
}

func TestCollectPages_MergesAllPages(t *testing.T) {
	ctx := context.Background()

	calls := 0
	fetch := func(_ context.Context, skipToken *string) (any, error) {
		calls++
		switch calls {
		case 1:
			return map[string]any{
				"values":   []any{1.0, 2.0},
				"nextPage": "http://example.com?skipToken=page2",
			}, nil
		case 2:
			return map[string]any{
				"values":   []any{3.0},
				"nextPage": "",
			}, nil
		default:
			t.Fatalf("unexpected fetch call %d", calls)
			return nil, nil
		}
	}

	result, err := CollectPages(ctx, false, nil, fetch)
	if err != nil {
		t.Fatalf("CollectPages failed: %v", err)
	}
	merged := result.(map[string]any)
	values := merged["values"].([]any)
	if len(values) != 3 {
		t.Fatalf("expected 3 merged values, got %d (%v)", len(values), values)
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 fetch calls, got %d", calls)
	}
}

// TestCollectPages_RepeatedSkipTokenStopsLoop guards against a server that
// keeps returning the same (non-advancing) skipToken forever, which
// previously caused CollectPages to hang indefinitely.
func TestCollectPages_RepeatedSkipTokenStopsLoop(t *testing.T) {
	ctx := context.Background()

	calls := 0
	fetch := func(_ context.Context, skipToken *string) (any, error) {
		calls++
		if calls > 5 {
			t.Fatalf("CollectPages did not stop on a repeated skipToken (call %d)", calls)
		}
		// Every page (including the first) points to the same stuck
		// cursor, simulating a server whose pagination never advances.
		return map[string]any{
			"values":   []any{float64(calls)},
			"nextPage": "http://example.com?skipToken=stuck",
		}, nil
	}

	result, err := CollectPages(ctx, false, nil, fetch)
	if err != nil {
		t.Fatalf("CollectPages returned an error instead of stopping cleanly: %v", err)
	}
	merged := result.(map[string]any)
	values := merged["values"].([]any)
	// First page (calls==1) plus exactly one re-fetch of "stuck" (calls==2)
	// before the guard detects the repeat and stops.
	if len(values) != 2 {
		t.Fatalf("expected 2 merged values before loop-detection stopped, got %d (%v)", len(values), values)
	}
	if calls != 2 {
		t.Fatalf("expected exactly 2 fetch calls before stopping, got %d", calls)
	}
}

// TestCollectPages_MaxPagesCap guards against a server that produces an
// unbounded stream of distinct (non-repeating) skipTokens.
func TestCollectPages_MaxPagesCap(t *testing.T) {
	ctx := context.Background()

	calls := 0
	fetch := func(_ context.Context, skipToken *string) (any, error) {
		calls++
		return map[string]any{
			"values":   []any{float64(calls)},
			"nextPage": fmt.Sprintf("http://example.com?skipToken=token-%d", calls),
		}, nil
	}

	_, err := CollectPages(ctx, false, nil, fetch)
	if err == nil {
		t.Fatal("expected CollectPages to return an error after exceeding the max page cap")
	}
	var partialErr *PartialError
	if !errors.As(err, &partialErr) {
		t.Fatalf("expected a *PartialError, got %T: %v", err, err)
	}
	// One fetch for the first page, plus maxPaginationPages subsequent
	// fetches before the cap trips.
	if calls != maxPaginationPages+1 {
		t.Fatalf("expected %d fetch calls, got %d", maxPaginationPages+1, calls)
	}
}
