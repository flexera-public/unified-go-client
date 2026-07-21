package flexera

import (
	"encoding/json"
	"strings"
)

// FilterRecommendationsByCategory returns a re-marshaled JSON array containing
// only the recommendations whose `type` matches the given kind. The Optima
// recommendations index endpoint returns all categories mixed; callers filter
// client-side. The kind is matched case-insensitively after replacing
// underscores with spaces (e.g. "usage_reduction" matches "Usage Reduction").
//
// This lives in the library so every consumer (CLI, Terraform provider, web)
// filters recommendations the same way.
func FilterRecommendationsByCategory(body []byte, kind string) ([]byte, error) {
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	wantedType := strings.ToLower(strings.ReplaceAll(kind, "_", " "))
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if t, ok := r["type"].(string); ok && strings.EqualFold(strings.TrimSpace(t), wantedType) {
			out = append(out, r)
		}
	}
	return json.Marshal(out)
}
