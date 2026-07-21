package flexera

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
)

// GoaErrorMediaType is the Content-Type returned by Goa services for
// structured error payloads.
const GoaErrorMediaType = "application/vnd.goa.error"

// APIError represents the structured error envelope returned by Flexera
// Goa-based services.
type APIError struct {
	Fault     bool   `json:"fault"`
	ID        string `json:"id"`
	Message   string `json:"message"`
	Name      string `json:"name"`
	Temporary bool   `json:"temporary"`
	Timeout   bool   `json:"timeout"`
}

// ParseAPIError decodes a Goa-style error envelope from a response body.
// Returns ok=false when the body is empty, not JSON, or lacks recognisable
// error fields.
func ParseAPIError(body []byte) (*APIError, bool) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, false
	}
	var parsed APIError
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false
	}
	if strings.TrimSpace(parsed.Message) == "" &&
		strings.TrimSpace(parsed.Name) == "" &&
		strings.TrimSpace(parsed.ID) == "" {
		return nil, false
	}
	return &parsed, true
}

// Error formats an APIError into a status-prefixed message suitable for
// surfacing to end users.
func (e *APIError) Error() string {
	parts := []string{}
	if name := strings.TrimSpace(e.Name); name != "" {
		parts = append(parts, fmt.Sprintf("name=%s", name))
	}
	if id := strings.TrimSpace(e.ID); id != "" {
		parts = append(parts, fmt.Sprintf("id=%s", id))
	}
	parts = append(parts, fmt.Sprintf("fault=%t", e.Fault))
	parts = append(parts, fmt.Sprintf("temporary=%t", e.Temporary))
	parts = append(parts, fmt.Sprintf("timeout=%t", e.Timeout))

	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "unknown error"
	}
	return fmt.Sprintf("%s (%s)", message, strings.Join(parts, ", "))
}

// ResponseError builds an error describing a failed API response. It prefers
// the structured Goa envelope when present; otherwise it falls back to the
// raw body (or the status text when the body is empty).
func ResponseError(statusCode int, body []byte) error {
	if parsed, ok := ParseAPIError(body); ok {
		return fmt.Errorf("request failed with status %d: %s", statusCode, parsed.Error())
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return fmt.Errorf("request failed with status %d: %s", statusCode, message)
}

// ExpectStatus returns nil when statusCode is one of allowed, otherwise an
// error that wraps ResponseError(statusCode, body) with a "(allowed: ...)"
// note. Use after invoking a generated *WithResponse method to assert the
// expected write/read status code.
func ExpectStatus(statusCode int, body []byte, allowed ...int) error {
	if slices.Contains(allowed, statusCode) {
		return nil
	}
	return fmt.Errorf("unexpected status %d (allowed: %s): %w", statusCode, formatAllowedStatuses(allowed), ResponseError(statusCode, body))
}

func formatAllowedStatuses(allowed []int) string {
	if len(allowed) == 0 {
		return "none"
	}
	unique := make([]int, 0, len(allowed))
	seen := map[int]struct{}{}
	for _, code := range allowed {
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		unique = append(unique, code)
	}
	sort.Ints(unique)
	parts := make([]string, 0, len(unique))
	for _, code := range unique {
		parts = append(parts, fmt.Sprintf("%d", code))
	}
	return strings.Join(parts, ",")
}
