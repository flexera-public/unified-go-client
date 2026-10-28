package flexera

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestCollectPagesAs_PreservesNumbersAndMetadata(t *testing.T) {
	type page struct {
		Values   []any  `json:"values"`
		Count    int64  `json:"count"`
		Total    uint64 `json:"total"`
		NextPage string `json:"nextPage,omitempty"`
		PrevPage string `json:"prevPage,omitempty"`
	}
	calls := 0
	start := "start"
	got, err := CollectPagesAs(context.Background(), false, &start, func(_ context.Context, token *string) (*page, error) {
		calls++
		if calls == 1 {
			if token == nil || *token != start {
				t.Fatalf("initial token = %v", token)
			}
			return &page{
				Values: []any{json.Number("9007199254740993")}, Count: 1, Total: 18446744073709551615,
				NextPage: "https://example.test?skipToken=2", PrevPage: "previous",
			}, nil
		}
		if calls != 2 || token == nil || *token != "2" {
			t.Fatalf("unexpected fetch: calls=%d token=%v", calls, token)
		}
		return &page{Values: []any{json.Number("-9007199254740993")}, Count: 1, Total: 18446744073709551615}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || got.Count != 2 || got.Total != 18446744073709551615 || got.NextPage != "" || got.PrevPage != "" {
		t.Fatalf("incorrect merged metadata: calls=%d result=%+v", calls, got)
	}
	if len(got.Values) != 2 || got.Values[0] != json.Number("9007199254740993") || got.Values[1] != json.Number("-9007199254740993") {
		t.Fatalf("numbers changed during typed conversion: %#v", got.Values)
	}
}

func TestCollectPagesAs_Errors(t *testing.T) {
	fetchErr := errors.New("fetch failed")
	t.Run("first page", func(t *testing.T) {
		got, err := CollectPagesAs(context.Background(), false, nil, func(context.Context, *string) (*IamUserList, error) {
			return nil, fetchErr
		})
		if got != nil || err != fetchErr {
			t.Fatalf("got (%v, %v), want (nil, original fetch error)", got, err)
		}
	})
	t.Run("partial result", func(t *testing.T) {
		calls := 0
		next := "https://example.test?skipToken=2"
		got, err := CollectPagesAs(context.Background(), false, nil, func(context.Context, *string) (*IamUserList, error) {
			calls++
			if calls > 1 {
				return nil, fetchErr
			}
			return &IamUserList{NextPage: &next, Values: []IamFlexeraIamUser{{Id: 123}}}, nil
		})
		var partial *PartialError
		if !errors.As(err, &partial) || !errors.Is(err, fetchErr) {
			t.Fatalf("lost partial/fetch error: %v", err)
		}
		if got == nil || len(got.Values) != 1 || got.Values[0].Id != 123 || partial.LastNextPage != next {
			t.Fatalf("incorrect partial response: got=%+v partial=%+v", got, partial)
		}
		if len(partial.Merged["values"].([]any)) != 1 {
			t.Fatal("missing raw partial result")
		}
	})
	for _, partialFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("conversion overflow, partial=%t", partialFailure), func(t *testing.T) {
			type page struct {
				Count    int64  `json:"count"`
				NextPage string `json:"nextPage,omitempty"`
			}
			calls := 0
			got, err := CollectPagesAs(context.Background(), false, nil, func(context.Context, *string) (*page, error) {
				calls++
				switch calls {
				case 1:
					return &page{Count: 9223372036854775807, NextPage: "https://example.test?skipToken=2"}, nil
				case 2:
					p := &page{Count: 1}
					if partialFailure {
						p.NextPage = "https://example.test?skipToken=3"
					}
					return p, nil
				default:
					return nil, fetchErr
				}
			})
			var conversionErr *json.UnmarshalTypeError
			if got != nil || !errors.As(err, &conversionErr) {
				t.Fatalf("expected nil result and conversion error, got (%+v, %v)", got, err)
			}
			var partial *PartialError
			if errors.As(err, &partial) != partialFailure || errors.Is(err, fetchErr) != partialFailure {
				t.Fatalf("pagination error not preserved: %v", err)
			}
			if partialFailure && partial.Merged["count"] != json.Number("9223372036854775808") {
				t.Fatalf("raw partial count lost: %+v", partial.Merged)
			}
		})
	}
}

func TestCollectPagesAs_Passthrough(t *testing.T) {
	t.Run("no paginate", func(t *testing.T) {
		first := &IamUserList{NextPage: stringPtr("https://example.test?skipToken=2")}
		got, err := CollectPagesAs(context.Background(), true, nil, func(context.Context, *string) (*IamUserList, error) {
			return first, nil
		})
		if err != nil || got != first {
			t.Fatalf("response not returned unchanged: (%p, %v)", got, err)
		}
	})
	t.Run("bare array", func(t *testing.T) {
		first := &IamRoleCollection{{Id: 1}}
		got, err := CollectPagesAs(context.Background(), false, nil, func(context.Context, *string) (*IamRoleCollection, error) {
			return first, nil
		})
		if err != nil || got != first {
			t.Fatalf("response not returned unchanged: (%p, %v)", got, err)
		}
	})
}

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
