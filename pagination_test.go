package flexera

import (
	"context"
	"testing"
)

func TestFetchAll(t *testing.T) {
	ctx := context.Background()

	// Mock fetchPage that returns 3 pages
	pageCount := 0
	fetchPage := func(skipToken *string) (Page[int], error) {
		pageCount++
		switch pageCount {
		case 1:
			return Page[int]{
				Values:   []int{1, 2},
				NextPage: stringPtr("http://example.com?page=2&skipToken=abc"),
			}, nil
		case 2:
			return Page[int]{
				Values:   []int{3, 4},
				NextPage: stringPtr("http://example.com?page=3&skipToken=def"),
			}, nil
		case 3:
			return Page[int]{
				Values:   []int{5},
				NextPage: nil,
			}, nil
		default:
			t.Fatalf("unexpected page count %d", pageCount)
			return Page[int]{}, nil
		}
	}

	result, err := FetchAll(ctx, fetchPage)
	if err != nil {
		t.Fatalf("FetchAll failed: %v", err)
	}

	expected := []int{1, 2, 3, 4, 5}
	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("at index %d, expected %d, got %d", i, v, result[i])
		}
	}
}

func TestExtractSkipToken(t *testing.T) {
	tests := []struct {
		name     string
		nextPage *string
		expected *string
	}{
		{
			name:     "nil nextPage",
			nextPage: nil,
			expected: nil,
		},
		{
			name:     "empty nextPage",
			nextPage: stringPtr(""),
			expected: nil,
		},
		{
			name:     "valid URL with skipToken",
			nextPage: stringPtr("http://example.com?page=2&skipToken=abc123"),
			expected: stringPtr("abc123"),
		},
		{
			name:     "URL without skipToken",
			nextPage: stringPtr("http://example.com?page=2"),
			expected: nil,
		},
		{
			name:     "invalid URL",
			nextPage: stringPtr("not-a-url"),
			expected: nil,
		},
		{
			name:     "URL-encoded skipToken with %3D (encoded equals)",
			nextPage: stringPtr("https://api.flexera.com/policy/v1/orgs/36737/projects/138746/applied-policies?limit=1000&skipToken=CAEa%2BAR7ImlkIjoiNjc3NDFiZmFkNGM4OTUyYWMyNzk2MWUzIn0%3D"),
			expected: stringPtr("CAEa%2BAR7ImlkIjoiNjc3NDFiZmFkNGM4OTUyYWMyNzk2MWUzIn0%3D"),
		},
		{
			name:     "URL-encoded skipToken preserves plus signs",
			nextPage: stringPtr("http://example.com?skipToken=token+with+plus"),
			expected: stringPtr("token+with+plus"),
		},
		{
			name:     "skipToken as first parameter",
			nextPage: stringPtr("http://example.com?skipToken=first&limit=100"),
			expected: stringPtr("first"),
		},
		{
			name:     "skipToken as middle parameter",
			nextPage: stringPtr("http://example.com?limit=100&skipToken=middle&orderBy=name"),
			expected: stringPtr("middle"),
		},
		{
			name:     "skipToken as last parameter",
			nextPage: stringPtr("http://example.com?limit=100&skipToken=last"),
			expected: stringPtr("last"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSkipToken(tt.nextPage)
			if (result == nil && tt.expected != nil) || (result != nil && tt.expected == nil) {
				t.Errorf("ExtractSkipToken() = %v, expected %v", result, tt.expected)
			} else if result != nil && tt.expected != nil && *result != *tt.expected {
				t.Errorf("ExtractSkipToken() = %v, expected %v", *result, *tt.expected)
			}
		})
	}
}

// TestExtractSkipTokenPreservesEncoding verifies that we preserve URL encoding
// to prevent double-encoding when the OpenAPI client builds the next request
func TestExtractSkipTokenPreservesEncoding(t *testing.T) {
	// This is the critical test: ensure we DON'T decode the skipToken
	// because the OpenAPI client will encode it again
	nextPage := "https://api.flexera.com/data?skipToken=value%3D%3D"
	result := ExtractSkipToken(&nextPage)

	if result == nil {
		t.Fatal("ExtractSkipToken() returned nil")
	}

	// The token should still be URL-encoded (not decoded to "value==")
	if *result != "value%3D%3D" {
		t.Errorf("ExtractSkipToken() = %q, want %q (encoding should be preserved)", *result, "value%3D%3D")
	}

	// Verify the actual bug case from the logs
	buggyToken := "https://api.flexera.com/policy/v1/orgs/36737/projects/138746/applied-policies?limit=1000&orderBy=createdAt+desc&skipToken=CAEa%2BAR7ImlkIjoiNjc3NDFiZmFkNGM4OTUyYWMyNzk2MWUzIn0qDmNyZWF0ZWRBdCBkZXNjMOgH%3D"
	buggyResult := ExtractSkipToken(&buggyToken)

	if buggyResult == nil {
		t.Fatal("ExtractSkipToken() returned nil for real-world example")
	}

	// Should preserve %2B (encoded +) and %3D (encoded =)
	if *buggyResult != "CAEa%2BAR7ImlkIjoiNjc3NDFiZmFkNGM4OTUyYWMyNzk2MWUzIn0qDmNyZWF0ZWRBdCBkZXNjMOgH%3D" {
		t.Errorf("ExtractSkipToken() failed to preserve encoding for real-world token: got %q", *buggyResult)
	}
}

func stringPtr(s string) *string {
	return &s
}
