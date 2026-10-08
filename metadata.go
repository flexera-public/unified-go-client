package flexera

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// APIOperation describes an operation independently of any CLI flag names.
// Definition retains the original operation, including extensions. Parameters
// includes inherited path-item parameters, with operation-level overrides.
// Examples are upstream documentation, not validated or sanitized requests.
type APIOperation struct {
	OperationID string            `json:"operationId"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Definition  json.RawMessage   `json:"definition"`
	Parameters  []json.RawMessage `json:"parameters"`
}

//go:embed unified-openapi/openapi3.json
var metadataSpec []byte

type apiMetadata struct {
	operations map[string]APIOperation
	components map[string]map[string]json.RawMessage
}

var metadataOnce sync.Once
var metadataIndex *apiMetadata
var metadataErr error

func loadAPIMetadata() (*apiMetadata, error) {
	metadataOnce.Do(func() { metadataIndex, metadataErr = parseAPIMetadata(metadataSpec) })
	return metadataIndex, metadataErr
}

func parseAPIMetadata(data []byte) (*apiMetadata, error) {
	var doc struct {
		Paths      map[string]map[string]json.RawMessage `json:"paths"`
		Components map[string]map[string]json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("API metadata: %w", err)
	}
	if doc.Paths == nil {
		return nil, fmt.Errorf("API metadata: paths are missing")
	}
	index := &apiMetadata{operations: map[string]APIOperation{}, components: doc.Components}
	for path, item := range doc.Paths {
		var inherited []json.RawMessage
		if raw := item["parameters"]; len(raw) != 0 {
			if err := json.Unmarshal(raw, &inherited); err != nil {
				return nil, fmt.Errorf("API metadata %s parameters: %w", path, err)
			}
		}
		for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
			raw := item[method]
			if len(raw) == 0 {
				continue
			}
			var op struct {
				ID         string            `json:"operationId"`
				Parameters []json.RawMessage `json:"parameters"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, fmt.Errorf("API metadata %s %s: %w", method, path, err)
			}
			if op.ID == "" {
				return nil, fmt.Errorf("API metadata %s %s: operationId is missing", method, path)
			}
			if _, exists := index.operations[op.ID]; exists {
				return nil, fmt.Errorf("API metadata: duplicate operationId %q", op.ID)
			}
			params := []json.RawMessage{}
			positions := map[string]int{}
			for _, group := range [][]json.RawMessage{inherited, op.Parameters} {
				for _, param := range group {
					param, err := index.resolveParameter(param)
					if err != nil {
						return nil, fmt.Errorf("API metadata %s: %w", op.ID, err)
					}
					var p struct {
						Name string `json:"name"`
						In   string `json:"in"`
					}
					if err := json.Unmarshal(param, &p); err != nil {
						return nil, fmt.Errorf("API metadata %s parameter: %w", op.ID, err)
					}
					if p.Name == "" || p.In == "" {
						return nil, fmt.Errorf("API metadata %s: parameter name/location is missing", op.ID)
					}
					key := p.In + ":" + p.Name
					if position, exists := positions[key]; exists {
						params[position] = param
					} else {
						positions[key] = len(params)
						params = append(params, param)
					}
				}
			}
			index.operations[op.ID] = APIOperation{op.ID, strings.ToUpper(method), path, raw, params}
		}
	}
	return index, nil
}

func (index *apiMetadata) resolveParameter(raw json.RawMessage) (json.RawMessage, error) {
	seen := map[string]bool{}
	for {
		var parameter struct {
			Ref string `json:"$ref"`
		}
		if err := json.Unmarshal(raw, &parameter); err != nil {
			return nil, err
		}
		if parameter.Ref == "" {
			return raw, nil
		}
		if !strings.HasPrefix(parameter.Ref, "#/components/parameters/") || seen[parameter.Ref] {
			return nil, fmt.Errorf("unsupported or cyclic parameter reference %q", parameter.Ref)
		}
		seen[parameter.Ref] = true
		var err error
		raw, err = index.component(parameter.Ref)
		if err != nil {
			return nil, err
		}
	}
}

func (index *apiMetadata) component(ref string) (json.RawMessage, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 4 || parts[0] != "#" || parts[1] != "components" {
		return nil, fmt.Errorf("API metadata: expected a local component reference, got %q", ref)
	}
	name := strings.NewReplacer("~1", "/", "~0", "~").Replace(parts[3])
	raw, found := index.components[parts[2]][name]
	if !found {
		return nil, fmt.Errorf("API metadata: unknown component %q", ref)
	}
	return raw, nil
}

// OperationMetadata returns a copy of the pinned OpenAPI metadata for an ID.
// It performs no authentication or network access.
func OperationMetadata(operationID string) (APIOperation, bool, error) {
	index, err := loadAPIMetadata()
	if err != nil {
		return APIOperation{}, false, err
	}
	op, found := index.operations[operationID]
	if !found {
		return APIOperation{}, false, nil
	}
	op.Definition = append(json.RawMessage(nil), op.Definition...)
	params := make([]json.RawMessage, len(op.Parameters))
	for i, raw := range op.Parameters {
		params[i] = append(json.RawMessage(nil), raw...)
	}
	op.Parameters = params
	return op, found, nil
}

// OperationIDs lists the pinned operation IDs in deterministic order.
func OperationIDs() ([]string, error) {
	index, err := loadAPIMetadata()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(index.operations))
	for id := range index.operations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// ComponentMetadata returns a copy of a local component (schema, example,
// request body, response, etc.). References are not expanded or fetched.
func ComponentMetadata(ref string) (json.RawMessage, error) {
	index, err := loadAPIMetadata()
	if err != nil {
		return nil, err
	}
	raw, err := index.component(ref)
	return append(json.RawMessage(nil), raw...), err
}
