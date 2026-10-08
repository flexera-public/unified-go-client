package flexera

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func TestOperationMetadataPinnedSpec(t *testing.T) {
	ids, err := OperationIDs()
	if err != nil || len(ids) == 0 || !sort.StringsAreSorted(ids) {
		t.Fatalf("IDs: %v, %v", ids, err)
	}
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(metadataSpec, &doc); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, path := range doc.Paths {
		for method := range path {
			switch method {
			case "get", "put", "post", "delete", "options", "head", "patch", "trace":
				count++
			}
		}
	}
	if len(ids) != count {
		t.Fatalf("registry covers %d of %d operations", len(ids), count)
	}
	for _, id := range ids {
		op, found, err := OperationMetadata(id)
		if err != nil || !found || !json.Valid(op.Definition) {
			t.Fatalf("%s: %+v, %v", id, op, err)
		}
		if string(op.Definition) != string(doc.Paths[op.Path][strings.ToLower(op.Method)]) {
			t.Fatalf("%s definition changed", id)
		}
		op.Definition[0] = '!'
		if len(op.Parameters) > 0 {
			op.Parameters[0][0] = '!'
		}
		again, _, err := OperationMetadata(id)
		if err != nil || !json.Valid(again.Definition) || len(again.Parameters) > 0 && !json.Valid(again.Parameters[0]) {
			t.Fatalf("%s metadata is mutable", id)
		}
	}
	if _, found, err := OperationMetadata("not-an-operation"); err != nil || found {
		t.Fatalf("missing operation: found=%v, err=%v", found, err)
	}
}

func TestComponentMetadataCopies(t *testing.T) {
	var doc struct {
		Components map[string]map[string]json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(metadataSpec, &doc); err != nil {
		t.Fatal(err)
	}
	for name := range doc.Components["schemas"] {
		ref := "#/components/schemas/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
		raw, err := ComponentMetadata(ref)
		if err != nil {
			t.Fatal(err)
		}
		raw[0] = '!'
		again, err := ComponentMetadata(ref)
		if err != nil || !json.Valid(again) {
			t.Fatalf("shared component mutated: %s, %v", ref, err)
		}
		return
	}
	t.Fatal("pinned component fixture missing")
}

func TestMetadataInheritanceAndComponents(t *testing.T) {
	index, err := parseAPIMetadata([]byte(`{
		"components":{"parameters":{"Limit":{"$ref":"#/components/parameters/LimitValue"},"LimitValue":{"name":"limit","in":"query","description":"Inherited"}},"schemas":{"A/B":{"type":"string","example":"sample"}}},
		"paths":{"/widgets":{"parameters":[{"$ref":"#/components/parameters/Limit"},{"name":"id","in":"path","required":true}],
			"get":{"operationId":"get","parameters":[{"name":"limit","in":"query","description":"Override","examples":{"named":{"value":4}}}],"deprecated":true,"x-guidance":"preserved"}
		}}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	op := index.operations["get"]
	if len(op.Parameters) != 2 || !strings.Contains(string(op.Parameters[0]), "Override") || !strings.Contains(string(op.Definition), "x-guidance") {
		t.Fatalf("metadata lost: %+v", op)
	}
	if raw, err := index.component("#/components/schemas/A~1B"); err != nil || !strings.Contains(string(raw), "sample") {
		t.Fatalf("escaped component: %s, %v", raw, err)
	}
	for _, ref := range []string{"https://example.invalid/spec", "#/components/schemas/Missing"} {
		if _, err := index.component(ref); err == nil {
			t.Fatalf("accepted invalid reference %s", ref)
		}
	}
	for _, data := range []string{
		`{}`, `{"paths":{"/widgets":{"get":{}}}}`,
		`{"paths":{"/widgets":{"get":{"operationId":"same"},"post":{"operationId":"same"}}}}`,
		`{"paths":{"/widgets":{"get":{"operationId":"get","parameters":[{"$ref":"#/components/parameters/Missing"}]}}}}`,
		`{"components":{"parameters":{"Cycle":{"$ref":"#/components/parameters/Cycle"}}},"paths":{"/widgets":{"get":{"operationId":"get","parameters":[{"$ref":"#/components/parameters/Cycle"}]}}}}`,
	} {
		if _, err := parseAPIMetadata([]byte(data)); err == nil {
			t.Fatalf("accepted invalid metadata: %s", data)
		}
	}
}
