package flexera

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Exercise actual HTTP responses through the generated client, retaining raw
// JSON so no float64 conversion happens before CollectPages sees the envelope.
func TestCollectPages_HTTPPreservesWireNumbers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		noPaginate bool
		failSecond bool
	}{
		{name: "all pages"},
		{name: "first page unchanged", noPaginate: true},
		{name: "partial result", failSecond: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			firstValues := `[{"id":9007199254740993,"nested":{"negative":-9007199254740993,"huge":18446744073709551616,"decimal":0.12345678901234567890123456789,"exponent":1.234567890123456789e+40},"array":[9007199254740995,1e-400]}]`
			secondValues := `[{"id":9007199254740997,"decimal":123456789.01234567890123456789}]`
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/iam/v1/orgs/42/access-policies" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Query().Get("skipToken") {
				case "":
					_, _ = fmt.Fprintf(w, `{"values":%s,"count":9007199254740993,"total":18446744073709551615,"metadata":{"number":9007199254740999},"prevPage":"old","nextPage":"/iam/v1/orgs/42/access-policies?skipToken=page2"}`, firstValues)
				case "page2":
					if tc.failSecond {
						http.Error(w, "temporary failure", http.StatusServiceUnavailable)
						return
					}
					_, _ = fmt.Fprintf(w, `{"values":%s,"count":2,"total":18446744073709551616}`, secondValues)
				default:
					t.Errorf("unexpected cursor: %s", r.URL.RawQuery)
					http.Error(w, "unexpected cursor", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			fetch := func(ctx context.Context, token *string) (any, error) {
				response, err := client.IamAccessPolicyIndex(ctx, 42, nil, func(_ context.Context, r *http.Request) error {
					if token != nil {
						q := r.URL.Query()
						q.Set("skipToken", *token)
						r.URL.RawQuery = q.Encode()
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return nil, fmt.Errorf("HTTP %d", response.StatusCode)
				}
				var envelope json.RawMessage
				err = json.NewDecoder(response.Body).Decode(&envelope)
				return envelope, err
			}
			result, err := CollectPages(context.Background(), tc.noPaginate, nil, fetch)
			if tc.failSecond {
				var partial *PartialError
				if !errors.As(err, &partial) || partial.LastNextPage != "/iam/v1/orgs/42/access-policies?skipToken=page2" {
					t.Fatalf("expected resumable partial error, got %v", err)
				}
				result = partial.Merged
			} else if err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			// RawMessage comparisons assert lexical preservation, not equality
			// after a second potentially lossy float64 decode.
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(wire, &envelope); err != nil {
				t.Fatal(err)
			}
			wantValues, wantCount, wantTotal := firstValues, "9007199254740993", "18446744073709551615"
			wantCalls := 1
			if !tc.noPaginate {
				wantCalls = 2
				if !tc.failSecond {
					wantValues = firstValues[:len(firstValues)-1] + "," + secondValues[1:]
					wantCount, wantTotal = "9007199254740995", "18446744073709551616"
					if _, ok := envelope["nextPage"]; ok {
						t.Fatal("nextPage retained after final page")
					}
					if _, ok := envelope["prevPage"]; ok {
						t.Fatal("prevPage retained after merge")
					}
				}
			}
			if calls != wantCalls {
				t.Fatalf("fetches = %d, want %d", calls, wantCalls)
			}
			for key, want := range map[string]string{
				"values": wantValues, "count": wantCount, "total": wantTotal,
				"metadata": `{"number":9007199254740999}`,
			} {
				// Map key order is irrelevant; compare each value object's raw
				// numeric fields recursively rather than reordering the wire.
				assertPaginationJSONNumbers(t, envelope[key], json.RawMessage(want))
			}
		})
	}
}

func assertPaginationJSONNumbers(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	var gotObject, wantObject map[string]json.RawMessage
	if json.Unmarshal(want, &wantObject) == nil && wantObject != nil {
		if err := json.Unmarshal(got, &gotObject); err != nil || len(gotObject) != len(wantObject) {
			t.Fatalf("object = %s, want %s", got, want)
		}
		for key, value := range wantObject {
			assertPaginationJSONNumbers(t, gotObject[key], value)
		}
		return
	}
	var gotArray, wantArray []json.RawMessage
	if json.Unmarshal(want, &wantArray) == nil && wantArray != nil {
		if err := json.Unmarshal(got, &gotArray); err != nil || len(gotArray) != len(wantArray) {
			t.Fatalf("array = %s, want %s", got, want)
		}
		for i, value := range wantArray {
			assertPaginationJSONNumbers(t, gotArray[i], value)
		}
		return
	}
	if string(got) != string(want) {
		t.Fatalf("wire value = %s, want %s", got, want)
	}
}
